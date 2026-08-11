# Release guide

JobDock publishes the agent as a versioned Linux amd64 image in GitHub Container Registry.

1. Run the full test suite and ensure the release version is reflected in the installer, Compose default, and documentation.
2. Create and push an annotated semantic-version tag such as `v0.1.0` from the reviewed release commit.
3. The `Publish agent image` workflow builds `Dockerfile.agent` with the version embedded in the binary and publishes immutable `0.1.0`, moving `0.1`, and `latest` tags. Pre-release tags do not update `latest`.
4. On the first release, set the `jobdock-agent` package visibility to public in the repository package settings. Confirm anonymous access with `docker pull ghcr.io/alejandro-gz/jobdock-agent:0.1.0`.
5. Run both documented installer commands on disposable CPU and NVIDIA hosts. Confirm enrollment and verify the reported agent version before announcing the release.

Never replace an existing immutable patch tag. If its contents are wrong, publish a new patch version.
