package filestore

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxBuildSourceEntries = 10000

var ErrOffsetMismatch = errors.New("upload offset does not match stored size")
var ErrLimitExceeded = errors.New("storage limit exceeded")

type Store struct {
	root           string
	maxLogBytes    int64
	maxOutputBytes int64
	maxInputBytes  int64
}

func New(root string, maxLogBytes, maxOutputBytes, maxInputBytes int64) (*Store, error) {
	for _, directory := range []string{"jobs", "builds"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o750); err != nil {
			return nil, err
		}
	}
	return &Store{root: root, maxLogBytes: maxLogBytes, maxOutputBytes: maxOutputBytes, maxInputBytes: maxInputBytes}, nil
}

type InputMetadata struct {
	Path   string
	Size   int64
	SHA256 string
}

type BuildSourceMetadata struct {
	Filename string
	Size     int64
	SHA256   string
}

type BuildArtifactMetadata struct {
	Size    int64
	SHA256  string
	Created bool
}

// StoreBuildArtifact atomically persists one immutable Docker-compatible
// image archive. Repeated identical publications are idempotent.
func (s *Store) StoreBuildArtifact(buildID string, source io.Reader, limit int64) (BuildArtifactMetadata, error) {
	if !safeSegment(buildID) || limit <= 0 {
		return BuildArtifactMetadata{}, errors.New("invalid build artifact request")
	}
	dir := filepath.Join(s.root, "builds", buildID, "artifact")
	destination := filepath.Join(dir, "image.tar")
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return BuildArtifactMetadata{}, errors.New("build artifact is not a regular file")
		}
		file, openErr := os.Open(destination)
		if openErr != nil {
			return BuildArtifactMetadata{}, openErr
		}
		defer file.Close()
		hash := sha256.New()
		size, hashErr := io.Copy(hash, file)
		return BuildArtifactMetadata{Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)), Created: false}, hashErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return BuildArtifactMetadata{}, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return BuildArtifactMetadata{}, err
	}
	temporary, err := os.CreateTemp(dir, ".jobdock-artifact-*")
	if err != nil {
		return BuildArtifactMetadata{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, limit+1))
	if copyErr == nil && written > limit {
		copyErr = ErrLimitExceeded
	}
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return BuildArtifactMetadata{}, copyErr
	}
	if err = os.Chmod(temporaryPath, 0o440); err != nil {
		return BuildArtifactMetadata{}, err
	}
	if err = os.Rename(temporaryPath, destination); err != nil {
		return BuildArtifactMetadata{}, err
	}
	return BuildArtifactMetadata{Size: written, SHA256: hex.EncodeToString(hash.Sum(nil)), Created: true}, nil
}

func (s *Store) OpenBuildArtifact(buildID string) (*os.File, error) {
	if !safeSegment(buildID) {
		return nil, errors.New("invalid build ID")
	}
	path := filepath.Join(s.root, "builds", buildID, "artifact", "image.tar")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("build artifact is not a regular file")
	}
	return os.Open(path)
}

func (s *Store) DeleteBuildArtifact(buildID string) error {
	if !safeSegment(buildID) {
		return errors.New("invalid build ID")
	}
	return os.RemoveAll(filepath.Join(s.root, "builds", buildID, "artifact"))
}

// StoreBuildSource persists the exact uploaded source bytes once. The digest
// identifies a reproducible source generation; existing sources are immutable.
func (s *Store) StoreBuildSource(buildID, filename string, source io.Reader) (BuildSourceMetadata, error) {
	if !safeSegment(buildID) {
		return BuildSourceMetadata{}, errors.New("invalid build ID")
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." || len(filename) > 255 {
		return BuildSourceMetadata{}, errors.New("invalid source filename")
	}
	dir := filepath.Join(s.root, "builds", buildID, "source")
	destination := filepath.Join(dir, "source.archive")
	if _, err := os.Lstat(destination); err == nil {
		return BuildSourceMetadata{}, errors.New("build source is immutable")
	} else if !errors.Is(err, os.ErrNotExist) {
		return BuildSourceMetadata{}, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return BuildSourceMetadata{}, err
	}
	temporary, err := os.CreateTemp(dir, ".jobdock-source-*")
	if err != nil {
		return BuildSourceMetadata{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, s.maxInputBytes+1))
	if copyErr == nil && written > s.maxInputBytes {
		copyErr = ErrLimitExceeded
	}
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return BuildSourceMetadata{}, copyErr
	}
	if err = os.Chmod(temporaryPath, 0o440); err != nil {
		return BuildSourceMetadata{}, err
	}
	if err = os.Rename(temporaryPath, destination); err != nil {
		return BuildSourceMetadata{}, err
	}
	return BuildSourceMetadata{Filename: filename, Size: written, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *Store) DeleteBuild(buildID string) error {
	if !safeSegment(buildID) {
		return errors.New("invalid build ID")
	}
	return os.RemoveAll(filepath.Join(s.root, "builds", buildID))
}

