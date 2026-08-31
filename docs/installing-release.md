# Install a published release

This is the supported path from a Linux host with Docker to a healthy JobDock
control plane. It consumes only assets and immutable JobDock image references
from one stable GitHub Release.

## Requirements

- Linux amd64.
- Docker Engine with the Docker Compose plugin and a running daemon.
- `curl` and `sha256sum`.
- Outbound access to GitHub Releases and `ghcr.io`.
- Root access for the supported system directories.
- NVIDIA Container Toolkit only on execution hosts that will run GPU jobs.

Go, Node.js, Python, Git, and the JobDock repository are not required.

Source builds are enabled by default and remain the recommended installation. For a smaller control plane that only runs prebuilt OCI images, add `--builder disabled` to the installer command. The server stays fully operational while `jobdock-builder` and BuildKit are omitted. Re-run the same versioned installer with `--builder enabled` later to activate source builds without reinstalling or migrating data.

## Install the current stable release

For a public domain, point its A/AAAA record at the host, allow inbound TCP
80/443 and UDP 443, then run:

```sh
curl -fsSL https://github.com/Alejandro-GZ/JobDock/releases/latest/download/install-control-plane.sh \
  | sudo sh -s -- --mode domain --domain dock.example.com
```

The bootstrap performs all preflight checks before modifying system paths. It
then resolves the stable release tag, downloads only `SHA256SUMS`, the release
manifest, the digest-pinned Compose files, Caddy configuration, and the matching agent installer. It
verifies `SHA256SUMS` before installing any downloaded payload.

After starting the services, it waits for `/health/ready`. Success is reported
only after readiness passes, together with the web URL and a one-time setup
token. Open the web console, enter that token, and choose the permanent
administrator username and password. The token becomes unusable as soon as the
first administrator is created. Store it when it is printed; a same-version
reinstall does not print or replace it.

## Install an explicit version

Pass a stable semantic version after the shell command:

```sh
curl -fsSL https://github.com/Alejandro-GZ/JobDock/releases/latest/download/install-control-plane.sh \
  | sudo sh -s -- --version 0.3.0 --mode domain --domain dock.example.com
```

The selected tag controls every downloaded asset. The generated Compose file
contains the server and builder references by immutable digest. It does not use `latest`
for installed JobDock components.

## Exposure modes

`domain` is the recommended internet-facing mode. It runs Caddy, exposes only
80/443, obtains and renews the certificate automatically, redirects HTTP to
HTTPS, and applies HSTS. The JobDock server port remains private to Compose.
Certificate failures report the domain and the DNS/port checks to perform.

`proxy` is for an existing reverse proxy on the same host. It does not start
Caddy and binds JobDock to loopback by default:

```sh
curl -fsSL https://github.com/Alejandro-GZ/JobDock/releases/latest/download/install-control-plane.sh \
  | sudo sh -s -- --mode proxy --public-url https://dock.example.com --port 8080
```

Forward the original `Host`, `X-Forwarded-Proto`, and `X-Forwarded-For` headers.
Disable response buffering for SSE, especially `/api/v1/jobs/*/series/stream`,
log tails, and job update streams. Configure HTTPS redirects and HSTS at that
proxy, not in JobDock.

`local` is intended only for trusted LAN/development use and requires an
explicit insecure opt-in:

```sh
curl -fsSL https://github.com/Alejandro-GZ/JobDock/releases/latest/download/install-control-plane.sh \
  | sudo sh -s -- --mode local --allow-insecure-http --port 8080 \
      --public-url http://dock.internal:8080
```

Do not expose local mode to an untrusted network. The chosen mode is persisted;
reinstallation reuses it and refuses an accidental mode change.

## Host layout and repeatability

The installer is independent of the current working directory and uses:

| Path | Purpose |
| --- | --- |
| `/etc/jobdock` | Effective Compose, release manifest, non-secret defaults, overrides and install state |
| `/etc/jobdock/secrets` | File-mounted setup, encryption and internal service credentials |
| `/var/lib/jobdock` | Persistent server, builder and BuildKit data |
| `/usr/local/lib/jobdock/releases/VERSION` | Verified, versioned release assets |

