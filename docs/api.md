# JobDock API contract

The canonical public contract is [`api/openapi.yaml`](../api/openapi.yaml). It
uses OpenAPI 3.1 and describes the browser, administrator, agent, and running-job
surfaces served under `/api/v1`.

## Authentication scopes

Browser requests use the secure session cookie and CSRF token. CLI and CI
requests use personal access tokens in the `Authorization: Bearer` header.
Personal tokens are stored only as hashes, can expire or be revoked, and grant
one or more of `nodes:read`, `jobs:read`, `jobs:write`, `logs:read`, and
`artifacts:read`. The plaintext secret is returned only by the create response.

- Browser operations use the `jobdock_session` cookie. Mutations also require
  the session's `X-CSRF-Token` value.
- Agent operations use the revocable node bearer credential returned by
  enrollment.
- Job-context operations use the attempt-scoped bearer token mounted in the job
  container. They cover progress, metrics, parameters, events, cancellation,
  and durable checkpoint synchronization.

The contract includes local user administration, byte-offset log reads and
uploads, node enrollment/drain/resume/metadata operations, and every SDK
job-context endpoint. Secret values and agent/job credentials are write-only and
must not be emitted by generated clients in logs.

`GET /api/v1/nodes` remains the shared, polling-friendly capacity list.
`GET /api/v1/nodes/{nodeId}` returns the persisted operational host inventory,
active scheduler reservations and the latest resource observation for each
current attempt. Administrators receive all per-job observations. Members can
see the shared job ID, name, state and reservation, but observed values and job
links are present only for jobs they own. Aggregate node usage remains available
as pool-capacity information. Telemetry freshness is explicit and missing data
is never presented as zero usage.

NVIDIA inventory reports an instantaneous NVML GPU-busy sample for compatibility
and, when the agent advertises `gpu_window_telemetry_v1`, a ten-second average,
peak, sample timestamp and sample count. These host-wide readings are distinct
from per-job resource telemetry and from Windows WDDM engine counters.

## Time-series queries

Scalar metrics accept optional `timestamp`, `unit`, bounded JSON `metadata`, and
semantic `tags`. Unit, metadata, and tags are stable descriptors for a metric
name within one attempt;
omitting them inherits the descriptor, while a conflicting explicit descriptor
rejects the entire batch. Legacy `{name, value, step}` payloads remain valid.
The server derives `attempt_id` from the job credential and returns descriptors
through snapshots, incremental updates, and CSV exports. Tags are canonicalized
to lowercase, sorted, deduplicated, and stored on the descriptor rather than on
every time-series sample.

SDK metrics are stored as attempt-scoped structured samples and are available
from `GET /api/v1/jobs/{jobId}/metrics`. Automatic CPU, memory, GPU utilization,
and GPU memory samples are available from
`GET /api/v1/jobs/{jobId}/resources`. Both endpoints enforce normal job
authorization, accept RFC3339 `from`/`to` windows and bounded `limit` values,
and return the effective resolution and a `truncated` indicator. Passing
`format=csv` returns the same bounded selection as a CSV download.

Metric queries accept repeated `name` filters and
`resolution=auto|raw|1m|5m`. Resource queries accept
`resolution=auto|5s|5m`. `attempt_id` defaults to the current attempt and is
validated against the job, which prevents series from different reruns being
combined.

The metric response includes exact `last`, `min`, `max`, `avg`, and
`sample_count` summaries for each selected series. These summaries cover the
authorized time window and remain independent of point downsampling and the
bounded response limit, allowing scalar dashboard widgets to avoid downloading
full histories.

`GET /api/v1/jobs/{jobId}/metrics/catalog` discovers attempt-scoped metric
sources from descriptors only. It returns `name`, `type`, `unit`, `tags`,
bounded metadata, optional structural phase/milestone scope, and explicit
`declared`/`observed` state without loading samples. Repeating `tag` applies AND semantics;
for example `?tag=metric:loss&tag=phase:validation` returns only series carrying
both dimensions. Legacy untagged metrics remain visible in the unfiltered
catalog and simply do not match a tag filter.

Running jobs may send `POST /api/v1/job-context/observability/manifest` with a
versioned, bounded list of expected source schemas and stable pipeline phases.
The same endpoint supports idempotent runtime extension: identical declarations
are no-ops, while new sources/phases are appended atomically. The server derives the
attempt exclusively from the job token and persists declarations separately
from samples. `GET /api/v1/jobs/{jobId}/observability/catalog` returns the
attempt-scoped union of declared metrics and richer sources with their observed
state. A declaration therefore never creates a fake point, cursor, or
timestamp. Version 1 accepts 1–256 unique type/name pairs in at most 256 KiB;
each source may carry a unit, semantic tags, bounded metadata, and either a
phase or milestone scope. A phase has a stable ID plus optional display name,
order and bounded metadata. Material changes emit one attempt-aware
`observability_manifest_updated` event; retries of an identical request emit
nothing. Incompatible changes to an existing source type/unit/tags or phase
definition return `409` rather than rewriting meaning. Jobs that never declare a manifest retain dynamic
source discovery exactly as before.

