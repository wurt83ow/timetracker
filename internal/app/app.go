package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi"
	httpSwagger "github.com/swaggo/http-swagger"
	_ "github.com/wurt83ow/timetracker/docs" // подключение сгенерированных Swagger файлов
	"github.com/wurt83ow/timetracker/internal/apiservice"
	authz "github.com/wurt83ow/timetracker/internal/authorization"
	"github.com/wurt83ow/timetracker/internal/bdkeeper"
	"github.com/wurt83ow/timetracker/internal/config"
	"github.com/wurt83ow/timetracker/internal/controllers"
	"github.com/wurt83ow/timetracker/internal/logger"
	"github.com/wurt83ow/timetracker/internal/middleware"
	"github.com/wurt83ow/timetracker/internal/storage"
	"github.com/wurt83ow/timetracker/internal/workerpool"
	"go.uber.org/zap"
)

type Server struct {
	srv *http.Server
	ctx context.Context
}

func NewServer(ctx context.Context) *Server {
	server := new(Server)
	server.ctx = ctx
	return server
}

// !!! Заменить на .dev и conf
func ApiSystemAddress() string {
	return "localhost:8081"
}

func (server *Server) Serve() {
	// create and initialize a new option instance
	option := config.NewOptions()
	option.ParseFlags()
	fmt.Println(option.LogLevel())

	// get a new logger
	nLogger, err := logger.NewLogger(option.LogLevel())
	if err != nil {
		log.Fatalln(err)
	}

	nLogger.Info("Это Info", zap.Error(err))

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
	basecontr := initializeBaseController(server.ctx, memoryStorage, option, nLogger, authz)

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

	// Добавление маршрута для Swagger UI
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// configure and start the server
	server.srv = startServer(r, option.RunAddr())

	// Создаем канал для получения сигналов прерывания (например, CTRL+C)
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)

	// Блокируем выполнение до получения сигнала
	<-stopChan

	// Выполняем корректное завершение сервера
	server.Shutdown()
}

func initializeKeeper(dataBaseDSN func() string, logger *logger.Logger, userUpdateInterval func() string) *bdkeeper.BDKeeper {
	if dataBaseDSN() == "" {
		logger.Warn("DataBaseDSN is empty")
		return nil
	}

	return bdkeeper.NewBDKeeper(dataBaseDSN, logger, userUpdateInterval)
}

func initializeStorage(ctx context.Context, keeper storage.Keeper, logger *logger.Logger) *storage.MemoryStorage {
	if keeper == nil {
		logger.Warn("Keeper is nil, cannot initialize storage")
		return nil
	}

	return storage.NewMemoryStorage(ctx, keeper, logger)
}

func initializeBaseController(ctx context.Context, storage *storage.MemoryStorage, option *config.Options,
	logger *logger.Logger, authz *authz.JWTAuthz,
) *controllers.BaseController {
	return controllers.NewBaseController(ctx, storage, option, logger, authz)
}

func initializeWorkerPool(allTask []*workerpool.Task, option *config.Options, logger *logger.Logger) *workerpool.Pool {
	return workerpool.NewPool(allTask, option.Concurrency, logger, option.TaskExecutionInterval)
}

func initializeAuthz(option *config.Options, logger *logger.Logger) *authz.JWTAuthz {
	return authz.NewJWTAuthz(option.JWTSigningKey(), logger)
}

func initializeExtController(ctx context.Context, storage *storage.MemoryStorage, logger *logger.Logger) *controllers.ExtController {
	return controllers.NewExtController(ctx, storage, ApiSystemAddress, logger)
}

func initializeApiService(ctx context.Context, extcontr *controllers.ExtController, pool *workerpool.Pool, memoryStorage *storage.MemoryStorage, logger *logger.Logger, option *config.Options) *apiservice.ApiService {
	apiService := apiservice.NewApiService(ctx, extcontr, pool, memoryStorage, logger, option.TaskExecutionInterval)
	return apiService
}

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

func (server *Server) Shutdown() {
	log.Printf("server stopped")

	const shutdownTimeout = 5 * time.Second
	ctxShutDown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

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