func (s *Store) OpenBuildSource(buildID string) (*os.File, error) {
	if !safeSegment(buildID) {
		return nil, errors.New("invalid build ID")
	}
	path := filepath.Join(s.root, "builds", buildID, "source", "source.archive")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("build source is not a regular file")
	}
	return os.Open(path)
}

// PrepareBuildSource expands a supported source archive into a temporary,
// path-confined workspace. Callers must invoke cleanup when analysis ends.
func (s *Store) PrepareBuildSource(buildID, filename string) (projectDir string, cleanup func(), err error) {
	if !safeSegment(buildID) {
		return "", nil, errors.New("invalid build ID")
	}
	buildDir := filepath.Join(s.root, "builds", buildID)
	workspace, err := os.MkdirTemp(buildDir, ".analysis-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(workspace) }
	fail := func(cause error) (string, func(), error) {
		cleanup()
		return "", nil, cause
	}
	sourcePath := filepath.Join(buildDir, "source", "source.archive")
	lowerName := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lowerName, ".zip"):
		if err = s.extractBuildZip(sourcePath, workspace); err != nil {
			return fail(err)
		}
	case strings.HasSuffix(lowerName, ".tar.gz"), strings.HasSuffix(lowerName, ".tgz"):
		if err = s.extractBuildTarGzip(sourcePath, workspace); err != nil {
			return fail(err)
		}
	default:
		return fail(errors.New("unsupported source archive; upload a .zip, .tar.gz, or .tgz project"))
	}
	projectDir, err = singleProjectRoot(workspace)
	if err != nil {
		return fail(err)
	}
	return projectDir, cleanup, nil
}

func (s *Store) extractBuildZip(sourcePath, destination string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("invalid ZIP project archive: %w", err)
	}
	if len(archive.File) == 0 || len(archive.File) > maxBuildSourceEntries {
		return errors.New("project archive must contain between 1 and 10000 entries")
	}
	var expanded int64
	for _, entry := range archive.File {
		if entry.Name == "." || entry.Name == "./" {
			continue
		}
		clean, cleanErr := safeRelativePath(entry.Name)
		if cleanErr != nil {
			return fmt.Errorf("unsafe project archive path %q", entry.Name)
		}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return fmt.Errorf("project archive entry %q is not a regular file or directory", entry.Name)
		}
		target := filepath.Join(destination, clean)
		if !within(destination, target) {
			return fmt.Errorf("project archive path %q escapes its workspace", entry.Name)
		}
		if mode.IsDir() {
			if err = os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			return openErr
		}
		expanded, err = writeBuildSourceFile(destination, target, mode, reader, expanded, s.maxInputBytes)
		reader.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) extractBuildTarGzip(sourcePath, destination string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("invalid gzip project archive: %w", err)
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	var expanded int64
	entries := 0
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("invalid tar project archive: %w", nextErr)
		}
		if header.Name == "." || header.Name == "./" {
			continue
		}
		entries++
		if entries > maxBuildSourceEntries {
			return errors.New("project archive contains more than 10000 entries")
		}
		clean, cleanErr := safeRelativePath(header.Name)
		if cleanErr != nil {
			return fmt.Errorf("unsafe project archive path %q", header.Name)
		}
		target := filepath.Join(destination, clean)
		if !within(destination, target) {
			return fmt.Errorf("project archive path %q escapes its workspace", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			expanded, err = writeBuildSourceFile(destination, target, fs.FileMode(header.Mode), io.LimitReader(archive, header.Size), expanded, s.maxInputBytes)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("project archive entry %q is not a regular file or directory", header.Name)
		}
	}
	if entries == 0 {
		return errors.New("project archive is empty")
	}
	return nil
}

