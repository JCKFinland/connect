package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/pkg/logger"
)

func main() {

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log := logger.New(
		cfg.Log.Level,
		cfg.App.Env,
	)

	log.Info("Starting CONNECT Backend")

	db, err := database.Connect(cfg)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()

	if err := database.RunMigrations(log); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	router := api.NewRouter(log, db)

	serverAddr := fmt.Sprintf(":%s", cfg.App.Port)

	go func() {

		log.Info("HTTP server started",
			"address", serverAddr,
		)

		if err := router.Run(serverAddr); err != nil {
			log.Error(err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(
		quit,
		os.Interrupt,
		syscall.SIGTERM,
	)

	<-quit

	log.Info("Shutting down CONNECT Backend...")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	_ = ctx

	log.Info("Shutdown complete")
}