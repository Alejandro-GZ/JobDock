# Security policy and threat model

JobDock v0.1 is for trusted small teams running cooperative workloads. Docker containers are not equivalent to virtual machines and are not a sufficient boundary for hostile public multi-tenancy.

## Trust boundaries

- `jobdock-server` is a control-plane security boundary and must never mount the Docker socket.
- `jobdock-agent` is trusted host-administrative software. Its Docker socket access is effectively root-equivalent even when the container itself is not privileged.
- Job containers receive no Docker socket, privileged mode, host network, host PID namespace, added Linux capabilities, or published ports.
- Attempt credentials are mounted as read-only files and scoped to one job.

Secrets are encrypted with AES-256-GCM. The master key lives outside SQLite and must be protected and backed up separately. Official deployments keep the master key, first-run token, and builder credential in restrictive files mounted only into their consuming services; they are not persisted in container environment values. The first-run token becomes unusable after the initial administrator is created. Secret values are never returned to browser clients. Environment injection into jobs is available only as an explicit compatibility choice because container configuration can expose environment data to host administrators.

The official `domain` deployment terminates TLS at its private Caddy edge. The
`proxy` deployment trusts forwarded client metadata and therefore binds to
loopback by default; never expose that backend port directly to untrusted
clients. Plain HTTP is limited to the explicit local/LAN mode.

Report suspected vulnerabilities privately to the repository maintainers. Do not open a public issue containing credentials, exploit details, or affected deployment information.
