# Testing JobDock

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
