#!/bin/sh
set -eu

DEFAULT_VERSION="latest"
DEFAULT_IMAGE_REFERENCE=""
IMAGE_REPOSITORY="ghcr.io/alejandro-gz/jobdock-agent"
CONTAINER_NAME="jobdock-agent"
STATE_DIR="${JOBDOCK_AGENT_STATE_DIR:-/var/lib/jobdock-agent}"

server=""
token=""
node_name="$(hostname 2>/dev/null || printf 'docker-node')"
labels=""
version="$DEFAULT_VERSION"
gpu_mode="auto"
allow_insecure="false"
operation="auto"
force_active="false"
health_timeout=60
server_set=false
name_set=false
labels_set=false
gpu_set=false

usage() {
  cat <<EOF
Install the JobDock CPU agent on a Linux amd64 or arm64 Docker host.

Usage:
  install-agent.sh --server URL --token TOKEN [options]

Options:
  --gpu                 Request all NVIDIA GPUs and require NVML discovery.
  --no-gpu              Disable NVIDIA discovery explicitly.
  --name NAME           Node name reported to JobDock (defaults to hostname).
  --labels KEY=VALUE    Comma-separated node labels.
  --version VERSION     Immutable agent image version (defaults to $DEFAULT_VERSION).
  --repair              Recreate the current agent using its preserved identity.
  --upgrade             Upgrade to this installer's immutable agent image.
  --force-active        Proceed despite locally active assignments (unsafe).
  --health-timeout SEC  Seconds to wait for authenticated startup (default: 60).
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
    --server) [ "$#" -ge 2 ] || fail "--server requires a value"; server="$2"; server_set=true; shift 2 ;;
    --token) [ "$#" -ge 2 ] || fail "--token requires a value"; token="$2"; shift 2 ;;
    --name) [ "$#" -ge 2 ] || fail "--name requires a value"; node_name="$2"; name_set=true; shift 2 ;;
    --labels) [ "$#" -ge 2 ] || fail "--labels requires a value"; labels="$2"; labels_set=true; shift 2 ;;
    --version) [ "$#" -ge 2 ] || fail "--version requires a value"; version="$2"; shift 2 ;;
    --gpu) gpu_mode="required"; gpu_set=true; shift ;;
    --no-gpu) gpu_mode="disabled"; gpu_set=true; shift ;;
    --repair) [ "$operation" = "auto" ] || fail "choose only one operation"; operation="repair"; shift ;;
    --upgrade) [ "$operation" = "auto" ] || fail "choose only one operation"; operation="upgrade"; shift ;;
    --force-active) force_active="true"; shift ;;
    --health-timeout) [ "$#" -ge 2 ] || fail "--health-timeout requires a value"; health_timeout="$2"; shift 2 ;;
    --allow-insecure-http) allow_insecure="true"; shift ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
done

case "$health_timeout" in *[!0-9]*|'') fail "health timeout must be a positive integer" ;; esac
[ "$health_timeout" -gt 0 ] || fail "health timeout must be positive"

[ "$(uname -s)" = "Linux" ] || fail "only Linux is supported"
case "$(uname -m)" in
  x86_64|amd64) host_arch=amd64 ;;
  aarch64|arm64) host_arch=arm64 ;;
  *) fail "only amd64 and arm64 hosts are supported" ;;
esac
command -v docker >/dev/null 2>&1 || fail "Docker is not installed"
command -v curl >/dev/null 2>&1 || fail "curl is required to verify enrollment"
docker info >/dev/null 2>&1 || fail "Docker is unavailable; run this command with Docker access"

existing=false
if docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then existing=true; fi
config_file="$STATE_DIR/install.conf"
read_config() { [ -f "$config_file" ] && awk -F= -v key="$1" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$config_file" || true; }
if [ "$existing" = "true" ]; then
  existing_environment=$(docker container inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$CONTAINER_NAME")
  read_existing_env() { printf '%s\n' "$existing_environment" | awk -F= -v key="$1" '$1 == key {sub(/^[^=]*=/, ""); print; exit}'; }
  [ "$server_set" = "true" ] || server=$(read_config server)
  [ -n "$server" ] || server=$(read_existing_env JOBDOCK_SERVER_URL)
  [ "$name_set" = "true" ] || { saved=$(read_config node_name); [ -z "$saved" ] || node_name="$saved"; }
  [ "$name_set" = "true" ] || { saved=$(read_existing_env JOBDOCK_NODE_NAME); [ -z "$saved" ] || node_name="$saved"; }
  [ "$labels_set" = "true" ] || labels=$(read_config labels)
  [ "$labels_set" = "true" ] || { [ -n "$labels" ] || labels=$(read_existing_env JOBDOCK_NODE_LABELS); }
  [ "$gpu_set" = "true" ] || { saved=$(read_config gpu_mode); [ -z "$saved" ] || gpu_mode="$saved"; }
  [ "$gpu_set" = "true" ] || { saved=$(read_existing_env JOBDOCK_GPU_MODE); [ -z "$saved" ] || gpu_mode="$saved"; }
  saved=$(read_config allow_insecure); [ -z "$saved" ] || allow_insecure="$saved"
  saved=$(read_existing_env JOBDOCK_ALLOW_INSECURE_HTTP); [ -z "$saved" ] || allow_insecure="$saved"
  [ -f "$STATE_DIR/credential.json" ] || fail "existing agent identity is missing; repair cannot safely create a different node. Restore credential.json or explicitly remove the installation and enroll again"
  token=""
fi

if [ "$host_arch" = arm64 ]; then
  [ "$gpu_mode" != required ] || fail "NVIDIA GPU mode is not officially supported on linux/arm64; use --no-gpu"
  [ "$gpu_mode" != auto ] || gpu_mode=disabled
fi

[ -n "$server" ] || fail "--server is required for a new or legacy installation"
[ "$existing" = "true" ] || [ -n "$token" ] || fail "--token is required for initial enrollment"
[ -n "$node_name" ] || fail "node name cannot be empty"
case "$version" in *[!0-9A-Za-z._-]*|'') fail "invalid image version" ;; esac
case "$server" in
  https://*) ;;
  http://*) [ "$allow_insecure" = "true" ] || fail "HTTP requires --allow-insecure-http" ;;
  *) fail "server URL must start with https:// or http://" ;;
