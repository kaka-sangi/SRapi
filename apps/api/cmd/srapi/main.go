package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srapi/srapi/apps/api/internal/app"
	"github.com/srapi/srapi/apps/api/internal/config"
	platformlogger "github.com/srapi/srapi/apps/api/internal/platform/logger"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the local process liveness endpoint")
	flag.Parse()

	logger := platformlogger.New()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if *healthcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := app.Healthcheck(ctx, cfg); err != nil {
			logger.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize app", "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("starting API", "address", cfg.Address(), "mode", cfg.Server.Mode, "version", cfg.Server.Version)
		if err := application.Serve(); err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("API stopped")
}
