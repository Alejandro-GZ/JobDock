package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
)

func TestReadBuildDigestSupportsBuildKitMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(path, []byte(`{"containerimage.digest":"sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := readBuildDigest(path)
	if err != nil || digest != "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("digest=%q error=%v", digest, err)
	}
	if err = os.WriteFile(path, []byte(`{"containerimage.descriptor":{"digest":"sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err = readBuildDigest(path)
	if err != nil || digest != "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef" {
		t.Fatalf("descriptor digest=%q error=%v", digest, err)
	}
}

func TestBuildKitValidatesModeInputsBeforeExecution(t *testing.T) {
	root := t.TempDir()
	executor := NewBuildKit(config.Builder{BuildctlBinary: "must-not-run", BuildkitAddress: "tcp://buildkit", MaxArtifactBytes: 1024})
	work := domain.BuildWork{Build: domain.Build{Mode: domain.BuildModeDockerfile}}
	if _, err := executor.Build(context.Background(), work, root, filepath.Join(root, "artifact.tar"), os.Stderr); err == nil {
		t.Fatal("Dockerfile build without Dockerfile was accepted")
	}
	work.Build.Mode = domain.BuildModeRailpack
	if _, err := executor.Build(context.Background(), work, root, filepath.Join(root, "artifact.tar"), os.Stderr); err == nil {
		t.Fatal("Railpack build without persisted plan was accepted")
	}
}

func TestSourcePathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := sourcePath(root, "../Dockerfile"); err == nil {
		t.Fatal("source traversal was accepted")
	}
	path, err := sourcePath(root, "services/api")
	if err != nil || path != filepath.Join(root, "services", "api") {
		t.Fatalf("resolved path=%q error=%v", path, err)
	}
}
