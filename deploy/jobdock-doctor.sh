#!/bin/sh
set -u

CONFIG_DIR="${JOBDOCK_INSTALL_CONFIG_DIR:-/etc/jobdock}"
DATA_DIR="${JOBDOCK_INSTALL_DATA_DIR:-/var/lib/jobdock}"
FORMAT=human
GPU=false
EXPOSURE=""
FAILURES=0
RESULTS=""

usage() { printf '%s\n' 'Usage: jobdock-doctor [--json] [--gpu] [--exposure MODE] [--config-dir PATH] [--data-dir PATH]'; }
while [ "$#" -gt 0 ]; do
  case "$1" in
    --json) FORMAT=json; shift ;;
    --gpu) GPU=true; shift ;;
    --exposure) [ "$#" -ge 2 ] || exit 2; EXPOSURE="$2"; shift 2 ;;
    --config-dir) [ "$#" -ge 2 ] || exit 2; CONFIG_DIR="$2"; shift 2 ;;
    --data-dir) [ "$#" -ge 2 ] || exit 2; DATA_DIR="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done
case "$EXPOSURE" in ""|domain|proxy|local) :;; *) printf '%s\n' '--exposure must be domain, proxy, or local' >&2; exit 2;; esac

escape_json() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
record() {
  check="$1" status="$2" explanation="$3" remediation="$4"
  [ "$status" != fail ] || FAILURES=$((FAILURES + 1))
  if [ "$FORMAT" = human ]; then
    case "$status" in pass) marker='PASS';; warn) marker='WARN';; *) marker='FAIL';; esac
    printf '[%s] %-22s %s\n' "$marker" "$check" "$explanation"
    [ -z "$remediation" ] || printf '       Remediation: %s\n' "$remediation"
  else
    item=$(printf '{"check":"%s","status":"%s","explanation":"%s","remediation":"%s"}' "$(escape_json "$check")" "$status" "$(escape_json "$explanation")" "$(escape_json "$remediation")")
    [ -z "$RESULTS" ] && RESULTS="$item" || RESULTS="$RESULTS,$item"
  fi
}

if [ "$(uname -s 2>/dev/null)" = Linux ]; then record os pass 'Linux host detected.' ''; else record os fail 'JobDock requires Linux.' 'Use a supported Linux host.'; fi
case "$(uname -m 2>/dev/null)" in x86_64|amd64) record architecture pass 'linux/amd64 is supported.' '';; *) record architecture fail "Unsupported architecture: $(uname -m 2>/dev/null)." 'Use linux/amd64 for this release.';; esac
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then record docker pass 'Docker Engine is reachable.' ''; else record docker fail 'Docker Engine is unavailable.' 'Install/start Docker and grant this user daemon access.'; fi
if docker compose version >/dev/null 2>&1; then record compose pass "$(docker compose version 2>/dev/null | head -n1)" ''; else record compose fail 'Docker Compose plugin is unavailable.' 'Install the Docker Compose plugin.'; fi

memory_kib=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo 2>/dev/null)
if [ "${memory_kib:-0}" -ge 2097152 ]; then record memory pass "$((memory_kib / 1024)) MiB host memory detected." ''; else record memory fail 'Less than 2 GiB host memory is available.' 'Use a host with at least 2 GiB RAM; 8 GiB is recommended with builds.'; fi
probe_path="$DATA_DIR"; [ -e "$probe_path" ] || probe_path=$(dirname "$probe_path")
free_kib=$(df -Pk "$probe_path" 2>/dev/null | awk 'NR==2 {print $4}')
if [ "${free_kib:-0}" -ge 10485760 ]; then record disk pass "$((free_kib / 1024)) MiB free near $DATA_DIR." ''; else record disk fail 'Less than 10 GiB free storage is available.' 'Free disk space or choose a larger data directory.'; fi
if [ -e "$CONFIG_DIR" ] && [ ! -r "$CONFIG_DIR" ]; then record permissions fail "$CONFIG_DIR is not readable." 'Run as an authorized administrator.'; else record permissions pass 'Installation paths are accessible.' ''; fi
if [ "$EXPOSURE" = domain ] && [ ! -f "$CONFIG_DIR/jobdock.env" ]; then
  if command -v ss >/dev/null 2>&1; then
    for required_port in 80 443; do
      if ss -ltn 2>/dev/null | grep -Eq "[:.]${required_port}[[:space:]]"; then record "port_$required_port" fail "TCP $required_port is already occupied." 'Stop the conflicting service or use proxy mode.'; else record "port_$required_port" pass "TCP $required_port is available." ''; fi
    done
  else
    record ports warn 'Port ownership could not be inspected because ss is unavailable.' 'Install iproute2 or verify TCP 80/443 manually.'
  fi
fi

