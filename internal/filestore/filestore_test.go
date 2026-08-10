package filestore

import (
	"bytes"
	"testing"
)

func TestOutputRejectsTraversal(t *testing.T) {
	store, err := New(t.TempDir(), 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendOutput("job", "../escape", 0, bytes.NewBufferString("x")); err == nil {
		t.Fatal("expected traversal to fail")
	}
}
func TestLogOffsetsAreIdempotent(t *testing.T) {
	store, err := New(t.TempDir(), 1024, 1024)
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
}
