package filestore

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrOffsetMismatch = errors.New("upload offset does not match stored size")
var ErrLimitExceeded = errors.New("storage limit exceeded")

type Store struct {
	root           string
	maxLogBytes    int64
	maxOutputBytes int64
}

func New(root string, maxLogBytes, maxOutputBytes int64) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "jobs"), 0o750); err != nil {
		return nil, err
	}
	return &Store{root: root, maxLogBytes: maxLogBytes, maxOutputBytes: maxOutputBytes}, nil
}

func (s *Store) JobDir(jobID string) (string, error) {
	if !safeSegment(jobID) {
		return "", errors.New("invalid job ID")
	}
	return filepath.Join(s.root, "jobs", jobID), nil
}

func (s *Store) AppendLog(jobID, stream string, offset int64, source io.Reader) (int64, error) {
	if stream != "stdout" && stream != "stderr" {
		return 0, errors.New("invalid log stream")
	}
	dir, err := s.JobDir(jobID)
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
	dir, err := s.JobDir(jobID)
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

func (s *Store) LogSize(jobID, stream string) (int64, error) {
	if stream != "stdout" && stream != "stderr" {
		return 0, errors.New("invalid log stream")
	}
	dir, err := s.JobDir(jobID)
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
	clean, err := safeRelativePath(relativePath)
	if err != nil {
		return 0, err
	}
	dir, err := s.JobDir(jobID)
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

func (s *Store) WriteMetadata(jobID string, value any) error {
	dir, err := s.JobDir(jobID)
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

func (s *Store) DeleteJob(jobID string) error {
	dir, err := s.JobDir(jobID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
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
