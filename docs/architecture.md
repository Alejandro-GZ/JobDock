# JobDock architecture

JobDock is a modular monolith with remote, pull-based execution agents.

```text
Browser / SDK ──HTTPS──> jobdock-server ──SQLite + filesystem
                            ▲
                            │ authenticated long polling and uploads
                            │
                   jobdock-agent (trusted container)
                            │ local Docker socket
                            ▼
                       job containers
```

## Responsibilities

The server owns authoritative desired state, authorization, resource reservations, scheduling, audit history, central logs, and output storage. Agents own observed container state, local spooling, Docker lifecycle operations, resource samples, and reconciliation. A job container has a single attempt-scoped credential and cannot manage other jobs or nodes.

The server never connects to a remote Docker daemon. Agents initiate every network connection, which supports hosts behind NAT and avoids exposing Docker TCP endpoints.

## Scheduling

Scheduling first removes offline/draining nodes, label mismatches, and nodes without unreserved CPU, memory, or GPU capacity. Best-fit then minimizes VRAM, memory, and CPU waste. Assignment and reservation are one SQLite transaction. FIFO is the only queue policy in v0.1.

Agents report NVIDIA devices by UUID and Linux CPU packages with their logical CPU IDs. A job may retain automatic best-fit placement or select a target node plus exact GPU UUIDs and an optional physical CPU package. Explicit selections survive reruns: busy or temporarily unavailable hardware leaves the job queued and is never substituted silently. CPU package affinity uses Docker `CpusetCpus` together with the normal CPU quota.

Agents advertising `host_inventory_v1` also report an operational, non-sensitive
host inventory on every heartbeat: Docker host and OS identity, kernel and
architecture, Docker storage and cgroup configuration, workspace capacity, CPU
vendor/package/thread topology, and NVIDIA PCI, driver, compute-capability and
live device data. JobDock deliberately does not collect firmware, board, DIMM,
disk or network serial identifiers. Older agents remain valid and expose their
existing aggregate capacity with the extended fields marked unavailable.

`LOST` means that execution is unknown, not failed. JobDock never starts another attempt automatically. If the original agent returns, Docker labels and the local assignment journal allow it to report the real state.

An explicit rerun moves a terminal job back to `QUEUED` without changing its
specification or immutable inputs. The scheduler allocates the next attempt
number and assignment atomically. Attempts retain their execution node,
timestamps, terminal result, output manifest, logs, outputs, events, and
telemetry. Every agent upload is checked against the current attempt so a
delayed retry cannot write into a later execution.

## Storage

- SQLite WAL: users, sessions, node inventory, jobs, attempts, assignments, metadata, structured SDK metric samples, compact resource samples, events, secrets, and audit records.
- Server filesystem: attempt-scoped stdout/stderr files, outputs, and generated metadata.
- Agent filesystem: assignment journal, bounded attempt workspaces, job token files, secret files, and outputs.

Source builds are modeled independently from jobs. A build owns an immutable
source archive identified by its SHA-256 digest, a lifecycle event history, and
bounded build logs. Successful builds reference an immutable OCI digest; failed
analysis or execution records a visible reason without creating an invalid job.
This boundary allows the build executor to evolve independently while the job
runtime continues to accept only OCI images.

Railpack mode expands ZIP or compressed TAR sources into a temporary confined
workspace and invokes the pinned `railpack prepare` CLI. JobDock persists both
Railpack JSON outputs plus a normalized provider, runtime, package-manager and
entrypoint summary. Confirmation atomically creates a durable build assignment
and moves the build to `BUILDING`. A separately deployed `jobdock-builder`
claims that assignment with a renewable lease, verifies the immutable source
archive, and invokes `buildctl` against a dedicated rootless BuildKit daemon.
Railpack builds use the persisted plan through the pinned Railpack frontend;
Dockerfile builds use the built-in Dockerfile frontend. The builder uploads
ordered log chunks and the terminal OCI digest through a narrowly scoped API.
It never receives a browser session, administrative credential, server
filesystem mount, or Docker socket.

Build assignments, desired cancellation, ownership and leases live in SQLite.
The builder identity and current assignment are persisted in its own volume.
Consequently, a server restart leaves confirmed work unchanged and a builder
restart reclaims the same assignment; BuildKit's persistent cache makes the
repeated solve safe. After a successful solve, the builder publishes a
Docker-compatible image archive to central storage before it confirms the
build. JobDock persists its OCI digest, archive SHA-256, size and owner.

Jobs address managed images as
`jobdock://build/<build-id>@sha256:<digest>`. Assigned agents download the
archive with their scoped node credential, verify its bytes and import it into
their local Docker Engine. Internal Docker names are never part of the public
API or UI. Job creation validates artifact ownership and the exact digest in
the same database transaction, which also prevents races with garbage
collection. Reruns retain the same reference; creating a new build generation
is the explicit rebuild boundary.

The New job wizard exposes that pipeline as three execution-source choices.
`Project (Auto)` is the recommended path and pauses after Railpack analysis so
the detected provider, runtime, package manager, and entrypoint can be reviewed.
`Dockerfile` records a normalized source-relative build context and Dockerfile
path. `OCI image` keeps the direct-image and optional registry-credential flow.
Runtime command and working-directory fields are explicit overrides; leaving
them empty preserves the image defaults. Both managed paths finish the build,
resolve its immutable `jobdock://` reference, and submit the same `JobSpec`
used by the direct OCI path.

Job inputs are committed to central storage before `QUEUED` is persisted. Their
relative paths, sizes, and SHA-256 digests become part of the immutable job
specification. The assigned agent downloads and verifies that exact manifest,
materializes it beneath its workspace, and bind-mounts it at `/jobdock/input`
with Docker read-only semantics. Failed staging and completed local execution
remove partial/materialized input copies; explicit job deletion removes the
central generation.

Storage access is behind package boundaries so PostgreSQL and object storage can be implemented without changing the domain model.

Metrics reported by `job.metric()` are not generic lifecycle events. They are
stored in an attempt-aware time-series table with server-side aggregation and
bounded queries. Normalized resource telemetry uses the same attempt identity;
metric semantics are stored once in the stable series descriptor and are not
duplicated across samples. Tags omitted from later samples inherit that
descriptor, while an incompatible redefinition rejects the complete batch.
Metric discovery and semantic AND filters query only this bounded descriptor
catalog, so their cost is independent of the number of historical samples.
Dashboard templates are a declarative layer over that catalog. Their resolver
produces ordinary dashboard widgets plus explicit slot diagnostics; it neither
creates a second persisted dashboard model nor mutates observable definitions.
Matrix and progress availability is registered in compact rich-observable
descriptors during ingestion, allowing template resolution without reading
matrix values or progress history. Product-maintained templates remain static,
framework-neutral definitions and are copied into an ordinary dashboard only
when explicitly applied. Stable template IDs have independent definition and
schema versions. Resolution classifies full, partial, and incompatible matches;
unsupported definitions fall back without touching the saved dashboard. A
materialized dashboard stores immutable origin metadata for reproducibility,
but subsequent widget edits remain independent from the template catalog.
Both streams share a persisted monotonic cursor. Historical queries read at a
cursor-consistent snapshot, while a resumable SSE tail carries only newly
committed batches to the browser. This keeps live refresh work proportional to
new telemetry rather than total job history.
raw Docker Stats documents never enter persistence. This keeps future reruns
unambiguous while allowing the UI to share one chart model for both channels.

## Protocol compatibility

The MVP protocol version is `1`. Agents send `X-JobDock-Protocol-Version` and advertise their semantic software version. An incompatible protocol must fail visibly instead of silently degrading behavior.
