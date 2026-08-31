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
		"add --upgrade",
		"Upgrade plan",
		"--allow-irreversible",
		"One-time setup token:",
		"overrides.env",
		"--mode must be domain, proxy, or local",
		"local mode requires explicit --allow-insecure-http",
		"HTTPS readiness failed",
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
	installedBin := filepath.Join(root, "usr", "bin")
	for _, directory := range []string{release, bin} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	digest := strings.Repeat("a", 64)
	assets := map[string]string{
		"release-manifest.json":     "{\n  \"schema_version\": 2,\n  \"version\": \"1.2.3\",\n  \"tag\": \"v1.2.3\",\n  \"database\": {\"schema\": 32, \"rollback_floor\": 32},\n  \"images\": [\n    {\"image\": \"ghcr.io/alejandro-gz/jobdock-server\", \"reference\": \"ghcr.io/alejandro-gz/jobdock-server@sha256:" + digest + "\"},\n    {\"image\": \"ghcr.io/alejandro-gz/jobdock-builder\", \"reference\": \"ghcr.io/alejandro-gz/jobdock-builder@sha256:" + digest + "\"}\n  ]\n}",
		"docker-compose.yml":        "services:\n  jobdock-server:\n    image: \"ghcr.io/alejandro-gz/jobdock-server@sha256:" + digest + "\"\n  jobdock-builder:\n    image: \"ghcr.io/alejandro-gz/jobdock-builder@sha256:" + digest + "\"\n",
		"install-agent.sh":          "#!/bin/sh\nexit 0\n",
		"docker-compose.domain.yml": "services:\n  caddy:\n    image: caddy:2.10.2-alpine\n",
		"docker-compose.proxy.yml":  "services:\n  jobdock-server:\n    ports: [\"127.0.0.1:18080:8080\"]\n",
		"docker-compose.local.yml":  "services:\n  jobdock-server:\n    ports: [\"18080:8080\"]\n",
		"Caddyfile":                 "{$JOBDOCK_DOMAIN} { reverse_proxy jobdock-server:8080 }\n",
		"jobdock-doctor":            "#!/bin/sh\nexit 0\n",
	}
	for name, contents := range assets {
		if err := os.WriteFile(filepath.Join(release, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	checksum := exec.Command("sha256sum", "release-manifest.json", "docker-compose.yml", "docker-compose.domain.yml", "docker-compose.proxy.yml", "docker-compose.local.yml", "Caddyfile", "install-agent.sh", "jobdock-doctor")
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
  */health/ready) printf '{"status":"ready","version":"%s","database_schema":32}' "${JOBDOCK_TEST_HEALTH_VERSION:-1.2.3}"; exit 0;;
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
		"JOBDOCK_INSTALL_BIN_DIR="+installedBin,
		"JOBDOCK_RELEASES_URL=https://releases.example.test",
		"JOBDOCK_TEST_RELEASES_URL=https://releases.example.test",
		"JOBDOCK_TEST_RELEASE_DIR="+release,
		"JOBDOCK_TEST_DOCKER_CALLS="+calls,
	)
	installer := filepath.Join("..", "deploy", "install-control-plane.sh")
	run := exec.Command("sh", installer, "--version", "1.2.3", "--mode", "local", "--allow-insecure-http", "--port", "18080")
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
		filepath.Join(config, "docker-compose.exposure.yml"),
		filepath.Join(config, "Caddyfile"),
		filepath.Join(config, "jobdock.env"),
		filepath.Join(config, "overrides.env"),
		filepath.Join(config, "install-state"),
		filepath.Join(config, "secrets", "setup-token"),
		filepath.Join(config, "secrets", "master-key"),
		filepath.Join(config, "secrets", "server-builder-token"),
		filepath.Join(config, "secrets", "builder-token"),
		filepath.Join(data, "server"),
		filepath.Join(releases, "1.2.3", "release-manifest.json"),
		filepath.Join(installedBin, "jobdock-doctor"),
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

	for mode, arguments := range map[string][]string{
		"domain": {"--version", "1.2.3", "--mode", "domain", "--domain", "dock.example.test"},
		"proxy":  {"--version", "1.2.3", "--mode", "proxy", "--public-url", "https://dock.example.test"},
	} {
		modeConfig := filepath.Join(root, mode+"-etc")
		modeData := filepath.Join(root, mode+"-data")
		modeEnvironment := append(withoutEnvironment(environment, "JOBDOCK_INSTALL_CONFIG_DIR", "JOBDOCK_INSTALL_DATA_DIR"),
			"JOBDOCK_INSTALL_CONFIG_DIR="+modeConfig,
			"JOBDOCK_INSTALL_DATA_DIR="+modeData,
		)
		modeRun := exec.Command("sh", append([]string{installer}, arguments...)...)
		modeRun.Env = modeEnvironment
		if modeOutput, modeErr := modeRun.CombinedOutput(); modeErr != nil {
			t.Fatalf("%s install failed: %v\n%s", mode, modeErr, modeOutput)
		}
		modeDefaults := readTestFile(t, filepath.Join(modeConfig, "jobdock.env"))
		if !strings.Contains(modeDefaults, "JOBDOCK_EXPOSURE_MODE="+mode) || !strings.Contains(modeDefaults, "JOBDOCK_ALLOW_INSECURE_HTTP=false") || !strings.Contains(modeDefaults, "JOBDOCK_TRUST_PROXY_HEADERS=true") {
			t.Fatalf("%s install has incorrect exposure defaults:\n%s", mode, modeDefaults)
		}
		exposure := readTestFile(t, filepath.Join(modeConfig, "docker-compose.exposure.yml"))
		if (mode == "domain") != strings.Contains(exposure, "caddy:") {
			t.Fatalf("%s install selected the wrong Compose exposure:\n%s", mode, exposure)
		}
	}
	invalidDomainConfig := filepath.Join(root, "invalid-domain-etc")
	invalidDomainEnvironment := append(withoutEnvironment(environment, "JOBDOCK_INSTALL_CONFIG_DIR"), "JOBDOCK_INSTALL_CONFIG_DIR="+invalidDomainConfig)
	invalidDomainRun := exec.Command("sh", installer, "--version", "1.2.3", "--mode", "domain", "--domain", "-invalid.example.test")
	invalidDomainRun.Env = invalidDomainEnvironment
	invalidDomainOutput, invalidDomainErr := invalidDomainRun.CombinedOutput()
	if invalidDomainErr == nil || !strings.Contains(string(invalidDomainOutput), "valid fully qualified DNS name") {
		t.Fatalf("invalid domain was not rejected: %v\n%s", invalidDomainErr, invalidDomainOutput)
	}
	if _, statErr := os.Stat(invalidDomainConfig); !os.IsNotExist(statErr) {
		t.Fatal("invalid domain mutated the target")
	}

	manifestPath := filepath.Join(release, "release-manifest.json")
	upgradedManifest := strings.ReplaceAll(readTestFile(t, manifestPath), "1.2.3", "1.2.4")
	if err = os.WriteFile(manifestPath, []byte(upgradedManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	checksum = exec.Command("sha256sum", "release-manifest.json", "docker-compose.yml", "docker-compose.domain.yml", "docker-compose.proxy.yml", "docker-compose.local.yml", "Caddyfile", "install-agent.sh", "jobdock-doctor")
	checksum.Dir = release
	output, err = checksum.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(release, "SHA256SUMS"), output, 0o600); err != nil {
		t.Fatal(err)
	}
	upgradeEnvironment := append(withoutEnvironment(environment, "JOBDOCK_TEST_HEALTH_VERSION"), "JOBDOCK_TEST_HEALTH_VERSION=1.2.4")
	upgrade := exec.Command("sh", installer, "--version", "1.2.4", "--upgrade", "--no-backup", "--yes")
	upgrade.Env = upgradeEnvironment
	upgradeOutput, upgradeErr := upgrade.CombinedOutput()
	if upgradeErr != nil {
		t.Fatalf("transactional upgrade failed: %v\n%s", upgradeErr, upgradeOutput)
	}
	if !strings.Contains(string(upgradeOutput), "Upgrade plan") || !strings.Contains(readTestFile(t, filepath.Join(config, "install-state")), "version=1.2.4") {
		t.Fatal("upgrade did not report its plan or persist the target version")
	}
	history, globErr := filepath.Glob(filepath.Join(releases, "upgrade-history", "*-from-1.2.3-to-1.2.4", "result"))
	if globErr != nil || len(history) != 1 || !strings.Contains(readTestFile(t, history[0]), "status=succeeded") {
		t.Fatal("upgrade result was not recorded")
	}

	tamperedConfig := filepath.Join(root, "tampered-etc")
	if err = os.WriteFile(filepath.Join(release, "docker-compose.yml"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	tamperedEnvironment := append(withoutEnvironment(environment, "JOBDOCK_INSTALL_CONFIG_DIR"), "JOBDOCK_INSTALL_CONFIG_DIR="+tamperedConfig)
	run = exec.Command("sh", installer, "--version", "1.2.3", "--mode", "local", "--allow-insecure-http")
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
