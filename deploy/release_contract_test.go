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
		"quality:",
		"uses: ./.github/workflows/ci.yml",
		"needs: [validate, quality]",
		"steps.build.outputs.digest",
		"actions/upload-artifact@v4",
		"release-manifest.json",
		"schema_version: 1",
		"--arg commit \"$GITHUB_SHA\"",
		"server_digest:",
		"agent_digest:",
		"builder_digest:",
		"release:",
		"needs: [validate, verify]",
		"gh release create",
		"deploy/prepare-release-assets.sh",
		"release-assets/docker-compose.yml",
		"release-assets/install-agent.sh",
		"release-assets/SHA256SUMS",
		"--generate-notes",
		"--latest=false",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow is missing %q", required)
		}
	}
	if count := strings.Count(workflow, "uses: docker/build-push-action@v6"); count != 1 {
		t.Fatalf("release workflow declares %d image build steps, want one matrix build step", count)
	}
	ci := readReleaseFile(t, ".github", "workflows", "ci.yml")
	if !strings.Contains(ci, "workflow_call:") || !strings.Contains(ci, "branches: ['**']") {
		t.Fatal("required CI must be reusable by releases and must not run as an independent tag workflow")
	}
	if _, err := os.Stat(filepath.Join("..", ".github", "workflows", "release-agent.yml")); !os.IsNotExist(err) {
		t.Fatal("legacy agent-only release workflow must not coexist with the release set")
	}
	template := readReleaseFile(t, "deploy", "docker-compose.release.yml.tmpl")
	for _, required := range []string{"@@JOBDOCK_SERVER_REFERENCE@@", "@@JOBDOCK_BUILDER_REFERENCE@@"} {
		if !strings.Contains(template, required) {
			t.Fatalf("release Compose template is missing %q", required)
		}
	}
	assets := readReleaseFile(t, "deploy", "prepare-release-assets.sh")
	for _, required := range []string{"release-manifest.json", "docker-compose.yml", "install-agent.sh", "SHA256SUMS", "## Highlights", "## Changes", "sha256sum"} {
		if !strings.Contains(assets, required) {
			t.Fatalf("release asset packager is missing %q", required)
		}
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
