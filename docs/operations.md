# Operations guide

## Production requirements

- Terminate TLS at JobDock or a trusted reverse proxy. Agents reject HTTP unless explicitly configured for local development.
- Keep the server data volume and agent state/workspace volumes on durable filesystems.
- Treat every agent as host-administrative software: access to the Docker socket can control the host.
- Put JobDock on a trusted private network or VPN and restrict public access at the firewall or reverse proxy.

## Backup and restore

Stop the server or use SQLite's online backup mechanism, then copy the database, `jobs/`, and `master.key` from the same point in time. The encryption key is mandatory for restoring secrets. Restore into an empty data directory with the same or newer compatible JobDock version.

## Capacity defaults

- 10 GiB of persisted logs per job.
- 100 GiB of outputs per job.
- An agent should stop accepting work below the greater of 10 GiB or 10% workspace free space.
- Resource telemetry is sampled every five seconds.

Limits are controlled with `JOBDOCK_MAX_LOG_BYTES` and `JOBDOCK_MAX_OUTPUT_BYTES`. Limit events are visible in the job timeline; workloads are not killed merely because central collection reached a quota.

## Upgrades

Back up the server, upgrade the control plane, verify readiness, then upgrade agents gradually. Do not skip major protocol versions. A draining node receives no new work and can be upgraded after active jobs finish.

## Enrolling a Docker host

Create a one-time enrollment token from **Nodes**, then run one installer command on a Linux amd64 host with Docker ready. Enrollment tokens expire after 15 minutes and cannot be reused.

CPU host:

```sh
curl -fsSL https://dock.example.com/install-agent.sh | sudo sh -s -- --server https://dock.example.com --token YOUR_ONE_TIME_TOKEN --name cpu-01 --labels zone=lab
```

NVIDIA GPU host:

```sh
curl -fsSL https://dock.example.com/install-agent.sh | sudo sh -s -- --server https://dock.example.com --token YOUR_ONE_TIME_TOKEN --name gpu-01 --labels zone=lab --gpu
```

The host must have outbound access to the JobDock URL and GHCR. GPU hosts additionally require a working NVIDIA driver and NVIDIA Container Toolkit. A prepared host performs only an image pull, volume creation, and container start, so no repository checkout or local build is involved. Verify enrollment with `docker logs -f jobdock-agent` and confirm the heartbeat in **Nodes**.

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
