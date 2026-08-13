package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jobdock/jobdock/internal/domain"
)

func (a *Agent) materializeInputs(ctx context.Context, jobID string, manifest []domain.InputFile, root string) error {
	if !workspaceChild(a.config.WorkspaceDir, root) {
		return errors.New("input directory escapes the configured workspace")
	}
	if err := removeMaterializedInputs(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = removeMaterializedInputs(root)
		}
	}()
	for _, item := range manifest {
		destination, err := materializedPath(root, item.Path)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		endpoint := "/api/v1/agent/jobs/" + jobID + "/inputs/" + strings.ReplaceAll(url.PathEscape(item.Path), "%2F", "/")
		if err = a.downloadInput(ctx, endpoint, destination, item); err != nil {
			return fmt.Errorf("materialize input %q: %w", item.Path, err)
		}
	}
	var directories []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := os.Chmod(directories[index], 0o555); err != nil {
			return err
		}
	}
	succeeded = true
	return nil
}

func removeMaterializedInputs(root string) error {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func (a *Agent) downloadInput(ctx context.Context, endpoint, destination string, expected domain.InputFile) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.ServerURL+endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.credential)
	request.Header.Set("X-JobDock-Protocol-Version", "1")
	response, err := a.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return readAPIError(response)
	}
	if digest := response.Header.Get("X-JobDock-Content-SHA256"); digest == "" || digest != expected.SHA256 {
		return errors.New("server digest does not match the assignment manifest")
	}
	if response.ContentLength != expected.Size {
		return fmt.Errorf("server size does not match the assignment manifest: expected %d bytes, received %d", expected.Size, response.ContentLength)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".jobdock-input-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, expected.Size+1))
	if copyErr == nil && written != expected.Size {
		copyErr = fmt.Errorf("size mismatch: expected %d bytes, received %d", expected.Size, written)
	}
	if copyErr == nil && hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		copyErr = errors.New("SHA-256 mismatch")
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
	if err = os.Chmod(temporaryPath, 0o444); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func materializedPath(root, relative string) (string, error) {
	normalized := strings.ReplaceAll(relative, "\\", "/")
	clean := filepath.Clean(filepath.FromSlash(normalized))
	if normalized == "" || filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid input path %q", relative)
	}
	destination := filepath.Join(root, clean)
	resolved, err := filepath.Rel(root, destination)
	if err != nil || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("input path %q escapes the input directory", relative)
	}
	return destination, nil
}

func workspaceChild(workspace, candidate string) bool {
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(workspace, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
