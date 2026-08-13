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
	if strings.Contains(compose, "build:") {
		t.Fatal("release Compose file must not contain local image build instructions")
	}
	installer := readTestFile(t, filepath.Join(outputPath, "install-agent.sh"))
	if !strings.Contains(installer, `DEFAULT_VERSION="1.2.3-rc.1"`) || !strings.Contains(installer, `DEFAULT_IMAGE_REFERENCE="`+images[1]["reference"]+`"`) {
		t.Fatal("release agent installer is not pinned to the verified agent reference")
	}
	assertInstallerPullsDefaultReference(t, filepath.Join(outputPath, "install-agent.sh"), images[1]["reference"])
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

func assertInstallerPullsDefaultReference(t *testing.T, installerPath, reference string) {
	t.Helper()
	binDir := t.TempDir()
	callsPath := filepath.Join(binDir, "docker-calls")
	dockerStub := `#!/bin/sh
if [ "${1:-}" = "container" ] && [ "${2:-}" = "inspect" ]; then exit 1; fi
printf '%s\n' "$*" >>"$DOCKER_CALLS"
`
	unameStub := `#!/bin/sh
if [ "${1:-}" = "-m" ]; then printf 'x86_64\n'; else printf 'Linux\n'; fi
`
	for name, contents := range map[string]string{"docker": dockerStub, "uname": unameStub} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("sh", installerPath, "--server", "https://dock.example.test", "--token", "one-use-token")
	command.Env = append(withoutEnvironment(os.Environ(), "PATH", "DOCKER_CALLS"), "PATH="+binDir+":"+os.Getenv("PATH"), "DOCKER_CALLS="+callsPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run generated installer: %v\n%s", err, output)
	}
	calls := readTestFile(t, callsPath)
	if !strings.Contains(calls, "pull "+reference) || strings.Contains(calls, "jobdock-agent:latest") {
		t.Fatalf("generated installer did not pull its release agent by default:\n%s", calls)
	}
}

func withoutEnvironment(environment []string, names ...string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		keep := true
		for _, name := range names {
			if strings.HasPrefix(entry, name+"=") {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
