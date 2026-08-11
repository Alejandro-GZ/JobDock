#!/bin/sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

cat >"$temporary/docker" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "container" ] && [ "${2:-}" = "inspect" ]; then exit 1; fi
printf '%s\n' "$*" >>"$DOCKER_CALLS"
exit 0
EOF
chmod +x "$temporary/docker"
cat >"$temporary/uname" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-m" ]; then printf 'x86_64\n'; else printf 'Linux\n'; fi
EOF
chmod +x "$temporary/uname"
export PATH="$temporary:$PATH"
export DOCKER_CALLS="$temporary/calls"

sh "$root/deploy/install-agent.sh" --server https://dock.example.test --token one-use-token --name cpu-01 --labels zone=lab >/dev/null
grep -F "pull ghcr.io/alejandro-gz/jobdock-agent:latest" "$DOCKER_CALLS" >/dev/null
grep -F "JOBDOCK_GPU_MODE=auto" "$DOCKER_CALLS" >/dev/null
if grep -F -- "--gpus all" "$DOCKER_CALLS" >/dev/null; then
  echo "CPU installation unexpectedly requested GPUs" >&2
  exit 1
fi

: >"$DOCKER_CALLS"
sh "$root/deploy/install-agent.sh" --server http://dock.local:8080 --allow-insecure-http --token one-use-token --gpu --version 0.1.0 >/dev/null
grep -F -- "--gpus all" "$DOCKER_CALLS" >/dev/null
grep -F "JOBDOCK_GPU_MODE=required" "$DOCKER_CALLS" >/dev/null
grep -F "NVIDIA_DRIVER_CAPABILITIES=compute,utility" "$DOCKER_CALLS" >/dev/null

if sh "$root/deploy/install-agent.sh" --server http://dock.local:8080 --token one-use-token >/dev/null 2>&1; then
  echo "HTTP installation succeeded without explicit opt-in" >&2
  exit 1
fi
