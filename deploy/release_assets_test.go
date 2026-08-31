package deploy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	sdkPath := filepath.Join(root, "sdk")
	cliPath := filepath.Join(root, "cli")
	if err := os.Mkdir(sdkPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cliPath, 0o700); err != nil {
		t.Fatal(err)
	}
	wheelName := "jobdock_sdk-1.2.3rc1-py3-none-any.whl"
	sdistName := "jobdock_sdk-1.2.3rc1.tar.gz"
	wheelContents := []byte("verified wheel")
	sdistContents := []byte("verified source distribution")
	for name, data := range map[string][]byte{wheelName: wheelContents, sdistName: sdistContents} {
		if err := os.WriteFile(filepath.Join(sdkPath, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit := strings.Repeat("a", 40)
	cliContents := map[string][]byte{
		"jobdock-cli_1.2.3-rc.1_linux_amd64.tar.gz": []byte("verified amd64 cli archive"),
		"jobdock-cli_1.2.3-rc.1_linux_arm64.tar.gz": []byte("verified arm64 cli archive"),
	}
	for name, data := range cliContents {
		if err := os.WriteFile(filepath.Join(cliPath, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	components := []string{"server", "agent", "builder"}
	images := make([]map[string]any, 0, len(components))
	for index, component := range components {
		digest := "sha256:" + strings.Repeat(string(rune('1'+index)), 64)
		image := "ghcr.io/alejandro-gz/jobdock-" + component
		platforms := []string{"linux/amd64"}
		if component != "builder" {
			platforms = append(platforms, "linux/arm64")
		}
		images = append(images, map[string]any{"image": image, "digest": digest, "reference": image + "@" + digest, "platforms": platforms})
	}
	manifest := map[string]any{
		"schema_version": 2, "version": "1.2.3-rc.1", "tag": "v1.2.3-rc.1", "commit": commit, "images": images,
		"database": map[string]any{"schema": 32, "rollback_floor": 32},
		"sdk": map[string]any{
			"name": "jobdock-sdk", "version": "1.2.3rc1",
			"wheel": map[string]string{"filename": wheelName, "sha256": testSHA256(wheelContents)},
			"sdist": map[string]string{"filename": sdistName, "sha256": testSHA256(sdistContents)},
		},
		"cli": map[string]any{
			"name": "jobdock", "version": "1.2.3-rc.1", "server_api": "v1",
			"artifacts": []map[string]string{
				{"os": "linux", "arch": "amd64", "filename": "jobdock-cli_1.2.3-rc.1_linux_amd64.tar.gz", "sha256": testSHA256(cliContents["jobdock-cli_1.2.3-rc.1_linux_amd64.tar.gz"])},
				{"os": "linux", "arch": "arm64", "filename": "jobdock-cli_1.2.3-rc.1_linux_arm64.tar.gz", "sha256": testSHA256(cliContents["jobdock-cli_1.2.3-rc.1_linux_arm64.tar.gz"])},
			},
		},
	}
	contents, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifestPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("sh", filepath.Join("..", "deploy", "prepare-release-assets.sh"), manifestPath, outputPath, sdkPath, cliPath)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("prepare release assets: %v\n%s", runErr, output)
	}

	for _, name := range []string{"release-manifest.json", "docker-compose.yml", "docker-compose.domain.yml", "docker-compose.proxy.yml", "docker-compose.local.yml", "Caddyfile", "install-control-plane.sh", "install-agent.sh", "install-cli.sh", "jobdock-doctor", "jobdockctl", "SHA256SUMS", "release-notes.md", "jobdock-cli_1.2.3-rc.1_linux_amd64.tar.gz", "jobdock-cli_1.2.3-rc.1_linux_arm64.tar.gz", wheelName, sdistName} {
		if _, err = os.Stat(filepath.Join(outputPath, name)); err != nil {
			t.Fatalf("expected release asset %s: %v", name, err)
		}
	}
	compose := readTestFile(t, filepath.Join(outputPath, "docker-compose.yml"))
	if strings.Contains(compose, "@@JOBDOCK_") || !strings.Contains(compose, images[0]["reference"].(string)) || !strings.Contains(compose, images[2]["reference"].(string)) {
		t.Fatal("release Compose file is not pinned to the verified server and builder references")
	}
	if strings.Contains(compose, "build:") {
		t.Fatal("release Compose file must not contain local image build instructions")
	}
	domainCompose := readTestFile(t, filepath.Join(outputPath, "docker-compose.domain.yml"))
	if !strings.Contains(domainCompose, "caddy:2.10.2-alpine") || !strings.Contains(domainCompose, `"80:80"`) || !strings.Contains(domainCompose, `"443:443"`) {
		t.Fatal("domain deployment does not publish the versioned Caddy edge on 80/443")
	}
	proxyCompose := readTestFile(t, filepath.Join(outputPath, "docker-compose.proxy.yml"))
	if strings.Contains(proxyCompose, "caddy:") || !strings.Contains(proxyCompose, "127.0.0.1") {
		t.Fatal("proxy deployment must expose only the loopback server without Caddy")
	}
	caddyfile := readTestFile(t, filepath.Join(outputPath, "Caddyfile"))
	for _, required := range []string{"Strict-Transport-Security", "flush_interval -1", "reverse_proxy jobdock-server:8080"} {
		if !strings.Contains(caddyfile, required) {
			t.Fatalf("release Caddyfile is missing %q", required)
		}
	}
	installer := readTestFile(t, filepath.Join(outputPath, "install-agent.sh"))
	if !strings.Contains(installer, `DEFAULT_VERSION="1.2.3-rc.1"`) || !strings.Contains(installer, `DEFAULT_IMAGE_REFERENCE="`+images[1]["reference"].(string)+`"`) {
		t.Fatal("release agent installer is not pinned to the verified agent reference")
	}
	assertInstallerPullsDefaultReference(t, filepath.Join(outputPath, "install-agent.sh"), images[1]["reference"].(string))
	controlPlaneInstaller := readTestFile(t, filepath.Join(outputPath, "install-control-plane.sh"))
	if !strings.Contains(controlPlaneInstaller, "sha256sum --check") || !strings.Contains(controlPlaneInstaller, "docker compose") || !strings.Contains(controlPlaneInstaller, `DEFAULT_VERSION="1.2.3-rc.1"`) {
		t.Fatal("release control-plane installer is missing its pinned version, verification, or Compose startup")
	}
	notes := readTestFile(t, filepath.Join(outputPath, "release-notes.md"))
	if !strings.Contains(notes, "## Highlights") || !strings.Contains(notes, "## Changes") || !strings.Contains(notes, commit) || !strings.Contains(notes, "pip install jobdock-sdk==1.2.3rc1") {
		t.Fatal("release notes do not contain highlights, SDK installation, changes, and source commit context")
	}
	check := exec.Command("sha256sum", "--check", "SHA256SUMS")
	check.Dir = outputPath
	if output, checkErr := check.CombinedOutput(); checkErr != nil {
		t.Fatalf("verify release checksums: %v\n%s", checkErr, output)
	}

	if err = os.WriteFile(filepath.Join(sdkPath, wheelName), []byte("tampered wheel"), 0o600); err != nil {
		t.Fatal(err)
	}
	tamperedOutputPath := filepath.Join(root, "tampered-assets")
	tampered := exec.Command("sh", filepath.Join("..", "deploy", "prepare-release-assets.sh"), manifestPath, tamperedOutputPath, sdkPath, cliPath)
	output, tamperedErr := tampered.CombinedOutput()
	if tamperedErr == nil || !strings.Contains(string(output), "SDK wheel checksum mismatch") {
		t.Fatalf("tampered SDK wheel was not rejected: %v\n%s", tamperedErr, output)
	}
}

func testSHA256(contents []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(contents))
}

func assertInstallerPullsDefaultReference(t *testing.T, installerPath, reference string) {
	t.Helper()
	binDir := t.TempDir()
	callsPath := filepath.Join(binDir, "docker-calls")
	dockerStub := `#!/bin/sh
if [ "${1:-}" = "container" ] && [ "${2:-}" = "inspect" ]; then [ -f "$DOCKER_STARTED" ]; exit $?; fi
printf '%s\n' "$*" >>"$DOCKER_CALLS"
if [ "${1:-}" = "run" ]; then : >"$DOCKER_STARTED"; fi
`
	unameStub := `#!/bin/sh
if [ "${1:-}" = "-m" ]; then printf 'x86_64\n'; else printf 'Linux\n'; fi
`
	curlStub := "#!/bin/sh\nprintf '{\"status\":\"connected\"}\\n'\n"
	for name, contents := range map[string]string{"docker": dockerStub, "uname": unameStub, "curl": curlStub} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("sh", installerPath, "--server", "https://dock.example.test", "--token", "one-use-token")
	command.Env = append(withoutEnvironment(os.Environ(), "PATH", "DOCKER_CALLS", "DOCKER_STARTED", "JOBDOCK_AGENT_STATE_DIR"), "PATH="+binDir+":"+os.Getenv("PATH"), "DOCKER_CALLS="+callsPath, "DOCKER_STARTED="+filepath.Join(binDir, "started"), "JOBDOCK_AGENT_STATE_DIR="+filepath.Join(binDir, "state"))
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
