<p align="center">
  <img src="docs/assets/jobdock-logo.svg" alt="JobDock" width="286">
</p>

JobDock is a lightweight, self-hosted control plane for running containerized batch jobs across a private pool of Docker hosts.

The repository contains:

- `jobdock`: the scoped command-line client for local and CI automation.
- `jobdock-server`: the API, scheduler, persistent state, artifact store, and web application.
- `jobdock-builder`: an isolated source-build worker that drives rootless BuildKit and publishes managed images without a user registry.
- `jobdock-agent`: a trusted per-host Docker executor.
- `jobdock-sdk`: an optional Python telemetry SDK for running jobs.

Official releases publish version-matched `jobdock-server`, `jobdock-agent`,
and `jobdock-builder` images to `ghcr.io/alejandro-gz`. Stable releases also
move the matching minor tag and `latest`; prereleases only publish their exact
SemVer tag. See [the release guide](docs/releasing.md).

The versioned HTTP surface and client-generation contract are documented in
[`api/openapi.yaml`](api/openapi.yaml); contributor guidance for keeping it in
sync with the Go router is in [`docs/api.md`](docs/api.md).
Real-Docker release verification is described in
[`docs/testing.md`](docs/testing.md).
Terminal and CI usage is documented in the [CLI guide](docs/cli.md).

## Install a stable release

A published release requires only Linux amd64, Docker Engine with the Compose
plugin, `curl`, and `sha256sum`. Download its four assets, verify
`SHA256SUMS`, start the supplied `docker-compose.yml`, and enroll hosts with the
supplied `install-agent.sh`. The Compose file and installer are pinned to the
verified image digests from that release; no repository clone, Go, Node.js, or
local image build is involved.

See [Install a published release](docs/installing-release.md) for the complete
server, builder, CPU-agent, and NVIDIA-agent procedure.

Production deployments must terminate TLS and set
`JOBDOCK_ALLOW_INSECURE_HTTP=false`.

See [Architecture](docs/architecture.md), [Security](SECURITY.md), and [Operations](docs/operations.md) before deploying JobDock.

## Development

Requirements: Go 1.26+, Node.js 24+, Python 3.10+, and Docker Engine 29+.

The development Compose file is intentionally different from the downloadable
release Compose file: it uses repository Dockerfiles and `--build` so local
source changes are included.

```text
cp .env.example .env
docker compose --env-file .env -f deploy/docker-compose.yml up --build
go test ./...
go run ./cmd/jobdock-server
cd web && npm install && npm run dev
cd sdk/python && python -m pytest
```

JobDock is currently an MVP and targets trusted teams. Docker containers are not a security boundary for hostile multi-tenant workloads.

The agent is intentionally privileged through its Docker socket even though it does not use Docker's `privileged` container mode. Run it only on trusted hosts and never pass the socket to job containers. The source builder is a separate boundary: neither `jobdock-builder`, BuildKit, nor user build steps receive the host Docker socket.
