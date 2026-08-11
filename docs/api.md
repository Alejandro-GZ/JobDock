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
