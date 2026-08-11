package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jobdock/jobdock/internal/builder"
	"github.com/jobdock/jobdock/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("component", "builder")
	cfg, err := config.LoadBuilder()
	if err != nil {
		logger.Error("invalid_configuration", "error", err)
		os.Exit(1)
	}
	service, err := builder.New(cfg, logger)
	if err != nil {
		logger.Error("initialize_builder", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = service.Run(ctx); err != nil {
		logger.Error("builder_stopped", "error", err)
		os.Exit(1)
	}
}
