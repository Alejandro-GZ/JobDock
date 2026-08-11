package filestore

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOutputRejectsTraversal(t *testing.T) {
	store, err := New(t.TempDir(), 1024, 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendOutput("job", "../escape", 0, bytes.NewBufferString("x")); err == nil {
		t.Fatal("expected traversal to fail")
	}
}

func TestInputsAreImmutableBoundedAndRemovedWithJob(t *testing.T) {
	root := t.TempDir()
	store, err := New(root, 1024, 1024, 5)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.StoreInput("job", "dataset/value.txt", bytes.NewBufferString("hello"))
	if err != nil || metadata.Size != 5 || metadata.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("stored input: %#v %v", metadata, err)
	}
	if _, err = store.StoreInput("job", "dataset/value.txt", bytes.NewBufferString("again")); err == nil {
		t.Fatal("duplicate input replaced immutable content")
	}
	if _, err = store.StoreInput("job", "other.txt", bytes.NewBufferString("x")); err != ErrLimitExceeded {
		t.Fatalf("limit error = %v", err)
	}
	if _, err = store.StoreInput("job", "../escape", bytes.NewBufferString("x")); err == nil {
		t.Fatal("input traversal was accepted")
	}
	file, err := store.OpenInput("job", metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(file)
	_ = file.Close()
	if string(data) != "hello" {
		t.Fatalf("input content = %q", data)
	}
	if err = store.DeleteJob("job"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.OpenInput("job", metadata.Path); !os.IsNotExist(err) {
		t.Fatalf("input survived job cleanup: %v", err)
	}
}

func TestCheckpointUploadResumesAndPromotionPreservesLastConfirmed(t *testing.T) {
	store, err := New(t.TempDir(), 1<<20, 10<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := store.AppendCheckpoint("job", "sync-one", "model.pt", 0, bytes.NewBufferString("first")); err != nil || next != 5 {
		t.Fatalf("first chunk: offset=%d err=%v", next, err)
	}
	if next, err := store.AppendCheckpoint("job", "sync-one", "model.pt", 0, bytes.NewBufferString("duplicate")); err != ErrOffsetMismatch || next != 5 {
		t.Fatalf("resume offset: offset=%d err=%v", next, err)
	}
	if err := store.ConfirmCheckpoint("job", "sync-one", map[string]int64{"model.pt": 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCheckpoint("job", "sync-two", "model.pt", 0, bytes.NewBufferString("partial")); err != nil {
		t.Fatal(err)
	}

	jobDir, _ := store.JobDir("job")
	data, err := os.ReadFile(filepath.Join(jobDir, "checkpoints", "sync-one", "model.pt"))
	if err != nil || string(data) != "first" {
		t.Fatalf("confirmed generation changed: %q %v", data, err)
	}

	var archive bytes.Buffer
	if err = store.ArchiveCheckpoint("job", "sync-one", &archive); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil || len(reader.File) != 1 {
		t.Fatalf("checkpoint archive: %v %#v", err, reader.File)
	}
	file, _ := reader.File[0].Open()
	archived, _ := io.ReadAll(file)
	_ = file.Close()
	if string(archived) != "first" {
		t.Fatalf("archived checkpoint = %q", archived)
	}
}
func TestLogOffsetsAreIdempotent(t *testing.T) {
	store, err := New(t.TempDir(), 1024, 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	offset, err := store.AppendLog("job", "stdout", 0, bytes.NewBufferString("hello"))
	if err != nil || offset != 5 {
		t.Fatalf("first append: %d %v", offset, err)
	}
	offset, err = store.AppendLog("job", "stdout", 0, bytes.NewBufferString("hello"))
	if err != ErrOffsetMismatch || offset != 5 {
		t.Fatalf("expected offset mismatch at 5: %d %v", offset, err)
	}
	chunk, next, err := store.ReadLogChunk("job", "stdout", 2, 2)
	if err != nil || string(chunk) != "ll" || next != 4 {
		t.Fatalf("bounded chunk: %q %d %v", chunk, next, err)
	}
}
