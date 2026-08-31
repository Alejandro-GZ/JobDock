package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseComposeMakesBuilderOptional(t *testing.T) {
	content, err := os.ReadFile("docker-compose.release.yml.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if strings.Count(text, `profiles: ["builder"]`) != 2 {
		t.Fatal("builder and BuildKit must share the optional builder profile")
	}
	if !strings.Contains(text, `JOBDOCK_BUILDER_ENABLED: "${JOBDOCK_BUILDER_ENABLED:-true}"`) {
		t.Fatal("server must receive the builder capability state")
	}
}

func TestInstallerAndDoctorExposeBuilderMode(t *testing.T) {
	installer, err := os.ReadFile("install-control-plane.sh")
	if err != nil {
		t.Fatal(err)
	}
	doctor, err := os.ReadFile("jobdock-doctor.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--builder MODE", "JOBDOCK_BUILDER_ENABLED", "COMPOSE_PROFILES"} {
		if !strings.Contains(string(installer), expected) {
			t.Fatalf("installer missing %s", expected)
		}
	}
	if !strings.Contains(string(doctor), "Source builds are disabled by configuration") {
		t.Fatal("doctor does not distinguish a disabled builder")
	}
}
