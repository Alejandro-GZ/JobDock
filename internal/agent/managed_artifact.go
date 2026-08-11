package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (a *Agent) loadManagedArtifact(ctx context.Context, record *runtimeAssignment, workspace string) (string, string, error) {
	artifact := record.ManagedArtifact
	if artifact == nil {
		return "", "", errors.New("managed artifact metadata is absent from the assignment")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.ServerURL+"/api/v1/agent/assignments/"+record.ID+"/artifact", nil)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Authorization", "Bearer "+a.credential)
	request.Header.Set("X-JobDock-Protocol-Version", "1")
	artifactClient := *a.http
	artifactClient.Timeout = 0

	response, err := artifactClient.Do(request)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", readAPIError(response)
	}
	if response.Header.Get("X-JobDock-Content-SHA256") != artifact.SHA256 || response.Header.Get("X-JobDock-OCI-Digest") != artifact.Digest {
		return "", "", errors.New("managed artifact response does not match the assignment")
	}
	file, err := os.CreateTemp(workspace, ".jobdock-image-*.tar")
	if err != nil {
		return "", "", err
	}
	path := file.Name()
	defer os.Remove(path)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, artifact.Size+1))
	if copyErr == nil && written != artifact.Size {
		copyErr = fmt.Errorf("managed artifact size mismatch: expected %d, received %d", artifact.Size, written)
	}
	if copyErr == nil && hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		copyErr = errors.New("managed artifact SHA-256 mismatch")
	}
	if closeErr := file.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return "", "", copyErr
	}
	archive, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	loadErr := a.docker.Load(ctx, archive)
	closeErr := archive.Close()
	if loadErr != nil {
		return "", "", loadErr
	}
	if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
		return "", "", closeErr
	}
	if a.docker.ImageDigest(ctx, artifact.RuntimeImage) == "" {
		return "", "", errors.New("Docker did not load the managed artifact runtime image")
	}
	return artifact.RuntimeImage, artifact.Digest, nil
}
