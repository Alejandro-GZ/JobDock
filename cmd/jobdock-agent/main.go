package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jobdock/jobdock/internal/agent"
	"github.com/jobdock/jobdock/internal/config"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("jobdock-agent %s\n", agent.Version())
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("component", "agent")
	cfg, err := config.LoadAgent()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = agent.New(cfg, logger).Run(ctx); err != nil {
		logger.Error("agent_stopped", "error", err)
		os.Exit(1)
	}
}
