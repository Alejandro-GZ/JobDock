package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseWorkflowPublishesCompleteVersionedSet(t *testing.T) {
	workflow := readReleaseFile(t, ".github", "workflows", "release-images.yml")
	for _, required := range []string{
		"jobdock-server",
		"jobdock-agent",
		"jobdock-builder",
		"type=semver,pattern={{version}}",
		"type=semver,pattern={{major}}.{{minor}}",
		"type=raw,value=latest",
		"needs.validate.outputs.stable == 'true'",
		"build-args: VERSION=${{ needs.validate.outputs.version }}",
		"provenance: mode=max",
		"sbom: true",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow is missing %q", required)
		}
	}
	if _, err := os.Stat(filepath.Join("..", ".github", "workflows", "release-agent.yml")); !os.IsNotExist(err) {
		t.Fatal("legacy agent-only release workflow must not coexist with the release set")
	}
}

func TestReleaseDockerfilesEmbedVersionAndOCILabels(t *testing.T) {
	for _, name := range []string{"Dockerfile.server", "Dockerfile.agent", "Dockerfile.builder"} {
		contents := readReleaseFile(t, name)
		for _, required := range []string{"ARG VERSION=dev", "org.opencontainers.image.version", "org.opencontainers.image.source"} {
			if !strings.Contains(contents, required) {
				t.Fatalf("%s is missing %q", name, required)
			}
		}
	}
}

func readReleaseFile(t *testing.T, path ...string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(append([]string{".."}, path...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