release_url="${JOBDOCK_RELEASES_URL:-https://github.com/Alejandro-GZ/JobDock/releases}"
if command -v curl >/dev/null 2>&1 && curl -fsIL --max-time 10 "$release_url/latest" >/dev/null 2>&1; then record github pass 'GitHub Releases is reachable.' ''; else record github fail 'GitHub Releases is unreachable.' 'Check DNS, HTTPS egress, proxy, and firewall rules.'; fi
ghcr_code=$(curl -sSIL --max-time 10 -o /dev/null -w '%{http_code}' https://ghcr.io/v2/ 2>/dev/null)
case "$ghcr_code" in 200|401) record ghcr pass 'GHCR registry endpoint is reachable.' '';; *) record ghcr fail "GHCR returned ${ghcr_code:-no response}." 'Allow HTTPS egress to ghcr.io.';; esac

environment_file="$CONFIG_DIR/jobdock.env"
if [ -f "$environment_file" ]; then
  mode=$(awk -F= '$1=="JOBDOCK_EXPOSURE_MODE" {print $2; exit}' "$environment_file")
  public_url=$(awk -F= '$1=="JOBDOCK_PUBLIC_URL" {sub(/^[^=]*=/,""); print; exit}' "$environment_file")
  port=$(awk -F= '$1=="JOBDOCK_HTTP_PORT" {print $2; exit}' "$environment_file")
  case "$mode:$public_url" in domain:https://*|proxy:https://*|local:http://*) record public_url pass "$mode mode and $public_url are coherent." '';; *) record public_url fail 'Exposure mode and public URL are inconsistent.' 'Correct the supported exposure configuration before startup.';; esac
  if [ "$mode" = domain ]; then
    domain=${public_url#https://}; domain=${domain%%/*}
    if getent hosts "$domain" >/dev/null 2>&1; then record dns pass "$domain resolves from this host." ''; else record dns fail "$domain does not resolve." 'Point the domain A/AAAA record at this host.'; fi
    if curl -fsSIL --max-time 15 "$public_url/health/ready" >/dev/null 2>&1; then record tls pass 'Public HTTPS certificate and readiness are valid.' ''; else record tls fail 'Public HTTPS readiness failed.' 'Verify DNS, TCP 80/443, UDP 443, and Caddy logs.'; fi
  else
    if curl -fsS --max-time 5 "http://127.0.0.1:${port:-8080}/health/ready" >/dev/null 2>&1; then record server_health pass 'Server readiness endpoint is healthy.' ''; else record server_health warn 'Local readiness endpoint is not reachable.' 'Start the deployment or inspect jobdock-server logs.'; fi
  fi
  exposure="$CONFIG_DIR/docker-compose.exposure.yml"
  if [ -f "$CONFIG_DIR/docker-compose.yml" ] && [ -f "$exposure" ]; then
    compose() { docker compose --project-name jobdock --env-file "$environment_file" --env-file "$CONFIG_DIR/overrides.env" -f "$CONFIG_DIR/docker-compose.yml" -f "$exposure" "$@"; }
    if compose config --quiet >/dev/null 2>&1; then record configuration pass 'Effective Compose configuration is valid.' ''; else record configuration fail 'Effective Compose configuration is invalid.' 'Run docker compose config with the installed env and overlay files.'; fi
    running=$(compose ps --status running --services 2>/dev/null)
    for service in jobdock-server jobdock-builder buildkitd; do
      if printf '%s\n' "$running" | grep -qx "$service"; then record "service_$service" pass "$service is running." ''; else record "service_$service" warn "$service is not running." "Inspect $service logs; a deliberately disabled capability may be expected."; fi
    done
  fi
else
  record installation warn 'No installed JobDock configuration was found; runtime checks were skipped.' 'Run the release installer after resolving preflight failures.'
fi

if [ "$GPU" = true ]; then
  if command -v nvidia-smi >/dev/null 2>&1 && nvidia-smi -L >/dev/null 2>&1; then record nvidia_driver pass 'NVIDIA driver and nvidia-smi are operational.' ''; else record nvidia_driver fail 'nvidia-smi could not enumerate a GPU.' 'Install a supported NVIDIA driver.'; fi
  gpu_image="${JOBDOCK_DOCTOR_GPU_IMAGE:-nvidia/cuda:12.9.1-base-ubuntu24.04}"
  if docker run --rm --gpus all "$gpu_image" nvidia-smi -L >/dev/null 2>&1; then record nvidia_container pass 'A real GPU container completed successfully.' ''; else record nvidia_container fail 'Docker could not run a GPU container.' 'Install/configure NVIDIA Container Toolkit and restart Docker.'; fi
fi

if [ "$FORMAT" = json ]; then printf '{"schema_version":1,"ok":%s,"checks":[%s]}\n' "$( [ "$FAILURES" -eq 0 ] && printf true || printf false )" "$RESULTS"; fi
[ "$FAILURES" -eq 0 ]
