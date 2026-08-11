package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
)

type Builder struct {
	config   config.Builder
	log      *slog.Logger
	executor Executor
	client   *client
	store    *filestore.Store
}

func New(cfg config.Builder, logger *slog.Logger) (*Builder, error) {
	return NewWithExecutor(cfg, logger, NewBuildKit(cfg))
}

func NewWithExecutor(cfg config.Builder, logger *slog.Logger, executor Executor) (*Builder, error) {
	for _, directory := range []string{cfg.StateDir, cfg.WorkspaceDir, filepath.Join(cfg.StateDir, "artifacts")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, err
		}
	}
	builderID, err := persistentBuilderID(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	files, err := filestore.New(cfg.WorkspaceDir, 1<<20, 1<<20, cfg.MaxSourceBytes)
	if err != nil {
		return nil, err
	}
	return &Builder{config: cfg, log: logger, executor: executor, client: newClient(cfg, builderID), store: files}, nil
}

func (b *Builder) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		work, err := b.client.next(ctx)
		if err != nil {
			b.log.Warn("build_poll_failed", "error", err)
		} else if work != nil {
			if err = b.persistWork(*work); err != nil {
				b.log.Error("persist_build_assignment_failed", "build_id", work.Build.ID, "error", err)
			} else {
				b.execute(ctx, *work)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(b.config.PollInterval):
		}
	}
	return nil
}

func (b *Builder) execute(parent context.Context, work domain.BuildWork) {
	buildCtx, cancel := context.WithTimeout(parent, b.config.BuildTimeout)
	defer cancel()
	cancelledByServer := make(chan struct{}, 1)
	go b.heartbeatLoop(buildCtx, work.Assignment.ID, cancel, cancelledByServer)
	logPath := filepath.Join(b.config.StateDir, "build.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		b.finish(parent, work, domain.BuildAssignmentFailed, "", err.Error())
		return
	}
	var logMu sync.Mutex
	writer := &lockedWriter{writer: logFile, mu: &logMu}
	uploadDone := make(chan struct{})
	go func() {
		defer close(uploadDone)
		b.uploadLogs(buildCtx, work.Assignment.ID, logFile, &logMu)
	}()
	projectDir, cleanup, err := b.prepareSource(buildCtx, work)
	if err == nil {
		defer cleanup()
		artifactPath := filepath.Join(b.config.StateDir, "artifacts", work.Build.ID+".oci.tar")
		_, _ = fmt.Fprintf(writer, "\nJobDock builder %s started %s build\n", b.client.builderID, work.Build.Mode)
		var digest string
		digest, err = b.executor.Build(buildCtx, work, projectDir, artifactPath, writer)
		if err == nil {
			_, _ = fmt.Fprintf(writer, "\nBuild completed with digest %s\n", digest)
			cancel()
			<-uploadDone
			_ = logFile.Close()
			b.finish(parent, work, domain.BuildAssignmentSucceeded, digest, "BuildKit completed the OCI build")
			return
		}
	}
	status, message := domain.BuildAssignmentFailed, err.Error()
	select {
	case <-cancelledByServer:
		status, message = domain.BuildAssignmentCancelled, "Build cancelled by user"
	default:
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			message = "Build exceeded the configured timeout of " + b.config.BuildTimeout.String()
		}
	}
	if status == domain.BuildAssignmentCancelled {
		_, _ = fmt.Fprintln(writer, "\nBuild cancelled by user")
	} else {
		_, _ = fmt.Fprintf(writer, "\nBuild failed: %s\n", message)
	}
	cancel()
	<-uploadDone
	_ = logFile.Close()
	b.finish(parent, work, status, "", message)
}

func (b *Builder) prepareSource(ctx context.Context, work domain.BuildWork) (string, func(), error) {
	buildRoot := filepath.Join(b.config.WorkspaceDir, "builds", work.Build.ID, "source")
	archivePath := filepath.Join(buildRoot, "source.archive")
	if err := os.MkdirAll(buildRoot, 0o700); err != nil {
		return "", nil, err
	}
	if _, err := os.Lstat(archivePath); errors.Is(err, os.ErrNotExist) {
		if err = b.client.downloadSource(ctx, work, buildRoot); err != nil {
			return "", nil, err
		}
	} else if err != nil {
		return "", nil, err
	}
	if err := verifyFile(archivePath, work.Build.Source.Size, work.Build.Source.SHA256); err != nil {
		return "", nil, err
	}
	return b.store.PrepareBuildSource(work.Build.ID, work.Build.Source.Filename)
}

func (b *Builder) heartbeatLoop(ctx context.Context, assignmentID string, cancel context.CancelFunc, cancelled chan<- struct{}) {
	interval := b.config.Lease / 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			assignment, err := b.client.heartbeat(ctx, assignmentID)
			if err != nil {
				b.log.Warn("build_heartbeat_failed", "assignment_id", assignmentID, "error", err)
				continue
			}
			if assignment.CancelRequested {
				select {
				case cancelled <- struct{}{}:
				default:
				}
				cancel()
				return
			}
		}
	}
}

func (b *Builder) uploadLogs(ctx context.Context, assignmentID string, file *os.File, mu *sync.Mutex) {
	serverOffset, err := b.client.discoverLogOffset(ctx, assignmentID)
	if err != nil {
		b.log.Warn("build_log_offset_failed", "error", err)
		serverOffset = 0
	}
	var localOffset int64
	buffer := make([]byte, 64<<10)
	for {
		mu.Lock()
		_, _ = file.Seek(localOffset, io.SeekStart)
		count, readErr := file.Read(buffer)
		mu.Unlock()
		if count > 0 {
			next, mismatch, uploadErr := b.client.uploadLog(context.WithoutCancel(ctx), assignmentID, serverOffset, buffer[:count])
			if uploadErr != nil {
				b.log.Warn("build_log_upload_failed", "error", uploadErr)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if mismatch {
				serverOffset = next
				continue
			}
			serverOffset, localOffset = next, localOffset+int64(count)
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			b.log.Warn("build_log_read_failed", "error", readErr)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (b *Builder) finish(ctx context.Context, work domain.BuildWork, status domain.BuildAssignmentStatus, digest, message string) {
	for ctx.Err() == nil {
		requestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		err := b.client.complete(requestCtx, work.Assignment.ID, status, digest, message)
		cancel()
		if err == nil {
			_ = os.Remove(filepath.Join(b.config.StateDir, "current.json"))
			b.log.Info("build_completed", "build_id", work.Build.ID, "status", status, "digest", digest)
			return
		}
		b.log.Warn("build_completion_report_failed", "build_id", work.Build.ID, "error", err)
		time.Sleep(b.config.PollInterval)
	}
}

func (b *Builder) persistWork(work domain.BuildWork) error {
	data, err := json.MarshalIndent(work, "", "  ")
	if err != nil {
		return err
	}
	temporary := filepath.Join(b.config.StateDir, "current.json.tmp")
	if err = os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(b.config.StateDir, "current.json"))
}

type lockedWriter struct {
	writer io.Writer
	mu     *sync.Mutex
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

func persistentBuilderID(stateDir string) (string, error) {
	path := filepath.Join(stateDir, "builder-id")
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return string(data), nil
	}
	id := ids.New()
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func verifyFile(path string, expectedSize int64, expectedDigest string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil {
		return err
	}
	if written != expectedSize || hex.EncodeToString(hash.Sum(nil)) != expectedDigest {
		return errors.New("persisted source does not match its immutable identity")
	}
	return nil
}
