#!/bin/sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

cat >"$temporary/docker" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "container" ] && [ "${2:-}" = "inspect" ]; then
  [ -f "$DOCKER_STARTED" ] || exit 1
  case "$*" in *"{{.Config.Image}}"*) printf 'ghcr.io/alejandro-gz/jobdock-agent:latest\n' ;; *"{{range .Config.Env}}"*) printf 'JOBDOCK_SERVER_URL=https://dock.example.test\nJOBDOCK_NODE_NAME=cpu-01\nJOBDOCK_GPU_MODE=auto\n' ;; esac
  exit 0
fi
if [ "${1:-}" = "logs" ]; then printf '{"msg":"agent_authenticated","node_id":"preserved-node"}\n'; exit 0; fi
printf '%s\n' "$*" >>"$DOCKER_CALLS"
if [ "${1:-}" = "run" ]; then : >"$DOCKER_STARTED"; fi
exit 0
EOF
chmod +x "$temporary/docker"
cat >"$temporary/uname" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-m" ]; then printf 'x86_64\n'; else printf 'Linux\n'; fi
EOF
chmod +x "$temporary/uname"
cat >"$temporary/curl" <<'EOF'
#!/bin/sh
printf '{"status":"connected"}\n'
EOF
chmod +x "$temporary/curl"
export PATH="$temporary:$PATH"
export DOCKER_CALLS="$temporary/calls"
export DOCKER_STARTED="$temporary/started"
export JOBDOCK_AGENT_STATE_DIR="$temporary/state"

sh "$root/deploy/install-agent.sh" --server https://dock.example.test --token one-use-token --name cpu-01 --labels zone=lab >/dev/null
grep -F "pull ghcr.io/alejandro-gz/jobdock-agent:latest" "$DOCKER_CALLS" >/dev/null
grep -F "JOBDOCK_GPU_MODE=auto" "$DOCKER_CALLS" >/dev/null
if grep -F -- "--gpus all" "$DOCKER_CALLS" >/dev/null; then
  echo "CPU installation unexpectedly requested GPUs" >&2
  exit 1
fi

printf '{"node_id":"preserved-node","credential":"preserved"}\n' >"$JOBDOCK_AGENT_STATE_DIR/credential.json"
sh "$root/deploy/install-agent.sh" --repair >/dev/null
grep -F "rename jobdock-agent jobdock-agent-rollback" "$DOCKER_CALLS" >/dev/null
grep -F "ghcr.io/alejandro-gz/jobdock-agent:latest" "$JOBDOCK_AGENT_STATE_DIR/install.conf" >/dev/null

: >"$DOCKER_CALLS"
rm -f "$DOCKER_STARTED"
sh "$root/deploy/install-agent.sh" --server http://dock.local:8080 --allow-insecure-http --token one-use-token --gpu --version 0.1.0 >/dev/null
grep -F -- "--gpus all" "$DOCKER_CALLS" >/dev/null
grep -F "JOBDOCK_GPU_MODE=required" "$DOCKER_CALLS" >/dev/null
grep -F "NVIDIA_DRIVER_CAPABILITIES=compute,utility" "$DOCKER_CALLS" >/dev/null

mkdir -p "$JOBDOCK_AGENT_STATE_DIR/assignments"
printf '{"job_id":"active-job","completed":false}\n' >"$JOBDOCK_AGENT_STATE_DIR/assignments/active-job.json"
if sh "$root/deploy/install-agent.sh" --upgrade --version 0.2.0 >/dev/null 2>&1; then
  echo "Upgrade unexpectedly proceeded with an active assignment" >&2
  exit 1
fi
rm -f "$JOBDOCK_AGENT_STATE_DIR/assignments/active-job.json"
rm -f "$DOCKER_STARTED"

if sh "$root/deploy/install-agent.sh" --server http://dock.local:8080 --token one-use-token >/dev/null 2>&1; then
  echo "HTTP installation succeeded without explicit opt-in" >&2
  exit 1
fi