Configuration is therefore separate from persistent data and from versioned
assets. Reinstalling the same version re-verifies the published assets,
reconciles the Compose services, and preserves the existing configuration,
credentials, database, logs, artifacts, and build cache. Installing a different
version through this command is refused until the supported upgrade flow is
used.

Setup, master-key, and builder credentials are generated from the operating
system CSPRNG. They are stored as restrictive files and mounted into only the
services that consume them; they are not persisted as container environment
values. The server and builder read those values through their corresponding
`JOBDOCK_*_FILE` settings.

Advanced tuning belongs in `/etc/jobdock/overrides.env`. The bootstrap loads it
after the generated defaults and validates the effective Compose configuration
before pulling or starting services. Do not edit the generated Compose file.

If image pull, startup, or readiness fails, the bootstrap exits non-zero and
prints the exact Compose command needed to inspect status or logs. It never
announces a partially healthy installation as successful.

## Diagnose a host or installation

Every release publishes the read-only `jobdock-doctor` command, and the
installer places it in `/usr/local/bin`. Run it before or after installation:

```sh
jobdock-doctor
jobdock-doctor --json
jobdock-doctor --gpu
```

It checks the supported OS/architecture, Docker and Compose, memory, storage,
permissions, GitHub/GHCR reachability, exposure URL, DNS/TLS, server, builder,
BuildKit, and—when explicitly requested—the NVIDIA driver plus a real GPU
container. Every failed check includes remediation. `--json` emits schema
version 1 for automation. The command never repairs or modifies the host.

## Inspect the installation

The generated project can be inspected without a repository checkout:

```sh
sudo docker compose \
  --project-name jobdock \
  --env-file /etc/jobdock/jobdock.env \
  --env-file /etc/jobdock/overrides.env \
  -f /etc/jobdock/docker-compose.yml \
  -f /etc/jobdock/docker-compose.exposure.yml \
  ps
```

`/etc/jobdock/release-manifest.json` records the tag, source commit, immutable
digests for server, agent, and builder, the precompiled CLI platform and hash,
and the matching Python SDK hashes.

## Enroll an execution host

Sign in, create a one-use enrollment token in **Nodes**, and use the agent
installer stored with the installed release:

```sh
sudo /usr/local/lib/jobdock/releases/0.3.0/install-agent.sh \
  --server https://dock.example.com \
  --token YOUR_ONE_TIME_TOKEN \
  --name cpu-01 \
  --labels zone=lab
```

Add `--gpu` on a correctly configured NVIDIA execution host. The installed
agent script defaults to the agent digest from the same release as the control
plane.

## Upgrade the control plane

Download `install-control-plane.sh` from the target release and run it against the existing system layout. It verifies the target manifest, checksums, and immutable images, then displays the version and database-schema plan before changing configuration:

```sh
sudo sh install-control-plane.sh --version 0.4.0 --upgrade
```

Add `--yes` for non-interactive automation. By default, the installer creates and validates a consistent pre-upgrade backup through `jobdockctl`; `--no-backup` is an explicit high-risk opt-out. A release that raises the SQLite schema is classified as irreversible and also requires `--allow-irreversible` after its release notes have been reviewed.

The upgrade preserves `jobdock.env`, `overrides.env`, secrets, persistent data, and the exposure mode. Its plan and outcome are recorded under `/usr/local/lib/jobdock/releases/upgrade-history`. If readiness fails before an irreversible migration, the previous release configuration is restored automatically. Agent upgrades remain separate and should be rolled out gradually after the control plane is healthy.

## Development is a separate flow

Repository development uses `.env.example` and
`deploy/docker-compose.yml`, then supplies `--build` to compile local source.
That flow requires the checkout and development toolchains. The official
bootstrap never builds JobDock locally and never depends on the directory from
which it is invoked.
