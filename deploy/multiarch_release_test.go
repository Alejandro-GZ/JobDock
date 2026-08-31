package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestReleasePlatformSupportIsExplicit(t *testing.T) {
	workflowBytes, err := os.ReadFile("../.github/workflows/release-images.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	if strings.Count(workflow, "platforms: linux/amd64,linux/arm64") != 2 {
		t.Fatal("server and agent must publish amd64/arm64 manifests")
	}
	if !strings.Contains(workflow, "platforms: linux/amd64") || !strings.Contains(workflow, "Builder must not advertise unverified linux/arm64 support") {
		t.Fatal("builder must remain explicitly amd64-only")
	}
	for _, required := range []string{"platform-smoke:", "--platform linux/arm64", "jobdock-server", "jobdock-agent"} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("multi-architecture release gate missing %s", required)
		}
	}
}

func TestDockerfilesDoNotPinTargetArchitecture(t *testing.T) {
	for _, name := range []string{"../Dockerfile.server", "../Dockerfile.agent", "../Dockerfile.builder"} {
		contents, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), "GOARCH=amd64") {
			t.Fatalf("%s pins the target architecture", name)
		}
	}
}

func TestInstallersAndDoctorRecognizeArm64Capabilities(t *testing.T) {
	for _, item := range []struct {
		name     string
		required []string
	}{
		{"install-control-plane.sh", []string{"aarch64|arm64", "source builds are not supported on linux/arm64"}},
		{"install-agent.sh", []string{"aarch64|arm64", "NVIDIA GPU mode is not officially supported on linux/arm64"}},
		{"install-cli.sh", []string{"aarch64|arm64", "platform=linux_arm64"}},
		{"jobdock-doctor.sh", []string{"linux/arm64 is supported for the server and CPU agent", "NVIDIA GPU mode is not officially supported"}},
	} {
		contents, err := os.ReadFile(item.name)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range item.required {
			if !strings.Contains(string(contents), required) {
				t.Fatalf("%s missing %s", item.name, required)
			}
		}
	}
}
