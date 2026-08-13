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

func TestBuildSourceIsImmutableAndLogsAreOffsetBounded(t *testing.T) {
	store, err := New(t.TempDir(), 12, 1024, 32)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.StoreBuildSource("build-one", "project.tar.gz", bytes.NewBufferString("immutable source"))
	if err != nil || source.Size != 16 || source.SHA256 != "fc17afe4af56fca9d2943b7901e7517611b37a36db7a7775b3e341e7d20a6ba0" {
		t.Fatalf("source metadata: %#v %v", source, err)
	}
	if _, err = store.StoreBuildSource("build-one", "replacement.tar.gz", bytes.NewBufferString("replacement")); err == nil {
		t.Fatal("immutable build source was replaced")
	}
	if next, err := store.AppendBuildLog("build-one", 0, bytes.NewBufferString("first second")); err != nil || next != 12 {
		t.Fatalf("append build log: offset=%d err=%v", next, err)
	}
	if next, err := store.AppendBuildLog("build-one", 0, bytes.NewBufferString("duplicate")); err != ErrOffsetMismatch || next != 12 {
		t.Fatalf("build log offset mismatch: offset=%d err=%v", next, err)
	}
	chunk, next, err := store.ReadBuildLogChunk("build-one", 6, 6)
	if err != nil || string(chunk) != "second" || next != 12 {
		t.Fatalf("build log chunk: data=%q offset=%d err=%v", chunk, next, err)
	}
}

func TestBuildSourceExtractionIsConfinedAndCollapsesProjectRoot(t *testing.T) {
	root := t.TempDir()
	store, err := New(root, 1<<20, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var source bytes.Buffer
	archive := zip.NewWriter(&source)
	file, _ := archive.Create("example/package.json")
	_, _ = file.Write([]byte(`{"name":"example"}`))
	_ = archive.Close()
	if _, err = store.StoreBuildSource("safe-build", "project.zip", &source); err != nil {
		t.Fatal(err)
	}
	project, cleanup, err := store.PrepareBuildSource("safe-build", "project.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if filepath.Base(project) != "example" {
		t.Fatalf("project root = %s", project)
	}
	if contents, readErr := os.ReadFile(filepath.Join(project, "package.json")); readErr != nil || string(contents) != `{"name":"example"}` {
		t.Fatalf("extracted manifest=%q error=%v", contents, readErr)
	}

	var unsafe bytes.Buffer
	unsafeArchive := zip.NewWriter(&unsafe)
	escape, _ := unsafeArchive.Create("../escape")
	_, _ = escape.Write([]byte("bad"))
	_ = unsafeArchive.Close()
	if _, err = store.StoreBuildSource("unsafe-build", "project.zip", &unsafe); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.PrepareBuildSource("unsafe-build", "project.zip"); err == nil {
		t.Fatal("archive traversal was accepted")
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

func TestAttemptFilesRemainIsolatedAndArchiveKeepsInputs(t *testing.T) {
	store, err := New(t.TempDir(), 1024, 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.StoreInput("job", "dataset.txt", bytes.NewBufferString("input")); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendAttemptLog("job", "attempt-one", "stdout", 0, bytes.NewBufferString("first")); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendAttemptOutput("job", "attempt-one", "result.txt", 0, bytes.NewBufferString("one")); err != nil {
		t.Fatal(err)
	}
	output, err := store.OpenAttemptOutput("job", "attempt-one", "result.txt")
	if err != nil {
		t.Fatal(err)
	}
	outputData, _ := io.ReadAll(output)
	_ = output.Close()
	if string(outputData) != "one" {
		t.Fatalf("attempt output = %q", outputData)
	}
	if _, err = store.OpenAttemptOutput("job", "attempt-one", "../result.txt"); err == nil {
		t.Fatal("unsafe output path was accepted")
	}
	if _, err = store.AppendAttemptLog("job", "attempt-two", "stdout", 0, bytes.NewBufferString("second")); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err = store.ArchiveAttempt("job", "attempt-one", &archive); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, item := range reader.File {
		file, openErr := item.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data, _ := io.ReadAll(file)
		_ = file.Close()
		contents[item.Name] = string(data)
	}
	if contents["logs/stdout.log"] != "first" || contents["output/result.txt"] != "one" || contents["inputs/dataset.txt"] != "input" {
		t.Fatalf("attempt archive contents: %#v", contents)
	}
	if bytes.Contains(archive.Bytes(), []byte("second")) {
		t.Fatal("archive leaked data from another attempt")
	}
}

func TestLegacyFilesPromoteIntoFirstAttempt(t *testing.T) {
	store, err := New(t.TempDir(), 1024, 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendLog("job", "stdout", 0, bytes.NewBufferString("legacy")); err != nil {
		t.Fatal(err)
	}
	if err = store.PromoteLegacyAttempt("job", "attempt-one"); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if _, err = store.ReadAttemptLog("job", "attempt-one", "stdout", 0, &log); err != nil || log.String() != "legacy" {
		t.Fatalf("promoted log = %q, err=%v", log.String(), err)
	}
	if err = store.PromoteLegacyAttempt("job", "attempt-one"); err != nil {
		t.Fatalf("promotion is not idempotent: %v", err)
	}
}
