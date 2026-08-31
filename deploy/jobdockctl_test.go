package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestJobDockctlContract(t *testing.T) {
	script := readTestFile(t, filepath.Join("..", "deploy", "jobdockctl.sh"))
	for _, required := range []string{"backup", "restore", "SHA256SUMS", "payload.tar", "database_schema", "includes_secrets", "insufficient free space", "--force", "unsafe path", "incompatible downgrade", "--healthcheck", "0600"} {
		if !strings.Contains(script, required) {
			t.Fatalf("jobdockctl is missing %q", required)
		}
	}
}

func TestJobDockctlBackupRestoreRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("system backup flow runs on Linux release hosts")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root := t.TempDir()
	config := filepath.Join(root, "config")
	data := filepath.Join(root, "data")
	bin := filepath.Join(root, "bin")
	for _, dir := range []string{filepath.Join(config, "secrets"), filepath.Join(data, "server", "jobs"), bin} {
		if err = os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(config, "install-state"):                "version=1.2.3\ndatabase_schema=32\n",
		filepath.Join(config, "jobdock.env"):                  "JOBDOCK_HTTP_PORT=8080\n",
		filepath.Join(config, "overrides.env"):                "",
		filepath.Join(config, "docker-compose.yml"):           "services: {}\n",
		filepath.Join(config, "docker-compose.exposure.yml"):  "services: {}\n",
		filepath.Join(config, "secrets", "master-key"):        "master-secret",
		filepath.Join(data, "server", "jobdock.db"):           "sqlite-snapshot",
		filepath.Join(data, "server", "jobs", "artifact.bin"): "artifact",
	}
	for path, contents := range files {
		if err = os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	docker := filepath.Join(bin, "docker")
	if err = os.WriteFile(docker, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$DOCKER_CALLS\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "backup.tar")
	script := filepath.Join("..", "deploy", "jobdockctl.sh")
	environment := append(withoutEnvironment(os.Environ(), "PATH", "JOBDOCK_INSTALL_CONFIG_DIR", "JOBDOCK_INSTALL_DATA_DIR", "JOBDOCK_INSTALL_ALLOW_NON_ROOT", "DOCKER_CALLS"), "PATH="+bin+":"+os.Getenv("PATH"), "JOBDOCK_INSTALL_CONFIG_DIR="+config, "JOBDOCK_INSTALL_DATA_DIR="+data, "JOBDOCK_INSTALL_ALLOW_NON_ROOT=true", "DOCKER_CALLS="+filepath.Join(root, "docker-calls"))
	command := exec.Command(sh, script, "backup", "--output", archive)
	command.Env = environment
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("backup failed: %v\n%s", runErr, output)
	}
	info, err := os.Stat(archive)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode: %v %v", info, err)
	}
	restoredConfig, restoredData := filepath.Join(root, "restored-config"), filepath.Join(root, "restored-data")
	restoreEnvironment := append(withoutEnvironment(environment, "JOBDOCK_INSTALL_CONFIG_DIR", "JOBDOCK_INSTALL_DATA_DIR"), "JOBDOCK_INSTALL_CONFIG_DIR="+restoredConfig, "JOBDOCK_INSTALL_DATA_DIR="+restoredData)
	command = exec.Command(sh, script, "restore", archive)
	command.Env = restoreEnvironment
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("restore failed: %v\n%s", runErr, output)
	}
	for path, expected := range map[string]string{filepath.Join(restoredConfig, "secrets", "master-key"): "master-secret", filepath.Join(restoredData, "server", "jobdock.db"): "sqlite-snapshot", filepath.Join(restoredData, "server", "jobs", "artifact.bin"): "artifact"} {
		contents, readErr := os.ReadFile(path)
		if readErr != nil || string(contents) != expected {
			t.Fatalf("restored %s: %q %v", path, contents, readErr)
		}
	}
}
