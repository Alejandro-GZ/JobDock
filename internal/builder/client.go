package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

type client struct {
	config    config.Builder
	builderID string
	http      *http.Client
}

func newClient(cfg config.Builder, builderID string) *client {
	return &client{config: cfg, builderID: builderID, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *client) next(ctx context.Context) (*domain.BuildWork, error) {
	request, err := c.request(ctx, http.MethodGet, "/api/v1/builder/assignments/next", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, readAPIError(response)
	}
	var work domain.BuildWork
	if err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&work); err != nil {
		return nil, err
	}
	return &work, nil
}

func (c *client) heartbeat(ctx context.Context, assignmentID string) (domain.BuildAssignment, error) {
	var assignment domain.BuildAssignment
	err := c.json(ctx, http.MethodPost, "/api/v1/builder/assignments/"+assignmentID+"/heartbeat", struct{}{}, &assignment)
	return assignment, err
}

func (c *client) downloadSource(ctx context.Context, work domain.BuildWork, destination string) error {
	request, err := c.request(ctx, http.MethodGet, "/api/v1/builder/assignments/"+work.Assignment.ID+"/source", nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return readAPIError(response)
	}
	if digest := response.Header.Get("X-JobDock-Source-SHA256"); digest != work.Build.Source.SHA256 {
		return errors.New("source digest header does not match the persisted build")
	}
	temporary, err := os.CreateTemp(destination, ".source-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, c.config.MaxSourceBytes+1))
	if copyErr == nil && written != work.Build.Source.Size {
		copyErr = fmt.Errorf("source size mismatch: expected %d bytes, received %d", work.Build.Source.Size, written)
	}
	if copyErr == nil && hex.EncodeToString(hash.Sum(nil)) != work.Build.Source.SHA256 {
		copyErr = errors.New("source SHA-256 mismatch")
	}
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	return os.Rename(temporaryPath, destination+"/source.archive")
}

func (c *client) discoverLogOffset(ctx context.Context, assignmentID string) (int64, error) {
	next, mismatch, err := c.uploadLog(ctx, assignmentID, 0, nil)
	if mismatch || err == nil {
		return next, err
	}
	return 0, err
}

func (c *client) uploadLog(ctx context.Context, assignmentID string, offset int64, data []byte) (next int64, mismatch bool, err error) {
	request, err := c.request(ctx, http.MethodPut, "/api/v1/builder/assignments/"+assignmentID+"/logs", bytes.NewReader(data))
	if err != nil {
		return offset, false, err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-JobDock-Upload-Offset", strconv.FormatInt(offset, 10))
	response, err := c.http.Do(request)
	if err != nil {
		return offset, false, err
	}
	defer response.Body.Close()
	next = offset + int64(len(data))
	if value := response.Header.Get("X-JobDock-Next-Offset"); value != "" {
		next, _ = strconv.ParseInt(value, 10, 64)
	}
	if response.StatusCode == http.StatusConflict {
		return next, true, nil
	}
	if response.StatusCode != http.StatusNoContent {
		return offset, false, readAPIError(response)
	}
	return next, false, nil
}

func (c *client) complete(ctx context.Context, assignmentID string, status domain.BuildAssignmentStatus, digest, message string) error {
	return c.json(ctx, http.MethodPost, "/api/v1/builder/assignments/"+assignmentID+"/complete", map[string]any{"status": status, "digest": digest, "message": message}, nil)
}

func (c *client) uploadArtifact(ctx context.Context, assignmentID, artifactPath, digest, runtimeImage string) (domain.ManagedArtifact, error) {
	file, err := os.Open(artifactPath)
	if err != nil {
		return domain.ManagedArtifact{}, err
	}
	defer file.Close()
	request, err := c.request(ctx, http.MethodPut, "/api/v1/builder/assignments/"+assignmentID+"/artifact", file)
	if err != nil {
		return domain.ManagedArtifact{}, err
	}
	request.Header.Set("Content-Type", domain.ManagedImageMediaType)
	request.Header.Set("X-JobDock-OCI-Digest", digest)
	request.Header.Set("X-JobDock-Runtime-Image", runtimeImage)
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return domain.ManagedArtifact{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return domain.ManagedArtifact{}, readAPIError(response)
	}
	var artifact domain.ManagedArtifact
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&artifact); err != nil {
		return artifact, err
	}
	if artifact.Digest != digest {
		return artifact, errors.New("server confirmed a different OCI digest")
	}
	return artifact, nil
}

func (c *client) json(ctx context.Context, method, path string, body, destination any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := c.request(ctx, method, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return readAPIError(response)
	}
	if destination != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(destination)
	}
	return nil
}

func (c *client) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.config.ServerURL+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.config.Token)
	request.Header.Set("X-JobDock-Protocol-Version", "1")
	request.Header.Set("X-JobDock-Builder-ID", c.builderID)
	return request, nil
}

func readAPIError(response *http.Response) error {
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&problem)
	if problem.Detail == "" {
		problem.Detail = response.Status
	}
	return fmt.Errorf("JobDock API %s: %s", strings.TrimSpace(problem.Code), problem.Detail)
}
