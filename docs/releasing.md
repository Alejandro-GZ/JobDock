# Release guide

JobDock publishes the server, agent, and builder as one versioned Linux amd64
release set in GitHub Container Registry:

- `ghcr.io/alejandro-gz/jobdock-server`
- `ghcr.io/alejandro-gz/jobdock-agent`
- `ghcr.io/alejandro-gz/jobdock-builder`

1. Run the full test suite and review the release notes.
2. Create and push an annotated semantic-version tag such as `v0.1.0` from the reviewed release commit.
3. The `Publish release images` workflow validates the tag, invokes the required CI workflow, and only then builds each Dockerfile once with the same version embedded in its binary and OCI labels. Every image is published with BuildKit provenance and an SBOM.
4. A stable tag publishes the immutable patch tag (`0.1.0`), the moving minor tag (`0.1`), and `latest`. A prerelease such as `v0.2.0-rc.1` publishes only `0.2.0-rc.1`; it never changes `0.2` or `latest`.
5. Each matrix build persists its published digest. The verification phase assembles these values into `release-manifest.json`, pulls those exact digests, and proves that each exact-version tag resolves to the same package. It never rebuilds an image for validation.
6. Only after all required CI, publication, and digest verification jobs succeed does the workflow create the GitHub Release. The digest manifest is retained as both a workflow artifact and a release asset.
7. On the first release, set all three package visibilities to public in the repository package settings.
8. Run the documented server/builder and CPU/NVIDIA agent installation flows on disposable hosts. Confirm component versions and agent enrollment before announcing the release.

Never replace an existing immutable patch or prerelease tag. If its contents are wrong, publish a new version.
