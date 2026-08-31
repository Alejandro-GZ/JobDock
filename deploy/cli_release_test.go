package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestCLIInstallerVerifiesBeforeExtraction(t *testing.T) {
	contents, err := os.ReadFile("install-cli.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	checksum := strings.Index(text, "sha256sum --check archive.sha256")
	extract := strings.Index(text, "tar -xzf")
	install := strings.Index(text, `install -m 0755 "$temporary/extracted/jobdock"`)
	if checksum < 0 || extract < checksum || install < extract {
		t.Fatal("CLI installer must verify the archive before extracting and installing it")
	}
	for _, required := range []string{"uname -s", "uname -m", "jobdock-cli_${VERSION}_${platform}.tar.gz", "SHA256SUMS"} {
		if !strings.Contains(text, required) {
			t.Fatalf("CLI installer missing %s", required)
		}
	}
}

func TestReleaseWorkflowPackagesAndSmokesCLI(t *testing.T) {
	contents, err := os.ReadFile("../.github/workflows/release-images.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{"cli-package:", "arch: [amd64, arm64]", "CGO_ENABLED=0 GOOS=linux GOARCH=\"$ARCH\"", "-X main.version=${VERSION}", "--version)\" = \"jobdock ${VERSION} (server API v1)", "jobdock-cli_${VERSION}_linux_${ARCH}.tar.gz", "server_api: \"v1\""} {
		if !strings.Contains(text, required) {
			t.Fatalf("release workflow missing %s", required)
		}
	}
}
