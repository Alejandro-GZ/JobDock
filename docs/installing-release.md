# Install a published release

This is the supported installation path for an administrator who does not
want a source checkout or development toolchains. It uses only assets attached
to one stable GitHub Release and version-matched images from GHCR.

## Requirements

- Linux amd64.
- Docker Engine with the Docker Compose plugin.
- `curl` and `sha256sum`.
- Outbound access to GitHub Releases and `ghcr.io`.
- NVIDIA Container Toolkit on hosts that will run GPU jobs.

Go, Node.js, Python, Git, and the JobDock repository are not required.

## Download and verify one version

Choose a stable version without a prerelease suffix and work in a new
directory. Do not combine assets from different versions.

```sh
VERSION=0.1.0
RELEASE_URL="https://github.com/alejandro-gz/JobDock/releases/download/v${VERSION}"
mkdir "jobdock-${VERSION}"
cd "jobdock-${VERSION}"

curl --fail --location --remote-name "${RELEASE_URL}/SHA256SUMS"
awk '{print $2}' SHA256SUMS | while read -r asset; do
  curl --fail --location --remote-name "${RELEASE_URL}/${asset}"
done
sha256sum --check SHA256SUMS
chmod 0755 install-agent.sh
```

`release-manifest.json` records the tag, source commit, immutable digest of the
server, builder, and agent, and the exact Python SDK version and distribution
hashes. The downloaded Compose file contains the server and builder digest
references directly and contains no `build:` instructions. Install the matching
SDK independently with the command shown in the release notes, for example
`pip install jobdock-sdk==0.3.0`.

## Start the control plane and builder

Create a private `.env` file. Use strong, independently generated values for
both secrets and configure the public HTTPS URL that agents will reach.

```dotenv
JOBDOCK_PUBLIC_URL=https://dock.example.com
JOBDOCK_ALLOW_INSECURE_HTTP=false
JOBDOCK_HTTP_PORT=8080
JOBDOCK_BOOTSTRAP_ADMIN_USERNAME=admin
JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD=replace-with-at-least-12-characters
JOBDOCK_BUILDER_TOKEN=replace-with-a-random-token-of-at-least-32-characters
```

Protect the file, pull the pinned images, and start the release:

```sh
chmod 0600 .env
docker compose --env-file .env -f docker-compose.yml pull
docker compose --env-file .env -f docker-compose.yml up -d
docker compose --env-file .env -f docker-compose.yml ps
```

Terminate TLS at a trusted reverse proxy before exposing JobDock. Plain HTTP
is only appropriate for explicit local evaluation.

## Enroll an execution host

Sign in, create a one-use enrollment token in **Nodes**, copy
`install-agent.sh` to the Linux amd64 Docker host, and run it within the token's
15-minute lifetime.

CPU host:

```sh
sudo ./install-agent.sh \
  --server https://dock.example.com \
  --token YOUR_ONE_TIME_TOKEN \
  --name cpu-01 \
  --labels zone=lab
```

NVIDIA host:

```sh
sudo ./install-agent.sh \
  --server https://dock.example.com \
  --token YOUR_ONE_TIME_TOKEN \
  --name gpu-01 \
  --labels zone=lab \
  --gpu
```

The attached installer defaults to the agent digest recorded for the same
release as the server. It does not use `latest`. Passing `--version` is an
explicit override and should only be used when following a documented upgrade
or compatibility procedure.

Verify enrollment with `docker logs -f jobdock-agent` and confirm the node
heartbeat in **Nodes**.

## Development is a separate flow

Repository development uses `.env.example` and
`deploy/docker-compose.yml`, then supplies `--build` to build local Dockerfiles.
That workflow requires the source tree and development toolchains and must not
be substituted for the release procedure above. Conversely, the downloadable
release Compose file has no local build context and can be run from any clean
directory containing its verified release assets.
