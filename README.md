# JobDock

JobDock is a lightweight, self-hosted control plane for running containerized batch jobs across a private pool of Docker hosts.

The repository contains:

- `jobdock-server`: the API, scheduler, persistent state, artifact store, and web application.
- `jobdock-agent`: a trusted per-host Docker executor.
- `jobdock-sdk`: an optional Python telemetry SDK for running jobs.

## Quick start

1. Copy `.env.example` to `.env` and replace every development credential.
2. Start the control plane with `docker compose --env-file .env -f deploy/docker-compose.yml up --build`.
3. Sign in at `http://localhost:8080`.
4. Create a one-time enrollment token in **Nodes**.
5. On each Docker host, set `JOBDOCK_SERVER_URL`, `JOBDOCK_ENROLLMENT_TOKEN`, and a unique `JOBDOCK_NODE_NAME`.
6. Start a CPU agent with `docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.agent.yml up -d --build`, or add `-f deploy/docker-compose.agent.gpu.yml` for NVIDIA discovery.

The explicit `--env-file .env` is required because the Compose files live under `deploy/`, while the development environment file lives at the repository root.

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

The agent is intentionally privileged through its Docker socket even though it does not use Docker's `privileged` container mode. Run it only on trusted hosts and never pass the socket to job containers.
