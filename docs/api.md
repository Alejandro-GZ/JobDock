# JobDock API contract

The canonical public contract is [`api/openapi.yaml`](../api/openapi.yaml). It
uses OpenAPI 3.1 and describes the browser, administrator, agent, and running-job
surfaces served under `/api/v1`.

## Authentication scopes

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

## Time-series queries

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
