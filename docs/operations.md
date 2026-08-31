# Operations guide

## Production requirements

- Terminate TLS at JobDock or a trusted reverse proxy. Agents reject HTTP unless explicitly configured for local development.
- Keep the server data volume and agent state/workspace volumes on durable filesystems.
- Treat every agent as host-administrative software: access to the Docker socket can control the host.
- Put JobDock on a trusted private network or VPN and restrict public access at the firewall or reverse proxy.

### Exposure modes

The supported release installer requires one explicit exposure mode. `domain`
runs the versioned Caddy edge, publishes only TCP 80/443 and UDP 443, redirects
HTTP to HTTPS, renews certificates automatically, and applies HSTS. `proxy`
does not run Caddy and binds JobDock to `127.0.0.1:8080` by default; the external
proxy must pass `Host`, `X-Forwarded-Proto`, and `X-Forwarded-For`, preserve
streaming responses, and disable buffering for SSE routes. `local` publishes
plain HTTP only after the installer receives `--allow-insecure-http`; do not use
it on an untrusted network.

Only domain and proxy deployments trust forwarded client metadata. Do not make
the proxy-mode loopback port publicly reachable while proxy-header trust is
enabled. HSTS belongs at the external proxy in proxy mode and is deliberately
absent from local mode.
- Run `jobdock-builder` and `buildkitd` as separate services. Neither service may mount the host Docker socket.

### Optional source builder

The supported OCI-only mode stores `JOBDOCK_BUILDER_ENABLED=false` and leaves `COMPOSE_PROFILES` empty. Jobs based on existing OCI images remain available, while source-build actions explain that the capability is disabled. `jobdock-doctor` reports this as intentional instead of treating the missing builder services as broken. Enabling source builds sets `JOBDOCK_BUILDER_ENABLED=true` and `COMPOSE_PROFILES=builder`; the doctor then requires both builder services to be running.

## Backup and restore

The release installer places `jobdockctl` in `/usr/local/bin`. Create a consistent, portable snapshot with:

```sh
sudo jobdockctl backup --output /srv/backups/jobdock-$(date +%F).tar
```

The command checks free space, briefly stops all state writers, captures SQLite, the master key, secrets, logs, outputs, managed images, builder identity and BuildKit state, writes a versioned manifest and checksums, and restarts the deployment only after validating the archive. The resulting file is mode `0600` and must be handled as a credential.

Restore onto an empty supported host layout with `sudo jobdockctl restore BACKUP.tar`. The command validates the outer layout, hashes, payload paths, file types and database compatibility before writing anything. Use `--force` only after inspection when replacing an existing installation; the previous directories are moved to timestamped recovery paths instead of deleted. When a newer installed release is present, its generated deployment files are retained and the restored database is migrated by normal server startup. Downgrades from a newer backup schema are rejected.

## Isolated source builder

The official installer generates a random builder credential and mounts
separate restrictive copies into the server and builder through
`JOBDOCK_BUILDER_TOKEN_FILE`. For manual development deployments, the legacy
`JOBDOCK_BUILDER_TOKEN` environment value remains supported. Treat it as a scoped service
credential: it can claim build work, download only assigned source archives,
append build logs, and report results, but it cannot access users, secrets,
jobs, nodes, or administrative APIs.

The supplied Compose deployment runs one build at a time through
`moby/buildkit:v0.30.0-rootless`. It uses no host Docker socket and separates
the server/builder control network from the builder/BuildKit network. Configure
the execution boundary with:

- `JOBDOCK_BUILD_CPU_LIMIT` (default `2.0` CPUs);
- `JOBDOCK_BUILD_MEMORY_LIMIT` (default `4g`);
- `JOBDOCK_BUILD_PID_LIMIT` (default `1024`);
- `JOBDOCK_BUILD_CACHE_LIMIT_MB` (default `20480` MiB, enforced by BuildKit GC);
- `JOBDOCK_BUILD_TIMEOUT` (default `30m`);
- `JOBDOCK_MAX_BUILD_ARTIFACT_BYTES` (default `20 GiB`);
- `JOBDOCK_BUILD_ARTIFACT_RETENTION` (default `30 days` for unreferenced artifacts).

The builder volume stores its stable identity and current assignment. A local
image archive is removed only after the server confirms its managed copy. The
BuildKit volume stores cache and in-progress solve state.
Keep both persistent across ordinary restarts. If the server restarts, the
assignment lease and build status remain in SQLite. If the builder restarts, it
reclaims its assignment and repeats the BuildKit solve against the persistent
cache instead of creating a second build record.

Verify the security boundary after deployment:

```sh
docker inspect jobdock-jobdock-builder-1 --format '{{json .Mounts}}'
docker inspect jobdock-buildkitd-1 --format '{{json .HostConfig.Resources}}'
```

The first output must not contain `/var/run/docker.sock`. Build cancellation is
cooperative at the control plane and forceful at the builder: the next lease
heartbeat cancels `buildctl`, while a late success is discarded in favor of the
persisted cancellation request.

Managed artifacts live in the server data volume. The hourly collector removes
metadata only when its retention window has elapsed and no non-deleted job
references the exact digest, then removes the archive. Back up the SQLite
database and server data directory together.

## Backup and restore

