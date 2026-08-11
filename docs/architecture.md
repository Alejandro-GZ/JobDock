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
entrypoint summary. Confirmation records user intent but does not start image
construction; the isolated BuildKit executor owns that later lifecycle step.

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
both streams share a persisted monotonic cursor. Historical queries read at a
cursor-consistent snapshot, while a resumable SSE tail carries only newly
committed batches to the browser. This keeps live refresh work proportional to
new telemetry rather than total job history.
raw Docker Stats documents never enter persistence. This keeps future reruns
unambiguous while allowing the UI to share one chart model for both channels.

## Protocol compatibility

The MVP protocol version is `1`. Agents send `X-JobDock-Protocol-Version` and advertise their semantic software version. An incompatible protocol must fail visibly instead of silently degrading behavior.