func writeBuildSourceFile(root, target string, mode fs.FileMode, source io.Reader, expanded, limit int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return expanded, err
	}
	if _, err := os.Lstat(target); err == nil {
		return expanded, errors.New("project archive contains duplicate paths")
	} else if !errors.Is(err, os.ErrNotExist) {
		return expanded, err
	}
	fileMode := fs.FileMode(0o640)
	if mode&0o111 != 0 {
		fileMode = 0o750
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return expanded, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, limit-expanded+1))
	if closeErr := file.Close(); copyErr == nil {
		copyErr = closeErr
	}
	expanded += written
	if copyErr != nil {
		return expanded, copyErr
	}
	if expanded > limit {
		return expanded, ErrLimitExceeded
	}
	if !within(root, target) {
		return expanded, errors.New("project archive path escapes its workspace")
	}
	return expanded, nil
}

func singleProjectRoot(workspace string) (string, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", errors.New("project archive is empty")
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(workspace, entries[0].Name()), nil
	}
	return workspace, nil
}

func (s *Store) AppendBuildLog(buildID string, offset int64, source io.Reader) (int64, error) {
	if !safeSegment(buildID) {
		return offset, errors.New("invalid build ID")
	}
	dir := filepath.Join(s.root, "builds", buildID, "logs")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return offset, err
	}
	path := filepath.Join(dir, "build.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return offset, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return offset, err
	}
	if info.Size() != offset {
		return info.Size(), ErrOffsetMismatch
	}
	remaining := s.maxLogBytes - info.Size()
	if remaining <= 0 {
		return offset, ErrLimitExceeded
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	written, err := io.Copy(file, io.LimitReader(source, remaining+1))
	if err != nil {
		return offset + written, err
	}
	if written > remaining {
		_ = file.Truncate(offset + remaining)
		return offset + remaining, ErrLimitExceeded
	}
	return offset + written, file.Sync()
}

func (s *Store) ReadBuildLog(buildID string, offset int64, destination io.Writer) (int64, error) {
	if !safeSegment(buildID) || offset < 0 {
		return offset, errors.New("invalid build log request")
	}
	file, err := os.Open(filepath.Join(s.root, "builds", buildID, "logs", "build.log"))
	if errors.Is(err, os.ErrNotExist) {
		return offset, nil
	}
	if err != nil {
		return offset, err
	}
	defer file.Close()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	written, err := io.Copy(destination, file)
	return offset + written, err
}

func (s *Store) ReadBuildLogChunk(buildID string, offset, limit int64) ([]byte, int64, error) {
	if !safeSegment(buildID) || offset < 0 || limit <= 0 {
		return nil, offset, errors.New("invalid build log request")
	}
	file, err := os.Open(filepath.Join(s.root, "builds", buildID, "logs", "build.log"))
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, offset, nil
	}
	if err != nil {
		return nil, offset, err
	}
	defer file.Close()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	return data, offset + int64(len(data)), err
}