Stop the server or use SQLite's online backup mechanism, then copy the database, `jobs/`, and `/etc/jobdock/secrets/master-key` from the same point in time. The encryption key is mandatory for restoring secrets. Restore into an empty data directory with the same or newer compatible JobDock version. Legacy development installs may still keep `master.key` inside the server data directory.

## Capacity defaults

- 10 GiB of persisted logs per job.
- 100 GiB of outputs per job.
- 10 GiB of immutable inputs per job and at most 1,024 input files.
- An agent should stop accepting work below the greater of 10 GiB or 10% workspace free space.
- Resource telemetry is sampled every five seconds.

Resource telemetry stores only normalized CPU millicores, memory bytes, average GPU utilization, and GPU memory bytes. Full Docker Stats documents are discarded in the agent and rejected by the server. Five-second samples are retained for 24 hours, then averaged into five-minute buckets and retained for 30 days. Configure these windows with `JOBDOCK_TELEMETRY_RAW_RETENTION` and `JOBDOCK_TELEMETRY_RETENTION`; the final retention must not be shorter than the raw window. Public resource and SDK metric queries are attempt-scoped and capped at 10,000 returned points; the UI uses a 2,000-point window and server-selected resolution by default. Live charts use a resumable SSE connection and append deltas locally, so reverse proxies must disable response buffering for `/api/v1/jobs/*/series/stream`.

Job limits are controlled with `JOBDOCK_MAX_LOG_BYTES`, `JOBDOCK_MAX_OUTPUT_BYTES`, and `JOBDOCK_MAX_INPUT_BYTES`. Input limits are checked before a job is queued; log/output limit events are visible in the job timeline, and workloads are not killed merely because central collection reached a quota. Build execution limits are configured independently as described above.

## Upgrades

Back up the server, upgrade the control plane, verify readiness, then upgrade agents gradually. Do not skip major protocol versions. A draining node receives no new work and can be upgraded after active jobs finish.

## Enrolling a Docker host

Open **Nodes → Enroll node** to obtain separate copy-ready CPU and NVIDIA commands. The server renders `/install-agent.sh` from its verified release manifest, so the default image is the immutable agent digest matching the running server and never an ambiguous `latest`. Enrollment tokens expire after 15 minutes, cannot be reused, and can be regenerated or revoked before use. Treat the command as a temporary credential and avoid shared shell history.

The installer waits for server-confirmed enrollment and a heartbeat before reporting success. If registration does not complete within 60 seconds it prints recent agent logs and an actionable connectivity/TLS/token error instead of announcing a partial installation.

Running the matching command again is idempotent. The installer detects `install`, `repair`, or `upgrade`, preserves `/var/lib/jobdock-agent/credential.json` and supported server/name/labels/GPU configuration, and keeps the previous container as a rollback candidate until the replacement authenticates. Upgrades are rejected while local assignment records are active unless the operator deliberately supplies `--force-active`; drain the node first in normal operation. Use `--repair` to recreate the current image, or run the installer served by an upgraded control plane to pull its new immutable digest. `--gpu` and `--no-gpu` declaratively change NVIDIA discovery.

CPU host:

```sh
sudo ./install-agent.sh --server https://dock.example.com --token YOUR_ONE_TIME_TOKEN --name cpu-01 --labels zone=lab
```

NVIDIA GPU host:

```sh
sudo ./install-agent.sh --server https://dock.example.com --token YOUR_ONE_TIME_TOKEN --name gpu-01 --labels zone=lab --gpu
```

The host must have outbound access to the JobDock URL and GHCR. GPU hosts additionally require a working NVIDIA driver and NVIDIA Container Toolkit. A prepared host performs only an image pull, volume creation, and container start, so no repository checkout or local build is involved. See [Install a published release](installing-release.md) for download and checksum verification. Verify enrollment with `docker logs -f jobdock-agent` and confirm the heartbeat in **Nodes**.

The installer rejects an existing `jobdock-agent` container instead of replacing it implicitly. Drain the node, remove the old container explicitly, and rerun the command with the desired `--version` during an upgrade. Persistent identity and reconciliation state remain in the `jobdock-agent-state` volume.

## Troubleshooting

- `NO_ONLINE_NODE`: verify agent credentials, server URL, TLS trust, and heartbeats.
- `NVML_UNAVAILABLE`: verify `nvidia-smi` works on the host and in a test container, then recreate the agent with all three Compose files and `--force-recreate`:

  ```text
  docker run --rm --gpus all ubuntu:24.04 nvidia-smi -L
  docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.agent.yml -f deploy/docker-compose.agent.gpu.yml up --build -d --force-recreate jobdock-agent
  docker inspect jobdock-jobdock-agent-1 --format "{{json .HostConfig.DeviceRequests}}"
  ```

- `NO_GPU_FOUND`: NVML loaded, but no complete NVIDIA device inventory was returned.
- `DISCOVERY_FAILED`: inspect the node diagnostic and agent logs for the specific NVML query that failed.
- `NO_COMPATIBLE_GPU`: confirm the requested count and minimum VRAM can fit on one online node; JobDock never combines GPUs across hosts.
- Jobs remain `LOST`: inspect Docker labels `jobdock.managed`, `jobdock.job_id`, and `jobdock.attempt_id` on the assigned host.
- Image pulls fail: store a registry secret containing Docker AuthConfig JSON and select it in the job specification.

Example registry secret value:

```json
{"username":"registry-user","password":"registry-token","serveraddress":"ghcr.io"}
```
