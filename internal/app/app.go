package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	authz "github.com/wurt83ow/timetracker/internal/authorization"
	"github.com/wurt83ow/timetracker/internal/bdkeeper"
	"github.com/wurt83ow/timetracker/internal/config"
	"github.com/wurt83ow/timetracker/internal/controllers"
	"github.com/wurt83ow/timetracker/internal/logger"
	"github.com/wurt83ow/timetracker/internal/middleware"
	"github.com/wurt83ow/timetracker/internal/storage"
	"go.uber.org/zap"
)

type Server struct {
	srv *http.Server
	ctx context.Context
	// db  *pgxpool.Pool
}

func NewServer(ctx context.Context) *Server {
	server := new(Server)
	server.ctx = ctx

	return server
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
	defer keeper.Close()

	// initialize the storage instance
	memoryStorage := initializeStorage(keeper, nLogger)

	// create a new NewJWTAuthz for user authorization
	authz := authz.NewJWTAuthz(option.JWTSigningKey(), nLogger)

	// create a new controller to process incoming requests
	basecontr := initializeBaseController(memoryStorage, option, nLogger, authz)

	// get a middleware for logging requests
	reqLog := middleware.NewReqLog(nLogger)

	// create router and mount routes
	r := chi.NewRouter()
	r.Use(reqLog.RequestLogger)
	r.Mount("/", basecontr.Route())

	// configure and start the server
	startServer(r, option.RunAddr())

}

func initializeKeeper(dataBaseDSN func() string, logger *logger.Logger, userUpdateInterval func() string) *bdkeeper.BDKeeper {
	if dataBaseDSN() == "" {
		return nil
	}

	return bdkeeper.NewBDKeeper(dataBaseDSN, logger, userUpdateInterval)
}

func initializeStorage(keeper storage.Keeper, logger *logger.Logger) *storage.MemoryStorage {
	if keeper == nil {
		return nil
	}

	return storage.NewMemoryStorage(keeper, logger)
}

func initializeBaseController(storage *storage.MemoryStorage, option *config.Options,
	logger *logger.Logger, authz *authz.JWTAuthz,
) *controllers.BaseController {
	return controllers.NewBaseController(storage, option, logger, authz)
}

func startServer(router chi.Router, address string) {
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

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalln(err)
	}
}
func (server *Server) Shutdown() {
	log.Printf("server stopped")

	const shutdownTimeout = 5 * time.Second
	ctxShutDown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

	defer cancel()

	if err := server.srv.Shutdown(ctxShutDown); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			//nolint:gocritic
			log.Fatalf("server Shutdown Failed:%s", err)
		}
	}

	log.Println("server exited properly")
}