func (s *Store) StoreInput(jobID, relativePath string, source io.Reader) (InputMetadata, error) {
	clean, err := safeRelativePath(relativePath)
	if err != nil {
		return InputMetadata{}, err
	}
	jobDir, err := s.JobDir(jobID)
	if err != nil {
		return InputMetadata{}, err
	}
	root := filepath.Join(jobDir, "inputs")
	destination := filepath.Join(root, clean)
	if !within(root, destination) {
		return InputMetadata{}, errors.New("input path escapes job directory")
	}
	if _, err = os.Lstat(destination); err == nil {
		return InputMetadata{}, errors.New("duplicate input path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return InputMetadata{}, err
	}
	used, err := directorySize(root)
	if err != nil {
		return InputMetadata{}, err
	}
	remaining := s.maxInputBytes - used
	if remaining < 0 {
		return InputMetadata{}, ErrLimitExceeded
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return InputMetadata{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".jobdock-input-*")
	if err != nil {
		return InputMetadata{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, remaining+1))
	if copyErr == nil && written > remaining {
		copyErr = ErrLimitExceeded
	}
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return InputMetadata{}, copyErr
	}
	if err = os.Chmod(temporaryPath, 0o440); err != nil {
		return InputMetadata{}, err
	}
	if err = os.Rename(temporaryPath, destination); err != nil {
		return InputMetadata{}, err
	}
	return InputMetadata{Path: filepath.ToSlash(clean), Size: written, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *Store) OpenInput(jobID, relativePath string) (*os.File, error) {
	clean, err := safeRelativePath(relativePath)
	if err != nil {
		return nil, err
	}
	jobDir, err := s.JobDir(jobID)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(jobDir, "inputs")
	path := filepath.Join(root, clean)
	if !within(root, path) {
		return nil, errors.New("input path escapes job directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("input is not a regular file")
	}
	return os.Open(path)
}

func (s *Store) JobDir(jobID string) (string, error) {
	if !safeSegment(jobID) {
		return "", errors.New("invalid job ID")
	}
	return filepath.Join(s.root, "jobs", jobID), nil
}

func (s *Store) AppendLog(jobID, stream string, offset int64, source io.Reader) (int64, error) {
	return s.appendLog(jobID, "", stream, offset, source)
}

func (s *Store) AppendAttemptLog(jobID, attemptID, stream string, offset int64, source io.Reader) (int64, error) {
	return s.appendLog(jobID, attemptID, stream, offset, source)
}

func (s *Store) appendLog(jobID, attemptID, stream string, offset int64, source io.Reader) (int64, error) {
	if stream != "stdout" && stream != "stderr" {
		return 0, errors.New("invalid log stream")
	}
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o750); err != nil {
		return 0, err
	}
	path := filepath.Join(dir, "logs", stream+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size() != offset {
		return info.Size(), ErrOffsetMismatch
	}
	used, err := directorySize(filepath.Join(dir, "logs"))
	if err != nil {
		return offset, err
	}
	remaining := s.maxLogBytes - used
	if remaining <= 0 {
		return offset, ErrLimitExceeded
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	written, err := io.Copy(file, io.LimitReader(source, remaining+1))
	newOffset := offset + written
	if err != nil {
		return newOffset, err
	}
	if written > remaining {
		_ = file.Truncate(offset + remaining)
		return offset + remaining, ErrLimitExceeded
	}
	return newOffset, file.Sync()
}

func (s *Store) ReadLog(jobID, stream string, offset int64, destination io.Writer) (int64, error) {
	return s.readLog(jobID, "", stream, offset, destination)
}

func (s *Store) ReadAttemptLog(jobID, attemptID, stream string, offset int64, destination io.Writer) (int64, error) {
	return s.readLog(jobID, attemptID, stream, offset, destination)
}

func (s *Store) readLog(jobID, attemptID, stream string, offset int64, destination io.Writer) (int64, error) {
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return offset, err
	}
	file, err := os.Open(filepath.Join(dir, "logs", stream+".log"))
	if errors.Is(err, os.ErrNotExist) {
		return offset, nil
	}
	if err != nil {
		return offset, err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	written, err := io.Copy(destination, file)
	return offset + written, err
}

// ReadLogChunk reads at most limit bytes starting at offset. It is used by live
// consumers so each refresh is proportional to newly appended data.
func (s *Store) ReadLogChunk(jobID, stream string, offset, limit int64) ([]byte, int64, error) {
	return s.readLogChunk(jobID, "", stream, offset, limit)
}

func (s *Store) ReadAttemptLogChunk(jobID, attemptID, stream string, offset, limit int64) ([]byte, int64, error) {
	return s.readLogChunk(jobID, attemptID, stream, offset, limit)
}

func (s *Store) readLogChunk(jobID, attemptID, stream string, offset, limit int64) ([]byte, int64, error) {
	if limit <= 0 {
		return nil, offset, errors.New("log chunk limit must be positive")
	}
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return nil, offset, err
	}
	if stream != "stdout" && stream != "stderr" {
		return nil, offset, errors.New("invalid log stream")
	}
	file, err := os.Open(filepath.Join(dir, "logs", stream+".log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, offset, nil
	}
	if err != nil {
		return nil, offset, err
	}
	defer file.Close()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	return data, offset + int64(len(data)), err
}

func (s *Store) LogSize(jobID, stream string) (int64, error) {
	return s.logSize(jobID, "", stream)
}

func (s *Store) AttemptLogSize(jobID, attemptID, stream string) (int64, error) {
	return s.logSize(jobID, attemptID, stream)
}

func (s *Store) logSize(jobID, attemptID, stream string) (int64, error) {
	if stream != "stdout" && stream != "stderr" {
		return 0, errors.New("invalid log stream")
	}
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(filepath.Join(dir, "logs", stream+".log"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *Store) AppendOutput(jobID, relativePath string, offset int64, source io.Reader) (int64, error) {
	return s.appendOutput(jobID, "", relativePath, offset, source)
}

func (s *Store) AppendAttemptOutput(jobID, attemptID, relativePath string, offset int64, source io.Reader) (int64, error) {
	return s.appendOutput(jobID, attemptID, relativePath, offset, source)
}

func (s *Store) OpenAttemptOutput(jobID, attemptID, relativePath string) (*os.File, error) {
	clean, err := safeRelativePath(relativePath)
	if err != nil {
		return nil, err
	}
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(dir, "output")
	path := filepath.Join(root, clean)
	if !within(root, path) {
		return nil, errors.New("output path escapes job directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("output is not a regular file")
	}
	return os.Open(path)
}

func (s *Store) appendOutput(jobID, attemptID, relativePath string, offset int64, source io.Reader) (int64, error) {
	clean, err := safeRelativePath(relativePath)
	if err != nil {
		return 0, err
	}
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return 0, err
	}
	outputRoot := filepath.Join(dir, "output")
	path := filepath.Join(outputRoot, clean)
	if !within(outputRoot, path) {
		return 0, errors.New("output path escapes job directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("output destination is not a regular file")
	}
	if info.Size() != offset {
		return info.Size(), ErrOffsetMismatch
	}
	used, err := directorySize(outputRoot)
	if err != nil {
		return offset, err
	}
	remaining := s.maxOutputBytes - used
	if remaining <= 0 {
		return offset, ErrLimitExceeded
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	written, err := io.Copy(file, io.LimitReader(source, remaining+1))
	newOffset := offset + written
	if err != nil {
		return newOffset, err
	}
	if written > remaining {
		_ = file.Truncate(offset + remaining)
		return offset + remaining, ErrLimitExceeded
	}
	return newOffset, file.Sync()
}

// AppendCheckpoint writes to an immutable staging generation. Offset conflicts
// return the durable server offset so an agent can resume after any disconnect.
func (s *Store) AppendCheckpoint(jobID, syncID, relativePath string, offset int64, source io.Reader) (int64, error) {
	if !safeSegment(syncID) {
		return 0, errors.New("invalid checkpoint sync ID")
	}
	clean, err := safeRelativePath(relativePath)
	if err != nil {
		return 0, err
	}
	jobDir, err := s.JobDir(jobID)
	if err != nil {
		return 0, err
	}
	root := filepath.Join(jobDir, "checkpoint-staging", syncID)
	path := filepath.Join(root, clean)
	if !within(root, path) {
		return 0, errors.New("checkpoint path escapes staging directory")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("checkpoint destination is not a regular file")
	}
	if info.Size() != offset {
		return info.Size(), ErrOffsetMismatch
	}
	used, err := directorySize(filepath.Join(jobDir, "output"))
	if err == nil {
		var checkpointBytes int64
		checkpointBytes, err = directorySize(filepath.Join(jobDir, "checkpoints"))
		used += checkpointBytes
	}
	if err == nil {
		var stagingBytes int64
		stagingBytes, err = directorySize(filepath.Join(jobDir, "checkpoint-staging"))
		used += stagingBytes
	}
	if err != nil {
		return offset, err
	}
	remaining := s.maxOutputBytes - used
	if remaining <= 0 {
		return offset, ErrLimitExceeded
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	written, err := io.Copy(file, io.LimitReader(source, remaining+1))
	next := offset + written
	if err != nil {
		return next, err
	}
	if written > remaining {
		_ = file.Truncate(offset + remaining)
		return offset + remaining, ErrLimitExceeded
	}
	return next, file.Sync()
}

// ConfirmCheckpoint atomically promotes a fully acknowledged staging
// directory. Older generations remain untouched, including when a newer node
// disappears mid-upload.
func (s *Store) ConfirmCheckpoint(jobID, syncID string, files map[string]int64) error {
	if !safeSegment(syncID) {
		return errors.New("invalid checkpoint sync ID")
	}
	jobDir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	staging := filepath.Join(jobDir, "checkpoint-staging", syncID)
	destination := filepath.Join(jobDir, "checkpoints", syncID)
	if _, err = os.Stat(destination); err == nil {
		return nil
	}
	if len(files) == 0 {
		if err = os.MkdirAll(staging, 0o750); err != nil {
			return err
		}
	}
	for name, expectedSize := range files {
		clean, pathErr := safeRelativePath(name)
		if pathErr != nil {
			return pathErr
		}
		info, statErr := os.Lstat(filepath.Join(staging, clean))
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != expectedSize {
			return fmt.Errorf("checkpoint file %q is not durably staged", name)
		}
	}
	seen := 0
	if err = filepath.WalkDir(staging, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return infoErr
		}
		relative, relativeErr := filepath.Rel(staging, path)
		if relativeErr != nil {
			return relativeErr
		}
		expected, ok := files[filepath.ToSlash(relative)]
		if !ok || expected != info.Size() {
			return fmt.Errorf("checkpoint staging does not match its manifest")
		}
		seen++
		return nil
	}); err != nil {
		return err
	}
	if seen != len(files) {
		return errors.New("checkpoint staging does not match its manifest")
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	return os.Rename(staging, destination)
}

func (s *Store) ArchiveCheckpoint(jobID, syncID string, destination io.Writer) error {
	if !safeSegment(syncID) {
		return errors.New("invalid checkpoint sync ID")
	}
	dir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	return archiveDirectory(filepath.Join(dir, "checkpoints", syncID), destination)
}

func (s *Store) WriteMetadata(jobID string, value any) error {
	return s.writeMetadata(jobID, "", value)
}

func (s *Store) WriteAttemptMetadata(jobID, attemptID string, value any) error {
	return s.writeMetadata(jobID, attemptID, value)
}

func (s *Store) writeMetadata(jobID, attemptID string, value any) error {
	dir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return err
	}
	metadataDir := filepath.Join(dir, "metadata")
	if err := os.MkdirAll(metadataDir, 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(metadataDir, "job-*.tmp")
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		os.Remove(temporary.Name())
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), filepath.Join(metadataDir, "job.json"))
}

func (s *Store) Archive(jobID string, destination io.Writer) error {
	dir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(destination)
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != dir && filepath.Base(path) == "checkpoint-staging" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = archive.Close()
		return err
	}
	return archive.Close()
}

func (s *Store) ArchiveAttempt(jobID, attemptID string, destination io.Writer) error {
	jobDir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	attemptDir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(destination)
	for _, source := range []struct {
		root   string
		prefix string
	}{{attemptDir, ""}, {filepath.Join(jobDir, "inputs"), "inputs"}} {
		walkErr := appendDirectoryToZip(archive, source.root, source.prefix)
		if walkErr != nil {
			_ = archive.Close()
			return walkErr
		}
	}
	return archive.Close()
}

// PromoteLegacyAttempt preserves data written before attempt-scoped storage was
// introduced. It is safe to call repeatedly and only moves the legacy mutable
// directories into the first attempt generation.
func (s *Store) PromoteLegacyAttempt(jobID, attemptID string) error {
	jobDir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	attemptDir, err := s.dataDir(jobID, attemptID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(attemptDir, 0o750); err != nil {
		return err
	}
	for _, name := range []string{"logs", "output", "metadata"} {
		source, destination := filepath.Join(jobDir, name), filepath.Join(attemptDir, name)
		if _, statErr := os.Stat(source); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			return statErr
		}
		if _, statErr := os.Stat(destination); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if err = os.Rename(source, destination); err != nil {
			return err
		}
	}
	return nil
}

func appendDirectoryToZip(archive *zip.Writer, root, prefix string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if prefix != "" {
			name = filepath.ToSlash(filepath.Join(prefix, relative))
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, header.Method = name, zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func archiveDirectory(dir string, destination io.Writer) error {
	archive := zip.NewWriter(destination)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, header.Method = filepath.ToSlash(relative), zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = archive.Close()
		return err
	}
	return archive.Close()
}

func (s *Store) DeleteJob(jobID string) error {
	dir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (s *Store) dataDir(jobID, attemptID string) (string, error) {
	dir, err := s.JobDir(jobID)
	if err != nil || attemptID == "" {
		return dir, err
	}
	if !safeSegment(attemptID) {
		return "", errors.New("invalid attempt ID")
	}
	return filepath.Join(dir, "attempts", attemptID), nil
}

func safeSegment(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, `/\\`)
}
func safeRelativePath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	clean := filepath.Clean(filepath.FromSlash(value))
	if value == "" || filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid relative path %q", value)
	}
	return clean, nil
}
func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func directorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, e := entry.Info()
			if e != nil {
				return e
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}
