package deploy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrepareReleaseAssetsPinsVerifiedComponents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release asset packaging runs on the Linux release runner")
	}
	for _, command := range []string{"sh", "jq", "sha256sum"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is not available: %v", command, err)
		}
	}

	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.json")
	outputPath := filepath.Join(root, "assets")
	commit := strings.Repeat("a", 40)
	components := []string{"server", "agent", "builder"}
	images := make([]map[string]string, 0, len(components))
	for index, component := range components {
		digest := "sha256:" + strings.Repeat(string(rune('1'+index)), 64)
		image := "ghcr.io/alejandro-gz/jobdock-" + component
		images = append(images, map[string]string{"image": image, "digest": digest, "reference": image + "@" + digest})
	}
	manifest := map[string]any{"schema_version": 1, "version": "1.2.3-rc.1", "tag": "v1.2.3-rc.1", "commit": commit, "images": images}
	contents, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifestPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("sh", filepath.Join("..", "deploy", "prepare-release-assets.sh"), manifestPath, outputPath)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("prepare release assets: %v\n%s", runErr, output)
	}

	for _, name := range []string{"release-manifest.json", "docker-compose.yml", "install-agent.sh", "SHA256SUMS", "release-notes.md"} {
		if _, err = os.Stat(filepath.Join(outputPath, name)); err != nil {
			t.Fatalf("expected release asset %s: %v", name, err)
		}
	}
	compose := readTestFile(t, filepath.Join(outputPath, "docker-compose.yml"))
	if strings.Contains(compose, "@@JOBDOCK_") || !strings.Contains(compose, images[0]["reference"]) || !strings.Contains(compose, images[2]["reference"]) {
		t.Fatal("release Compose file is not pinned to the verified server and builder references")
	}
	installer := readTestFile(t, filepath.Join(outputPath, "install-agent.sh"))
	if !strings.Contains(installer, `DEFAULT_VERSION="1.2.3-rc.1"`) || !strings.Contains(installer, `DEFAULT_IMAGE_REFERENCE="`+images[1]["reference"]+`"`) {
		t.Fatal("release agent installer is not pinned to the verified agent reference")
	}
	notes := readTestFile(t, filepath.Join(outputPath, "release-notes.md"))
	if !strings.Contains(notes, "## Highlights") || !strings.Contains(notes, "## Changes") || !strings.Contains(notes, commit) {
		t.Fatal("release notes do not contain highlights, changes, and source commit context")
	}
	check := exec.Command("sha256sum", "--check", "SHA256SUMS")
	check.Dir = outputPath
	if output, checkErr := check.CombinedOutput(); checkErr != nil {
		t.Fatalf("verify release checksums: %v\n%s", checkErr, output)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
