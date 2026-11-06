package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/httpapi"
	appRuntime "github.com/libredmm/librefanza/backend/internal/runtime"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	dataDir := envOr("LIBREFANZA_DATA_DIR", "./data")
	frontendDir := envOr("LIBREFANZA_FRONTEND_DIR", "../frontend/dist")
	store := config.NewStore(dataDir)
	provider := database.NewProvider(store)
	defer provider.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner := appRuntime.NewRunner(ctx, provider)
	defer runner.Close()
	go runner.Run()
	setupService := setup.NewService(store, setup.NewDatabaseBackend())
	defer setupService.Close()
	router := httpapi.NewRouter(setupService, provider)
	httpapi.RegisterTaskRoutes(router, provider, func(ctx context.Context) (httpapi.TaskService, error) {
		return runner.Manager(ctx)
	})
	httpapi.RegisterSPA(router, frontendDir)
	server := &http.Server{Addr: envOr("LIBREFANZA_LISTEN", "127.0.0.1:8080"), Handler: router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("服务监听 %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
