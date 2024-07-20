package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi"
	httpSwagger "github.com/swaggo/http-swagger"
	_ "github.com/wurt83ow/timetracker/docs" // connecting generated Swagger files
	"github.com/wurt83ow/timetracker/internal/apiservice"
	authz "github.com/wurt83ow/timetracker/internal/authorization"
	"github.com/wurt83ow/timetracker/internal/bdkeeper"
	"github.com/wurt83ow/timetracker/internal/config"
	"github.com/wurt83ow/timetracker/internal/controllers"
	"github.com/wurt83ow/timetracker/internal/logger"
	"github.com/wurt83ow/timetracker/internal/middleware"
	"github.com/wurt83ow/timetracker/internal/storage"
	"github.com/wurt83ow/timetracker/internal/workerpool"
)

type Server struct {
	srv *http.Server
	ctx context.Context
}

// NewServer creates a new Server instance with the provided context
func NewServer(ctx context.Context) *Server {
	server := new(Server)
	server.ctx = ctx
	return server
}

// !!! ApiSystemAddress returns the API system address
func ApiSystemAddress() string {
	return "localhost:8081"
}

// Serve starts the server and handles signal interruption for graceful shutdown
func (server *Server) Serve() {
	// create and initialize a new option instance
	option := config.NewOptions()
	option.ParseFlags()

	// get a new logger
	nLogger, err := logger.NewLogger(option.LogLevel())
	if err != nil {
		log.Fatalln(err)
	}

	// initialize the keeper instance
	keeper := initializeKeeper(option.DataBaseDSN, nLogger, option.UserUpdateInterval)
	if keeper == nil {
		nLogger.Debug("Failed to initialize keeper")
	}
	defer keeper.Close()

	// initialize the storage instance
	memoryStorage := initializeStorage(server.ctx, keeper, nLogger)
	if memoryStorage == nil {
		nLogger.Debug("Failed to initialize storage")
	}

	// create a new workerpool for concurrency task processing
	var allTask []*workerpool.Task
	pool := initializeWorkerPool(allTask, option, nLogger)

	// create a new NewJWTAuthz for user authorization
	authz := initializeAuthz(option, nLogger)

	// create a new controller to process incoming requests
	basecontr := initializeBaseController(server.ctx, memoryStorage, option.DefaultEndTime, nLogger, authz)

	// get a middleware for logging requests
	reqLog := middleware.NewReqLog(nLogger)

	// start the worker pool in the background
	go pool.RunBackground()

	// create a new controller for creating outgoing requests
	extcontr := initializeExtController(server.ctx, memoryStorage, nLogger)

	apiService := initializeApiService(server.ctx, extcontr, pool, memoryStorage, nLogger, option)
	apiService.Start()

	// create router and mount routes
	r := chi.NewRouter()
	r.Use(reqLog.RequestLogger)
	r.Mount("/", basecontr.Route())

	// Add route for Swagger UI
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// configure and start the server
	server.srv = startServer(r, option.RunAddr())

	// Create a channel to receive interrupt signals (e.g., CTRL+C)
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)

	// Block execution until a signal is received
	<-stopChan

	// Perform graceful server shutdown
	server.Shutdown()
}

// initializeKeeper initializes a BDKeeper instance
func initializeKeeper(dataBaseDSN func() string, logger *logger.Logger, userUpdateInterval func() string) *bdkeeper.BDKeeper {
	if dataBaseDSN() == "" {
		logger.Warn("DataBaseDSN is empty")
		return nil
	}

	return bdkeeper.NewBDKeeper(dataBaseDSN, logger, userUpdateInterval)
}

// initializeStorage initializes a MemoryStorage instance
func initializeStorage(ctx context.Context, keeper storage.Keeper, logger *logger.Logger) *storage.MemoryStorage {
	if keeper == nil {
		logger.Warn("Keeper is nil, cannot initialize storage")
		return nil
	}

	return storage.NewMemoryStorage(ctx, keeper, logger)
}

// initializeBaseController initializes a BaseController instance
func initializeBaseController(ctx context.Context, storage *storage.MemoryStorage, DefaultEndTime func() string,
	logger *logger.Logger, authz *authz.JWTAuthz,
) *controllers.BaseController {
	return controllers.NewBaseController(ctx, storage, DefaultEndTime, logger, authz)
}

// initializeWorkerPool initializes a worker pool with the provided tasks and options
func initializeWorkerPool(allTask []*workerpool.Task, option *config.Options, logger *logger.Logger) *workerpool.Pool {
	return workerpool.NewPool(allTask, option.Concurrency, logger, option.TaskExecutionInterval)
}

// initializeAuthz initializes a JWTAuthz instance for user authorization
func initializeAuthz(option *config.Options, logger *logger.Logger) *authz.JWTAuthz {
	return authz.NewJWTAuthz(option.JWTSigningKey(), logger)
}

// initializeExtController initializes an ExtController instance
func initializeExtController(ctx context.Context, storage *storage.MemoryStorage, logger *logger.Logger) *controllers.ExtController {
	return controllers.NewExtController(ctx, storage, ApiSystemAddress, logger)
}

// initializeApiService initializes an ApiService instance
func initializeApiService(ctx context.Context, extcontr *controllers.ExtController, pool *workerpool.Pool, memoryStorage *storage.MemoryStorage, logger *logger.Logger, option *config.Options) *apiservice.ApiService {
	apiService := apiservice.NewApiService(ctx, extcontr, pool, memoryStorage, logger, option.TaskExecutionInterval)
	return apiService
}

// startServer configures and starts an HTTP server with the provided router and address
func startServer(router chi.Router, address string) *http.Server {
	const (
		oneMegabyte = 1 << 20
		readTimeout = 3 * time.Second
	)

	server := &http.Server{
		Addr:                         address,
		Handler:                      router,
		ReadHeaderTimeout:            readTimeout,
		WriteTimeout:                 readTimeout,
		IdleTimeout:                  readTimeout,
		ReadTimeout:                  readTimeout,
		MaxHeaderBytes:               oneMegabyte, // 1 MB
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		TLSNextProto:                 nil,
		ConnState:                    nil,
		ErrorLog:                     nil,
		BaseContext:                  nil,
		ConnContext:                  nil,
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln(err)
		}
	}()

	return server
}

// Shutdown gracefully shuts down the server
func (server *Server) Shutdown() {
	log.Printf("server stopped")

	const shutdownTimeout = 5 * time.Second
	ctxShutDown, cancel := context.WithTimeout(server.ctx, shutdownTimeout)

	defer cancel()

	if server.srv != nil {
		if err := server.srv.Shutdown(ctxShutDown); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("server Shutdown Failed:%s", err)
			}
		}
	}

	log.Println("server exited properly")
}
