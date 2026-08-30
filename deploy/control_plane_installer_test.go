package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestControlPlaneInstallerContract(t *testing.T) {
	installer := readTestFile(t, filepath.Join("..", "deploy", "install-control-plane.sh"))
	for _, required := range []string{
		"Linux is required",
		"docker info",
		"docker compose version",
		"SHA256SUMS",
		"sha256sum --check",
		"/etc/jobdock",
		"/var/lib/jobdock",
		"@sha256:",
		"compose pull",
		"compose config --quiet",
		"compose up -d",
		"/health/ready",
		"is healthy",
		"use the supported upgrade flow",
		"One-time setup token:",
		"overrides.env",
	} {
		if !strings.Contains(installer, required) {
			t.Fatalf("control-plane installer is missing %q", required)
		}
	}
	for _, forbidden := range []string{"git clone", "go build", "npm install"} {
		if strings.Contains(installer, forbidden) {
			t.Fatalf("control-plane installer must not require source tooling: found %q", forbidden)
		}
	}
}

func TestControlPlaneInstallerIsVerifiedAndIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("control-plane bootstrap runs on Linux release hosts")
	}
	for _, command := range []string{"sh", "sha256sum"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is not available: %v", command, err)
		}
	}

	root := t.TempDir()
	release := filepath.Join(root, "published")
	bin := filepath.Join(root, "bin")
	config := filepath.Join(root, "etc", "jobdock")
	data := filepath.Join(root, "var", "lib", "jobdock")
	releases := filepath.Join(root, "usr", "lib", "jobdock", "releases")
	for _, directory := range []string{release, bin} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	digest := strings.Repeat("a", 64)
	assets := map[string]string{
		"release-manifest.json": "{\n  \"schema_version\": 2,\n  \"version\": \"1.2.3\",\n  \"tag\": \"v1.2.3\",\n  \"images\": [\n    {\"image\": \"ghcr.io/alejandro-gz/jobdock-server\", \"reference\": \"ghcr.io/alejandro-gz/jobdock-server@sha256:" + digest + "\"},\n    {\"image\": \"ghcr.io/alejandro-gz/jobdock-builder\", \"reference\": \"ghcr.io/alejandro-gz/jobdock-builder@sha256:" + digest + "\"}\n  ]\n}",
		"docker-compose.yml":    "services:\n  jobdock-server:\n    image: \"ghcr.io/alejandro-gz/jobdock-server@sha256:" + digest + "\"\n  jobdock-builder:\n    image: \"ghcr.io/alejandro-gz/jobdock-builder@sha256:" + digest + "\"\n",
		"install-agent.sh":      "#!/bin/sh\nexit 0\n",
	}
	for name, contents := range assets {
		if err := os.WriteFile(filepath.Join(release, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	checksum := exec.Command("sha256sum", "release-manifest.json", "docker-compose.yml", "install-agent.sh")
	checksum.Dir = release
	output, err := checksum.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(release, "SHA256SUMS"), output, 0o600); err != nil {
		t.Fatal(err)
	}

	writeExecutable(t, filepath.Join(bin, "uname"), `#!/bin/sh
case "${1:-}" in -s) echo Linux;; -m) echo x86_64;; *) echo Linux;; esac
`)
	writeExecutable(t, filepath.Join(bin, "chown"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "docker"), `#!/bin/sh
printf '%s\n' "$*" >> "$JOBDOCK_TEST_DOCKER_CALLS"
exit 0
`)
	writeExecutable(t, filepath.Join(bin, "curl"), `#!/bin/sh
output=""
write_out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output="$2"; shift 2;;
    --write-out) write_out="$2"; shift 2;;
    --fail|--location|--silent|--show-error) shift;;
    *) url="$1"; shift;;
  esac
done
case "$url" in
  */health/ready) exit 0;;
  */latest) printf '%s' "${JOBDOCK_TEST_RELEASES_URL}/tag/v1.2.3"; exit 0;;
esac
asset=${url##*/}
cp "$JOBDOCK_TEST_RELEASE_DIR/$asset" "$output"
`)

	calls := filepath.Join(root, "docker-calls")
	environment := append(withoutEnvironment(os.Environ(), "PATH"),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"JOBDOCK_INSTALL_ALLOW_NON_ROOT=true",
		"JOBDOCK_INSTALL_CONFIG_DIR="+config,
		"JOBDOCK_INSTALL_DATA_DIR="+data,
		"JOBDOCK_INSTALL_RELEASES_DIR="+releases,
		"JOBDOCK_RELEASES_URL=https://releases.example.test",
		"JOBDOCK_TEST_RELEASES_URL=https://releases.example.test",
		"JOBDOCK_TEST_RELEASE_DIR="+release,
		"JOBDOCK_TEST_DOCKER_CALLS="+calls,
	)
	installer := filepath.Join("..", "deploy", "install-control-plane.sh")
	run := exec.Command("sh", installer, "--version", "1.2.3", "--port", "18080")
	run.Env = environment
	firstOutput, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("first install failed: %v\n%s", err, firstOutput)
	}
	if !strings.Contains(string(firstOutput), "JobDock 1.2.3 is healthy") || !strings.Contains(string(firstOutput), "One-time setup token:") {
		t.Fatalf("installer did not report actionable success:\n%s", firstOutput)
	}
	for _, path := range []string{
		filepath.Join(config, "docker-compose.yml"),
		filepath.Join(config, "jobdock.env"),
		filepath.Join(config, "overrides.env"),
		filepath.Join(config, "install-state"),
		filepath.Join(config, "secrets", "setup-token"),
		filepath.Join(config, "secrets", "master-key"),
		filepath.Join(config, "secrets", "server-builder-token"),
		filepath.Join(config, "secrets", "builder-token"),
		filepath.Join(data, "server"),
		filepath.Join(releases, "1.2.3", "release-manifest.json"),
	} {
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("expected installed path %s: %v", path, err)
		}
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(config, "secrets"):                         0o700,
		filepath.Join(config, "jobdock.env"):                     0o640,
		filepath.Join(config, "overrides.env"):                   0o640,
		filepath.Join(config, "secrets", "setup-token"):          0o400,
		filepath.Join(config, "secrets", "master-key"):           0o400,
		filepath.Join(config, "secrets", "server-builder-token"): 0o400,
		filepath.Join(config, "secrets", "builder-token"):        0o400,
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat installed path %s: %v", path, statErr)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("installed path %s has permissions %v, want %v", path, info.Mode().Perm(), want)
		}
	}
	environmentPath := filepath.Join(config, "jobdock.env")
	before, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "JOBDOCK_DATA_DIR="+data+"/server") {
		t.Fatal("generated environment does not use the stable data directory")
	}
	if strings.Contains(string(before), "JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD=") || strings.Contains(string(before), "JOBDOCK_BUILDER_TOKEN=") {
		t.Fatal("generated environment contains plaintext credentials")
	}
	setupToken, err := os.ReadFile(filepath.Join(config, "secrets", "setup-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(data, "preserved"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}

	run = exec.Command("sh", installer, "--version", "1.2.3")
	run.Env = environment
	secondOutput, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("same-version reinstall failed: %v\n%s", err, secondOutput)
	}
	after, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || strings.Contains(string(secondOutput), "One-time setup token:") {
		t.Fatal("same-version reinstall replaced configuration or reprinted credentials")
	}
	if preservedToken, readErr := os.ReadFile(filepath.Join(config, "secrets", "setup-token")); readErr != nil || string(preservedToken) != string(setupToken) {
		t.Fatal("same-version reinstall replaced the setup credential")
	}
	if contents, err := os.ReadFile(filepath.Join(data, "preserved")); err != nil || string(contents) != "state" {
		t.Fatal("same-version reinstall did not preserve persistent state")
	}
	run = exec.Command("sh", installer)
	run.Env = environment
	stableOutput, err := run.CombinedOutput()
	if err != nil || !strings.Contains(string(stableOutput), "JobDock 1.2.3 is healthy") {
		t.Fatalf("current-stable resolution failed: %v\n%s", err, stableOutput)
	}
	dockerCalls := readTestFile(t, calls)
	for _, expected := range []string{"info", "compose version", "config --quiet", "pull", "up -d"} {
		if !strings.Contains(dockerCalls, expected) {
			t.Fatalf("installer did not call Docker %q:\n%s", expected, dockerCalls)
		}
	}

	tamperedConfig := filepath.Join(root, "tampered-etc")
	if err = os.WriteFile(filepath.Join(release, "docker-compose.yml"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	tamperedEnvironment := append(withoutEnvironment(environment, "JOBDOCK_INSTALL_CONFIG_DIR"), "JOBDOCK_INSTALL_CONFIG_DIR="+tamperedConfig)
	run = exec.Command("sh", installer, "--version", "1.2.3")
	run.Env = tamperedEnvironment
	tamperedOutput, tamperedErr := run.CombinedOutput()
	if tamperedErr == nil || !strings.Contains(string(tamperedOutput), "checksum verification failed") {
		t.Fatalf("tampered asset was not rejected: %v\n%s", tamperedErr, tamperedOutput)
	}
	if _, statErr := os.Stat(tamperedConfig); !os.IsNotExist(statErr) {
		t.Fatal("installer mutated the target before checksum verification")
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}