`POST /api/v1/jobs/{jobId}/dashboard/templates/resolve` evaluates a versioned
dashboard template against that same bounded catalog. The response reports each
slot as `resolved`, `missing`, `ambiguous`, or `incompatible` and returns only
fully materializable widgets in the regular dashboard widget schema. Resolution
is read-only, attempt-scoped, deterministic, and never reads metric samples.
An optional `overrides` array can select sources from an ambiguous slot's
reported candidates. Each entry is keyed by `widget_id` and `slot_id`; invalid,
duplicate, incompatible, or cardinality-breaking selections reject the request
with `invalid_dashboard_template_override`.
See [Dashboard templates](dashboard-templates.md) for the schema and matching
rules.

`GET /api/v1/dashboard/templates` returns the categorized, product-maintained,
framework-neutral catalog. Catalog retrieval does not inspect a job or load
telemetry. `GET /api/v1/observability/catalog` returns the versioned standard
metric-role and phase taxonomy.

`GET /api/v1/jobs/{jobId}/dashboard/templates/matches` evaluates the complete
official catalog against one attempt with a single bounded descriptor read and
reports which templates can be applied immediately.

Template IDs are stable. `version` identifies a revision of one template while
`schema_version` identifies the declarative language. Resolution reports
`compatible`, `partially_compatible`, or `incompatible`; unsupported schemas and
widget types use a machine-readable fallback instead of failing dashboard load.

`PUT /api/v1/jobs/{jobId}/dashboard` accepts optional `materialized_from`
provenance. Omitting it preserves the current origin during normal edits,
sending an object records a newly applied template, and sending `null` detaches
the origin explicitly. `GET` returns this provenance plus the saved dashboard's
compatibility and any controlled fallback reason.

Jobs can contain up to 32 independent dashboards per user. Use
`GET /api/v1/jobs/{jobId}/dashboards` to list their stable IDs, names, ordering,
active selection, and deterministic default. `POST` creates an empty dashboard;
providing `source_dashboard_id` duplicates only configuration and template
provenance, never attempt telemetry. The item endpoint supports `GET`/`PUT` for
configuration, `PATCH` for rename, activation, and default selection, and
`DELETE`. The final dashboard cannot be deleted. Removing the active or default
dashboard selects the earliest remaining default/order entry atomically.

The singular `/dashboard` endpoint remains a compatibility alias for the active
dashboard. Dashboard configuration is job-scoped and therefore survives reruns;
source availability, samples, and live streams remain explicitly attempt-scoped.
Switching between dashboards reuses cached bounded series queries when the
visible source set and attempt are unchanged.

Jobs without inputs use the regular JSON create request. To attach reproducible
inputs, `POST /api/v1/jobs` accepts `multipart/form-data` with one `spec` JSON
field and file fields named `input:<relative-path>`. The server stores every
file before the job becomes visible to the scheduler, generates an immutable
size/SHA-256 manifest, and rejects client-supplied manifests. Assigned agents
may download only paths present in that manifest.

## Reruns and attempt history

`POST /api/v1/jobs/{jobId}/rerun` reuses the persisted `JobSpec` and queues the
same job for another execution. It accepts an idempotency key and only permits
`SUCCEEDED`, `FAILED`, or `CANCELLED` jobs; `LOST` is deliberately excluded
because its container may still exist. `GET /api/v1/jobs/{jobId}/attempts`
returns newest-first numbered executions with their node, timestamps, exit
code, image digest, failure reason, and output manifest.

Logs, events, metrics, and resources accept `attempt_id`. Attempt ZIPs are
available from `/jobs/{jobId}/attempts/{attemptId}/archive.zip`. Agent event,
telemetry, log, and output uploads carry the assigned attempt identity; stale
requests from a previous execution are rejected instead of contaminating the
current one.

Each JSON snapshot includes a common, attempt-scoped `cursor`. Supplying that
cursor back to either query fixes the result to the same persistence boundary.
`GET /api/v1/jobs/{jobId}/series/stream?after=<cursor>` then emits only metric
batches and resource samples committed after that boundary. The SSE event ID is
the next cursor, so browser reconnection through `Last-Event-ID` is resumable
without downloading the historical window again.

## Drift protection

`TestOpenAPICoversRegisteredAPI` extracts every literal `/api/v1` method and
route registered by the Go server and compares it with the OpenAPI path table in
both directions. It also requires every operation to define an `operationId` and
responses. `TestOpenAPIComponentReferencesResolve` rejects duplicate operation
IDs and unresolved schema, parameter, or response references, while
`TestOpenAPIDomainSchemasCoverJSONFields` ensures the core Go response types do
not gain undocumented JSON fields.

CI runs the contract check explicitly before the race-enabled Go suite. Any
server route change must therefore update `api/openapi.yaml` in the same commit.
This makes the checked-in document safe to use as the input to an OpenAPI 3.1
client generator.
