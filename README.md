<p align="center">
  <img src="docs/assets/jobdock-logo.svg" alt="JobDock" width="286">
</p>

JobDock is a lightweight, self-hosted control plane for running containerized batch jobs across a private pool of Docker hosts.

The repository contains:

- `jobdock-server`: the API, scheduler, persistent state, artifact store, and web application.
- `jobdock-builder`: an isolated source-build worker that drives rootless BuildKit and publishes managed images without a user registry.
- `jobdock-agent`: a trusted per-host Docker executor.
- `jobdock-sdk`: an optional Python telemetry SDK for running jobs.

The versioned HTTP surface and client-generation contract are documented in
[`api/openapi.yaml`](api/openapi.yaml); contributor guidance for keeping it in
sync with the Go router is in [`docs/api.md`](docs/api.md).
Real-Docker release verification is described in
[`docs/testing.md`](docs/testing.md).

## Quick start

1. Copy `.env.example` to `.env` and replace every development credential.
2. Start the control plane with `docker compose --env-file .env -f deploy/docker-compose.yml up --build`.
3. Sign in at `http://localhost:8080`.
4. Create a one-time enrollment token in **Nodes**.
5. On each additional Docker host, run the versioned CPU installer with the one-time token:

```sh
curl -fsSL https://dock.example.com/install-agent.sh | sudo sh -s -- --server https://dock.example.com --token YOUR_ONE_TIME_TOKEN --name cpu-01
```

For an NVIDIA host, use the same command with `--gpu`:

```sh
curl -fsSL https://dock.example.com/install-agent.sh | sudo sh -s -- --server https://dock.example.com --token YOUR_ONE_TIME_TOKEN --name gpu-01 --gpu
```

The installer pulls the latest `ghcr.io/alejandro-gz/jobdock-agent:latest` image, creates persistent agent state, and starts the constrained agent container. It does not clone this repository, invoke a compiler, or edit Compose. The GPU mode requests NVIDIA devices and requires NVML discovery; a discovery failure marks the node `DEGRADED` and prevents assignments.

For an intentionally insecure local server, append `--allow-insecure-http`. The installer otherwise requires HTTPS.

The quick start permits plain HTTP for local evaluation. Production deployments must terminate TLS and set `JOBDOCK_ALLOW_INSECURE_HTTP=false`.

See [Architecture](docs/architecture.md), [Security](SECURITY.md), and [Operations](docs/operations.md) before deploying JobDock.

## Development

Requirements: Go 1.26+, Node.js 24+, Python 3.10+, and Docker Engine 29+.

```text
go test ./...
go run ./cmd/jobdock-server
cd web && npm install && npm run dev
cd sdk/python && python -m pytest
```

JobDock is currently an MVP and targets trusted teams. Docker containers are not a security boundary for hostile multi-tenant workloads.

The agent is intentionally privileged through its Docker socket even though it does not use Docker's `privileged` container mode. Run it only on trusted hosts and never pass the socket to job containers. The source builder is a separate boundary: neither `jobdock-builder`, BuildKit, nor user build steps receive the host Docker socket.
