# Testing JobDock

## Isolated builder acceptance

The default Compose stack includes `jobdock-builder` and rootless BuildKit. Set
`JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD` and a random
`JOBDOCK_BUILDER_TOKEN` (minimum 32 characters), then run:

```sh
docker compose -f deploy/docker-compose.yml up --build -d
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs -f jobdock-builder buildkitd
```

Confirm a supported Railpack project and verify that its build moves from
`ANALYZING` to `BUILDING` and then `SUCCEEDED`, build logs grow while the solve
is running, and the final result is a `sha256:` digest. Exercise Dockerfile mode
through `POST /api/v1/builds` with `mode=DOCKERFILE`, a source-relative
`context_path`, and `dockerfile_path`, then confirm the build. In New job,
verify all three source choices and complete `Build & Run` for both managed
modes; the resulting job image must be an immutable managed artifact reference.
Cancel a long-running build and verify that the builder terminates it on the
next heartbeat. Restart `jobdock-server` during a confirmed build and verify
the same assignment resumes without a second build row.

The default suites are hermetic and do not require Docker:

```sh
go test ./...
go vet ./...
(cd web && npm test && npm run build)
(cd sdk/python && python -m pytest)
```

## Real-Docker distributed lifecycle suite

`tests/e2e` runs the compiled server and agent as separate Linux processes and
uses a real Docker Engine for every workload. It covers:

- submit, scheduling, execution, log ingestion, output upload, and ZIP archive;
- immutable input upload, manifest verification, read-only mounting, limits, and cleanup;
- idempotent rerun with numbered attempts and isolated logs/output archives;
- cooperative stop;
- server process loss and restart with persistent state;
- agent process loss and restart with persistent state;
- `LOST` detection followed by reconciliation of the same attempt;
- image-pull failure reporting; and
- output-upload failure reporting without terminating a successful workload.

The suite is intentionally behind the `e2e` build tag and an explicit opt-in so
ordinary unit tests never access the Docker socket. Run it on Linux with:

```sh
mkdir -p .e2e/bin
go build -o .e2e/bin/jobdock-server ./cmd/jobdock-server
CGO_ENABLED=1 go build -o .e2e/bin/jobdock-agent ./cmd/jobdock-agent
docker pull alpine:3.20
JOBDOCK_E2E=1 \
JOBDOCK_E2E_SERVER_BIN="$PWD/.e2e/bin/jobdock-server" \
JOBDOCK_E2E_AGENT_BIN="$PWD/.e2e/bin/jobdock-agent" \
JOBDOCK_E2E_DOCKER_SOCKET=/var/run/docker.sock \
go test -tags=e2e ./tests/e2e -v -count=1 -timeout=12m
```

The agent has host-equivalent Docker control during this test. Use only an
isolated development or CI Docker Engine. Cleanup targets only containers whose
exact JobDock job IDs were created by the current test run. On failure, server
and agent logs are emitted into the Go test output.

## Managed image acceptance

Confirm that the builder publishes its image archive before reporting
`SUCCEEDED`. Create a job with the returned
`jobdock://build/...@sha256:...` reference and verify that the assigned agent
downloads it with its node credential, checks its size and SHA-256, imports it
through Docker's image-load API and runs without registry authentication.
Rerunning the job must preserve the digest. GC tests must retain referenced
artifacts and remove only expired, unreferenced records and files.
