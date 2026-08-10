# Security policy and threat model

JobDock v0.1 is for trusted small teams running cooperative workloads. Docker containers are not equivalent to virtual machines and are not a sufficient boundary for hostile public multi-tenancy.

## Trust boundaries

- `jobdock-server` is a control-plane security boundary and must never mount the Docker socket.
- `jobdock-agent` is trusted host-administrative software. Its Docker socket access is effectively root-equivalent even when the container itself is not privileged.
- Job containers receive no Docker socket, privileged mode, host network, host PID namespace, added Linux capabilities, or published ports.
- Attempt credentials are mounted as read-only files and scoped to one job.

Secrets are encrypted with AES-256-GCM. The master key lives outside SQLite and must be protected and backed up separately. Secret values are never returned to browser clients. File-mounted secrets are preferred; environment injection is available only as an explicit compatibility choice because container configuration can expose environment data to host administrators.

Report suspected vulnerabilities privately to the repository maintainers. Do not open a public issue containing credentials, exploit details, or affected deployment information.

