package main

import (
	"context"
	"github.com/gin-contrib/cors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"thesis/internal/config"
	"thesis/internal/server"
	"thesis/internal/storage"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Init config
	appConfig := config.MustLoad()

	// Init logger
	log := setupLogger()
	log.Info("starting app")
	log.Debug("debug logging enabled")

	// Init database connection
	var databaseC storage.DatabaseClient

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := databaseC.Connect(ctx, appConfig.Storage)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	log.Info("connected to database")

	// Init router
	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "https://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// TODO: routes

	// Serve
	startServer := &server.Server{}
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := startServer.RunServer(appConfig.Server.Address, router); err != nil {
			panic(err)
		}
	}()

	log.Info("server started")

	// If got signal stopping server
	<-done

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	log.Info("shutting down server")
	if err := startServer.Shutdown(shutdownCtx); err != nil {
		panic(err)
	}

	log.Info("server stopped")
}

func setupLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}
