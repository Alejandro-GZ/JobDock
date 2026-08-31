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

func TestDoctorContract(t *testing.T) {
	doctor := readTestFile(t, "jobdock-doctor.sh")
	for _, required := range []string{"schema_version", "Remediation:", "docker compose", "/proc/meminfo", "df -Pk", "github", "ghcr", "public_url", "getent hosts", "required_port", "tls", "jobdock-server", "jobdock-builder", "buildkitd", "nvidia-smi", "--gpus all", "--json", "--gpu", "--exposure"} {
		if !strings.Contains(doctor, required) {
			t.Fatalf("doctor is missing %q", required)
		}
	}
	for _, forbidden := range []string{"--repair", "docker system prune", "rm -rf"} {
		if strings.Contains(doctor, forbidden) {
			t.Fatalf("read-only doctor contains %q", forbidden)
		}
	}
}

func TestDoctorProducesStableJSONBeforeInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("doctor targets Linux")
	}
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\ncase \"${1:-}\" in -s) echo Linux;; -m) echo x86_64;; *) echo Linux;; esac\n")
	writeExecutable(t, filepath.Join(bin, "docker"), "#!/bin/sh\ncase \"$*\" in 'info'|'compose version') echo 'Docker test'; exit 0;; esac\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "curl"), "#!/bin/sh\ncase \"$*\" in *ghcr.io*) printf 401;; esac\nexit 0\n")
	command := exec.Command("sh", filepath.Join("..", "deploy", "jobdock-doctor.sh"), "--json", "--config-dir", filepath.Join(t.TempDir(), "missing"), "--data-dir", t.TempDir())
	command.Env = append(withoutEnvironment(os.Environ(), "PATH"), "PATH="+bin+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, output)
	}
	var result struct {
		SchemaVersion int  `json:"schema_version"`
		OK            bool `json:"ok"`
		Checks        []struct {
			Check       string `json:"check"`
			Status      string `json:"status"`
			Remediation string `json:"remediation"`
		} `json:"checks"`
	}
	if err = json.Unmarshal(output, &result); err != nil {
		t.Fatalf("invalid doctor JSON: %v\n%s", err, output)
	}
	if result.SchemaVersion != 1 || !result.OK || len(result.Checks) < 8 {
		t.Fatalf("unexpected doctor result: %#v", result)
	}
}
