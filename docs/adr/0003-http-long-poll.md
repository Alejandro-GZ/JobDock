# ADR 0003: One-port HTTPS protocol

Status: accepted.

The browser, Python SDK, and agents use the same versioned HTTPS API. Agent orders use bounded long polling; logs, telemetry, and outputs use acknowledged uploads; browser updates use SSE. This keeps firewalls and reverse proxies simple while preserving resumability and backpressure.
