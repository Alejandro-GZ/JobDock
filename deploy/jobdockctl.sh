#!/bin/sh
set -eu

CONFIG_DIR="${JOBDOCK_INSTALL_CONFIG_DIR:-/etc/jobdock}"
DATA_DIR="${JOBDOCK_INSTALL_DATA_DIR:-/var/lib/jobdock}"
SUPPORTED_DATABASE_SCHEMA=32
HEALTH_TIMEOUT="${JOBDOCK_INSTALL_HEALTH_TIMEOUT:-180}"

fail() { printf 'jobdockctl: %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'EOF'
Usage:
  jobdockctl backup [--output FILE]
  jobdockctl restore ARCHIVE [--force]

Backups contain JobDock state, artifacts, configuration, and secrets. Store
them as sensitive credentials. Restore validates the archive before writing.
EOF
}
require_root() { [ "${JOBDOCK_INSTALL_ALLOW_NON_ROOT:-false}" = "true" ] || [ "$(id -u)" -eq 0 ] || fail "run as root"; }
require_tools() { for tool in tar sha256sum mktemp awk grep du df date mkdir chmod mv cp docker dirname find sed sleep rm basename; do command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"; done; }
compose() { docker compose --project-name jobdock --env-file "$CONFIG_DIR/jobdock.env" --env-file "$CONFIG_DIR/overrides.env" -f "$CONFIG_DIR/docker-compose.yml" -f "$CONFIG_DIR/docker-compose.exposure.yml" "$@"; }
value() { awk -F= -v key="$2" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$1"; }
directory_empty() { [ ! -d "$1" ] || [ -z "$(find "$1" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]; }

backup() {
  output=""
  while [ "$#" -gt 0 ]; do case "$1" in --output) [ "$#" -ge 2 ] || fail "--output requires a file"; output="$2"; shift 2;; --help|-h) usage; exit 0;; *) fail "unknown backup option: $1";; esac; done
  [ -f "$CONFIG_DIR/install-state" ] || fail "JobDock is not installed in $CONFIG_DIR"
  [ -d "$DATA_DIR" ] || fail "JobDock data directory is missing: $DATA_DIR"
  version=$(value "$CONFIG_DIR/install-state" version); schema=$(value "$CONFIG_DIR/install-state" database_schema)
  [ -n "$version" ] || fail "installed version is unknown"
  [ -n "$schema" ] || schema=0
  timestamp=$(date -u +%Y%m%dT%H%M%SZ)
  [ -n "$output" ] || output="$PWD/jobdock-backup-$version-$timestamp.tar"
  case "$output" in /*) ;; *) output="$PWD/$output";; esac
  destination=$(dirname "$output"); mkdir -p "$destination"
  required_kb=$(du -sk "$CONFIG_DIR" "$DATA_DIR" | awk '{sum += $1} END {print sum * 2 + 10240}')
  available_kb=$(df -Pk "$destination" | awk 'NR == 2 {print $4}')
  [ "$available_kb" -ge "$required_kb" ] || fail "insufficient free space: need approximately ${required_kb} KiB, have ${available_kb} KiB"
  work=$(mktemp -d); payload="$work/payload"; mkdir -p "$payload/config" "$payload/data"
  services_stopped=false
  cleanup() { if [ "$services_stopped" = "true" ]; then compose up -d >/dev/null 2>&1 || true; fi; rm -rf "$work"; }
  trap cleanup EXIT HUP INT TERM
  printf 'Stopping JobDock writers for a consistent snapshot...\n'
  compose stop jobdock-server jobdock-builder buildkitd >/dev/null 2>&1 || compose stop jobdock-server >/dev/null
  services_stopped=true
  cp -a "$CONFIG_DIR/." "$payload/config/"
  cp -a "$DATA_DIR/." "$payload/data/"
  tar -C "$payload" -cf "$work/payload.tar" config data
  generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  cat > "$work/manifest.json" <<EOF
{"schema_version":1,"jobdock_version":"$version","database_schema":$schema,"generated_at":"$generated_at","includes_secrets":true,"includes_artifacts":true}
EOF
  (cd "$work" && sha256sum manifest.json payload.tar > SHA256SUMS)
  temporary_output="$output.tmp.$$"
  tar -C "$work" -cf "$temporary_output" manifest.json SHA256SUMS payload.tar
  chmod 0600 "$temporary_output"
  (cd "$work" && sha256sum --check SHA256SUMS >/dev/null) || fail "snapshot checksum verification failed"
  mv "$temporary_output" "$output"
  compose up -d >/dev/null; services_stopped=false
  elapsed=0
  while ! compose exec -T jobdock-server /usr/local/bin/jobdock-server --healthcheck >/dev/null 2>&1; do [ "$elapsed" -lt "$HEALTH_TIMEOUT" ] || fail "backup is valid but JobDock did not become ready after restart"; sleep 1; elapsed=$((elapsed + 1)); done
  printf 'Backup created: %s\n' "$output"
  printf 'This file contains the master key and workload secrets; keep its 0600 permissions and store it securely.\n'
}

restore() {
  archive=""; force=false
  while [ "$#" -gt 0 ]; do case "$1" in --force) force=true; shift;; --help|-h) usage; exit 0;; -*) fail "unknown restore option: $1";; *) [ -z "$archive" ] || fail "restore accepts one archive"; archive="$1"; shift;; esac; done
  [ -f "$archive" ] || fail "backup archive not found: $archive"
  work=$(mktemp -d); trap 'rm -rf "$work"' EXIT HUP INT TERM
  entries=$(tar -tf "$archive") || fail "backup archive cannot be read"
  [ "$entries" = "manifest.json
SHA256SUMS
payload.tar" ] || fail "backup archive contains an unexpected or unsafe layout"
  tar -C "$work" -xf "$archive"
  (cd "$work" && sha256sum --check SHA256SUMS >/dev/null) || fail "backup checksum validation failed"
  grep -Eq '^\{"schema_version":1,' "$work/manifest.json" || fail "unsupported backup manifest"
  schema=$(sed -n 's/.*"database_schema":\([0-9][0-9]*\).*/\1/p' "$work/manifest.json")
  version=$(sed -n 's/.*"jobdock_version":"\([^"]*\)".*/\1/p' "$work/manifest.json")
  [ -n "$schema" ] && [ -n "$version" ] || fail "backup manifest is incomplete"
  [ "$schema" -le "$SUPPORTED_DATABASE_SCHEMA" ] || fail "backup schema $schema is newer than this jobdockctl supports ($SUPPORTED_DATABASE_SCHEMA); refusing an incompatible downgrade"
  tar -tf "$work/payload.tar" | awk 'index($0,"/")==1 || $0 ~ /(^|\/)\.\.($|\/)/ || ($0 !~ /^config(\/|$)/ && $0 !~ /^data(\/|$)/) {bad=1} END {exit bad}' || fail "backup payload contains an unsafe path"
  tar -tvf "$work/payload.tar" | awk 'substr($1,1,1) != "-" && substr($1,1,1) != "d" {bad=1} END {exit bad}' || fail "backup payload contains links or special files"
  mkdir -p "$work/restored"; tar -C "$work/restored" -xf "$work/payload.tar"
  [ -d "$work/restored/config" ] && [ -d "$work/restored/data" ] || fail "backup payload is incomplete"
  [ -f "$work/restored/config/secrets/master-key" ] && [ -f "$work/restored/data/server/jobdock.db" ] || fail "backup lacks the master key or SQLite database"
  if ! directory_empty "$CONFIG_DIR" || ! directory_empty "$DATA_DIR"; then [ "$force" = "true" ] || fail "restore target is not empty; inspect it and rerun with --force to preserve it as a recovery copy"; fi
  stamp=$(date -u +%Y%m%dT%H%M%SZ); recovery_root="${DATA_DIR}.pre-restore-$stamp"
  current_release="$work/current-release"; mkdir -p "$current_release"
  if [ -f "$CONFIG_DIR/install-state" ]; then for file in docker-compose.yml docker-compose.exposure.yml Caddyfile release-manifest.json install-state; do [ ! -f "$CONFIG_DIR/$file" ] || cp -p "$CONFIG_DIR/$file" "$current_release/$file"; done; fi
  if [ -f "$CONFIG_DIR/docker-compose.yml" ]; then compose down >/dev/null 2>&1 || true; fi
  if [ -d "$CONFIG_DIR" ] && ! directory_empty "$CONFIG_DIR"; then mv "$CONFIG_DIR" "$recovery_root-config"; fi
  if [ -d "$DATA_DIR" ] && ! directory_empty "$DATA_DIR"; then mv "$DATA_DIR" "$recovery_root-data"; fi
  mkdir -p "$(dirname "$CONFIG_DIR")" "$(dirname "$DATA_DIR")"
  mv "$work/restored/config" "$CONFIG_DIR"; mv "$work/restored/data" "$DATA_DIR"
  if [ -f "$current_release/install-state" ]; then for file in "$current_release"/*; do cp -p "$file" "$CONFIG_DIR/$(basename "$file")"; done; fi
  chmod 0700 "$CONFIG_DIR/secrets"; chmod 0400 "$CONFIG_DIR"/secrets/*; chmod 0600 "$CONFIG_DIR/secrets/master-key"
  compose up -d || fail "restore was written but services failed to start; recovery copies are under $recovery_root-*"
  elapsed=0
  while ! compose exec -T jobdock-server /usr/local/bin/jobdock-server --healthcheck >/dev/null 2>&1; do [ "$elapsed" -lt "$HEALTH_TIMEOUT" ] || fail "restored services did not become ready; inspect Compose logs and recovery copies under $recovery_root-*"; sleep 1; elapsed=$((elapsed + 1)); done
  printf 'Restore accepted from JobDock %s (database schema %s). Current services will apply supported migrations during startup.\n' "$version" "$schema"
  printf 'The original backup was not modified. Previous destination data, when present, is under %s-*\n' "$recovery_root"
}

require_root; require_tools
case "${1:-}" in backup) shift; backup "$@";; restore) shift; restore "$@";; --help|-h|help|'') usage;; *) fail "unknown command: $1";; esac