esac
mkdir -p "$STATE_DIR"
chmod 0700 "$STATE_DIR"

if [ -n "$DEFAULT_IMAGE_REFERENCE" ] && [ "$version" = "$DEFAULT_VERSION" ]; then
  image="$DEFAULT_IMAGE_REFERENCE"
else
  image="$IMAGE_REPOSITORY:$version"
fi
printf 'Pulling %s...\n' "$image"
docker pull "$image"

rollback_name="$CONTAINER_NAME-rollback"
if [ "$existing" = "true" ]; then
  current_image=$(docker container inspect --format '{{.Config.Image}}' "$CONTAINER_NAME")
  if [ "$operation" = "auto" ]; then
    if [ "$current_image" = "$image" ]; then operation="repair"; else operation="upgrade"; fi
  fi
  if [ "$operation" = "upgrade" ] && [ "$force_active" != "true" ] && [ -d "$STATE_DIR/assignments" ] && grep -l '"completed"[[:space:]]*:[[:space:]]*false' "$STATE_DIR"/assignments/*.json >/dev/null 2>&1; then
    fail "active assignments were found; drain the node and wait for jobs to finish, or use --force-active only after assessing reconciliation risk"
  fi
  [ "$operation" != "repair" ] || image="$current_image"
  docker rm -f "$rollback_name" >/dev/null 2>&1 || true
  docker stop "$CONTAINER_NAME" >/dev/null
  docker rename "$CONTAINER_NAME" "$rollback_name"
  printf '%s will preserve node identity in %s.\n' "$(printf '%s' "$operation" | tr '[:lower:]' '[:upper:]')" "$STATE_DIR"
else
  operation="install"
fi

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
  -e "JOBDOCK_NODE_NAME=$node_name" \
  -e "JOBDOCK_ALLOW_INSECURE_HTTP=$allow_insecure" \
  -e "JOBDOCK_GPU_MODE=$gpu_mode"

if [ -n "$token" ]; then
  set -- "$@" -e "JOBDOCK_ENROLLMENT_TOKEN=$token"
fi

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
if ! "$@" >/dev/null; then
  if [ "$existing" = "true" ]; then docker rename "$rollback_name" "$CONTAINER_NAME"; docker start "$CONTAINER_NAME" >/dev/null; fi
  fail "new agent container could not be created; the previous container was restored when available"
fi

printf 'Waiting for JobDock enrollment and first heartbeat...\n'
elapsed=0
while [ "$elapsed" -lt "$health_timeout" ]; do
  if [ -n "$token" ]; then
    status=$(curl --fail --silent --show-error -H 'Content-Type: application/json' --data "{\"token\":\"$token\"}" "$server/api/v1/nodes/enrollment-status" 2>/dev/null || true)
    case "$status" in *'"status":"connected"'*) healthy=true ;; *) healthy=false ;; esac
  else
    if docker logs "$CONTAINER_NAME" 2>&1 | grep -q '"msg":"agent_authenticated"'; then healthy=true; else healthy=false; fi
  fi
  if [ "$healthy" = "true" ]; then
    temporary_config="$config_file.tmp.$$"
    { printf 'server=%s\n' "$server"; printf 'node_name=%s\n' "$node_name"; printf 'labels=%s\n' "$labels"; printf 'gpu_mode=%s\n' "$gpu_mode"; printf 'allow_insecure=%s\n' "$allow_insecure"; printf 'image=%s\n' "$image"; } > "$temporary_config"
    chmod 0600 "$temporary_config"; mv "$temporary_config" "$config_file"
    if [ "$existing" = "true" ]; then docker rm -f "$rollback_name" >/dev/null; fi
    printf 'JobDock agent %s completed: %s is authenticated and healthy.\n' "$operation" "$node_name"
    exit 0
  fi
  if ! docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
    if [ "$existing" = "true" ]; then docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true; docker rename "$rollback_name" "$CONTAINER_NAME"; docker start "$CONTAINER_NAME" >/dev/null; fi
    fail "replacement agent stopped before authentication; the previous container was restored when available. Inspect agent logs before retrying"
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done
docker logs --tail 40 "$CONTAINER_NAME" >&2 || true
if [ "$existing" = "true" ]; then
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker rename "$rollback_name" "$CONTAINER_NAME"
  docker start "$CONTAINER_NAME" >/dev/null
  fail "the replacement did not authenticate within ${health_timeout}s; the previous agent was restored. Inspect logs and retry repair or upgrade"
fi
fail "agent started but did not register within ${health_timeout}s; the failed container and logs were preserved. Verify the server URL, token, TLS trust, and outbound connectivity"
