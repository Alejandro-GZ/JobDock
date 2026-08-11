#!/bin/sh
set -eu

DEFAULT_VERSION="latest"
IMAGE_REPOSITORY="ghcr.io/alejandro-gz/jobdock-agent"
CONTAINER_NAME="jobdock-agent"
STATE_DIR="/var/lib/jobdock-agent"

server=""
token=""
node_name="$(hostname 2>/dev/null || printf 'docker-node')"
labels=""
version="$DEFAULT_VERSION"
gpu_mode="auto"
allow_insecure="false"

usage() {
  cat <<'EOF'
Install the JobDock agent on a Linux amd64 Docker host.

Usage:
  install-agent.sh --server URL --token TOKEN [options]

Options:
  --gpu                 Request all NVIDIA GPUs and require NVML discovery.
  --name NAME           Node name reported to JobDock (defaults to hostname).
  --labels KEY=VALUE    Comma-separated node labels.
  --version VERSION     Immutable agent image version (defaults to latest).
  --allow-insecure-http Permit an http:// server URL for local development.
  --help                Show this help.
EOF
}

fail() {
  printf 'JobDock agent installation failed: %s\n' "$1" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) [ "$#" -ge 2 ] || fail "--server requires a value"; server="$2"; shift 2 ;;
    --token) [ "$#" -ge 2 ] || fail "--token requires a value"; token="$2"; shift 2 ;;
    --name) [ "$#" -ge 2 ] || fail "--name requires a value"; node_name="$2"; shift 2 ;;
    --labels) [ "$#" -ge 2 ] || fail "--labels requires a value"; labels="$2"; shift 2 ;;
    --version) [ "$#" -ge 2 ] || fail "--version requires a value"; version="$2"; shift 2 ;;
    --gpu) gpu_mode="required"; shift ;;
    --allow-insecure-http) allow_insecure="true"; shift ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
done

[ -n "$server" ] || fail "--server is required"
[ -n "$token" ] || fail "--token is required"
[ -n "$node_name" ] || fail "node name cannot be empty"
case "$version" in *[!0-9A-Za-z._-]*|'') fail "invalid image version" ;; esac
case "$server" in
  https://*) ;;
  http://*) [ "$allow_insecure" = "true" ] || fail "HTTP requires --allow-insecure-http" ;;
  *) fail "server URL must start with https:// or http://" ;;
esac
[ "$(uname -s)" = "Linux" ] || fail "only Linux is supported"
case "$(uname -m)" in x86_64|amd64) ;; *) fail "only amd64 hosts are supported" ;; esac
command -v docker >/dev/null 2>&1 || fail "Docker is not installed"
docker info >/dev/null 2>&1 || fail "Docker is unavailable; run this command with Docker access"
if docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
  fail "container $CONTAINER_NAME already exists; remove it explicitly before reinstalling"
fi

image="$IMAGE_REPOSITORY:$version"
printf 'Pulling %s...\n' "$image"
docker pull "$image"

set -- docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  --read-only \
  --tmpfs /tmp:size=64m,mode=1777 \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$STATE_DIR:/var/lib/jobdock-agent" \
  -e "JOBDOCK_SERVER_URL=$server" \
  -e "JOBDOCK_ENROLLMENT_TOKEN=$token" \
  -e "JOBDOCK_NODE_NAME=$node_name" \
  -e "JOBDOCK_ALLOW_INSECURE_HTTP=$allow_insecure" \
  -e "JOBDOCK_GPU_MODE=$gpu_mode"

if [ -n "$labels" ]; then
  set -- "$@" -e "JOBDOCK_NODE_LABELS=$labels"
fi
if [ "$gpu_mode" = "required" ]; then
  set -- "$@" \
    --gpus all \
    -e NVIDIA_VISIBLE_DEVICES=all \
    -e NVIDIA_DRIVER_CAPABILITIES=compute,utility
fi
set -- "$@" "$image"
"$@" >/dev/null

printf 'JobDock agent %s started as %s.\n' "$version" "$node_name"
printf 'Follow enrollment with: docker logs -f %s\n' "$CONTAINER_NAME"
