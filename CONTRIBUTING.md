# Contributing

All code, API names, UI copy, tests, commits, and documentation must be written in English.

Before submitting a change:

1. Add or update tests for observable behavior.
2. Run `go test ./...`, the Python SDK tests, and the web typecheck/build.
3. Update `api/openapi.yaml` for public wire changes.
4. Add an architecture decision record for changes to trust boundaries, lifecycle semantics, persistence, or transport.
5. Preserve backwards compatibility within protocol version 1.

Never commit credentials, generated data directories, databases, agent state, or user job outputs.

