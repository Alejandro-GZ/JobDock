package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/httpapi"
	"github.com/jobdock/jobdock/internal/scheduler"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		response, err := http.Get("http://127.0.0.1:8080/health/live")
		if err != nil || response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).With("component", "server", "version", version)
	cfg, err := config.LoadServer()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if err = os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}
	if len(cfg.MasterKey) == 0 {
		cfg.MasterKey, err = loadOrCreateMasterKey(filepath.Join(cfg.DataDir, "master.key"))
		if err != nil {
			logger.Error("load master key", "error", err)
			os.Exit(1)
		}
	}
	box, err := secretbox.New(cfg.MasterKey)
	if err != nil {
		logger.Error("initialize secret encryption", "error", err)
		os.Exit(1)
	}
	repository, err := store.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	files, err := filestore.New(cfg.DataDir, cfg.MaxLogBytes, cfg.MaxOutputBytes, cfg.MaxInputBytes)
	if err != nil {
		logger.Error("open file store", "error", err)
		os.Exit(1)
	}
	api := httpapi.New(cfg, repository, files, box, logger)
	if err = api.BootstrapAdmin(context.Background()); err != nil {
		logger.Error("bootstrap administrator", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go scheduler.New(repository, box).Run(ctx)
	go monitorNodes(ctx, repository, cfg)
	go maintainTelemetry(ctx, repository, cfg, logger)
	go maintainManagedArtifacts(ctx, repository, files, cfg, logger)
	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		logger.Info("server_started", "address", cfg.ListenAddr, "public_url", cfg.PublicURL)
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("server_failed", "error", serveErr)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err = httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown_failed", "error", err)
	}
}

func maintainManagedArtifacts(ctx context.Context, repository *store.Store, files *filestore.Store, cfg config.Server, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		artifacts, err := repository.GarbageCollectManagedArtifacts(ctx, time.Now().UTC().Add(-cfg.BuildArtifactRetention))
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("managed_artifact_gc_failed", "error", err)
		}
		for _, artifact := range artifacts {
			if deleteErr := files.DeleteBuildArtifact(artifact.BuildID); deleteErr != nil {
				logger.Warn("managed_artifact_file_gc_failed", "build_id", artifact.BuildID, "error", deleteErr)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func maintainTelemetry(ctx context.Context, repository *store.Store, cfg config.Server, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		if err := repository.MaintainResourceTelemetry(ctx, time.Now().UTC(), cfg.TelemetryRawRetention, cfg.TelemetryRetention); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("telemetry_maintenance_failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func monitorNodes(ctx context.Context, repository *store.Store, cfg config.Server) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		_ = repository.MarkStaleNodes(ctx, time.Now().UTC().Add(-cfg.HeartbeatOfflineAfter), time.Now().UTC().Add(-cfg.JobLostAfter))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func loadOrCreateMasterKey(path string) ([]byte, error) {
	contents, err := os.ReadFile(path)
	if err == nil {
		return base64.StdEncoding.DecodeString(string(contents))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err = os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}
