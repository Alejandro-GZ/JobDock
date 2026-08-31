#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  printf 'Usage: %s RELEASE_MANIFEST OUTPUT_DIRECTORY SDK_DISTRIBUTION_DIRECTORY CLI_DISTRIBUTION_DIRECTORY\n' "$0" >&2
  exit 2
fi

manifest="$1"
output_dir="$2"
sdk_dir="$3"
cli_dir="$4"
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repository="${GITHUB_REPOSITORY:-Alejandro-GZ/JobDock}"

command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { printf 'sha256sum is required\n' >&2; exit 1; }
[ -f "$manifest" ] || { printf 'Release manifest not found: %s\n' "$manifest" >&2; exit 1; }

jq -e '
  .schema_version == 2 and
  (.version | type == "string" and length > 0) and
  (.tag == ("v" + .version)) and
  (.commit | test("^[0-9a-f]{40}$")) and
  (.database | (.schema | type == "number" and . >= 1) and (.rollback_floor | type == "number" and . >= 1) and (.rollback_floor <= .schema)) and
  (.images | length == 3) and
  ([.images[].image] | unique | length == 3) and
  (all(.images[]; .digest | test("^sha256:[0-9a-f]{64}$"))) and
  (all(.images[]; .reference == (.image + "@" + .digest))) and
  (.sdk.name == "jobdock-sdk") and
  (.sdk.version | type == "string" and test("^[0-9]+\\.[0-9]+\\.[0-9]+([ab]|rc|\\.dev)?[0-9]*(\\+[0-9A-Za-z.-]+)?$")) and
  (all(.sdk.wheel, .sdk.sdist; (.filename | type == "string" and test("^[0-9A-Za-z][0-9A-Za-z._-]*$")) and (.sha256 | test("^[0-9a-f]{64}$")))) and
  (.cli.name == "jobdock") and (.cli.version == .version) and (.cli.server_api == "v1") and
  (.cli.artifacts | length == 2) and
  ([.cli.artifacts[] | (.os + "/" + .arch)] | sort == ["linux/amd64", "linux/arm64"]) and
  (all(.cli.artifacts[]; (.filename | test("^[0-9A-Za-z][0-9A-Za-z._-]*$")) and (.sha256 | test("^[0-9a-f]{64}$")))) and
  (all(.images[]; (.platforms | type == "array" and length >= 1)))
' "$manifest" >/dev/null

version=$(jq -er '.version' "$manifest")
tag=$(jq -er '.tag' "$manifest")
commit=$(jq -er '.commit' "$manifest")
server_reference=$(jq -er '.images[] | select(.image | endswith("/jobdock-server")) | .reference' "$manifest")
agent_reference=$(jq -er '.images[] | select(.image | endswith("/jobdock-agent")) | .reference' "$manifest")
builder_reference=$(jq -er '.images[] | select(.image | endswith("/jobdock-builder")) | .reference' "$manifest")
sdk_version=$(jq -er '.sdk.version' "$manifest")
wheel_filename=$(jq -er '.sdk.wheel.filename' "$manifest")
wheel_sha256=$(jq -er '.sdk.wheel.sha256' "$manifest")
sdist_filename=$(jq -er '.sdk.sdist.filename' "$manifest")
sdist_sha256=$(jq -er '.sdk.sdist.sha256' "$manifest")

[ -f "$sdk_dir/$wheel_filename" ] || { printf 'SDK wheel not found: %s\n' "$wheel_filename" >&2; exit 1; }
[ -f "$sdk_dir/$sdist_filename" ] || { printf 'SDK sdist not found: %s\n' "$sdist_filename" >&2; exit 1; }
[ "$(sha256sum "$sdk_dir/$wheel_filename" | cut -d ' ' -f 1)" = "$wheel_sha256" ] || { printf 'SDK wheel checksum mismatch\n' >&2; exit 1; }
[ "$(sha256sum "$sdk_dir/$sdist_filename" | cut -d ' ' -f 1)" = "$sdist_sha256" ] || { printf 'SDK sdist checksum mismatch\n' >&2; exit 1; }
cli_filenames=""
while IFS="$(printf '\t')" read -r cli_filename cli_sha256; do
  [ -f "$cli_dir/$cli_filename" ] || { printf 'CLI archive not found: %s\n' "$cli_filename" >&2; exit 1; }
  [ "$(sha256sum "$cli_dir/$cli_filename" | cut -d ' ' -f 1)" = "$cli_sha256" ] || { printf 'CLI archive checksum mismatch\n' >&2; exit 1; }
  cli_filenames="$cli_filenames $cli_filename"
done <<EOF
$(jq -r '.cli.artifacts[] | [.filename, .sha256] | @tsv' "$manifest")
EOF

[ ! -e "$output_dir" ] || { printf 'Output path already exists: %s\n' "$output_dir" >&2; exit 1; }
mkdir -p -- "$output_dir"
cp -- "$manifest" "$output_dir/release-manifest.json"
cp -- "$sdk_dir/$wheel_filename" "$output_dir/$wheel_filename"
cp -- "$sdk_dir/$sdist_filename" "$output_dir/$sdist_filename"
for cli_filename in $cli_filenames; do cp -- "$cli_dir/$cli_filename" "$output_dir/$cli_filename"; done
cp -- "$script_dir/install-cli.sh" "$output_dir/install-cli.sh"
chmod 0755 "$output_dir/install-cli.sh"
awk \
  -v version="$version" \
  '$0 == "DEFAULT_VERSION=\"\"" {print "DEFAULT_VERSION=\"" version "\""; next}
   {print}' \
  "$script_dir/install-control-plane.sh" > "$output_dir/install-control-plane.sh"
chmod 0755 "$output_dir/install-control-plane.sh"
cp -- "$script_dir/jobdock-doctor.sh" "$output_dir/jobdock-doctor"
chmod 0755 "$output_dir/jobdock-doctor"
database_schema=$(jq -er '.database.schema' "$manifest")
awk -v schema="$database_schema" '$0 ~ /^SUPPORTED_DATABASE_SCHEMA=/ {print "SUPPORTED_DATABASE_SCHEMA=" schema; next} {print}' "$script_dir/jobdockctl.sh" > "$output_dir/jobdockctl"
chmod 0755 "$output_dir/jobdockctl"

awk \
  -v server="$server_reference" \
  -v builder="$builder_reference" \
  '{gsub(/@@JOBDOCK_SERVER_REFERENCE@@/, server); gsub(/@@JOBDOCK_BUILDER_REFERENCE@@/, builder); print}' \
  "$script_dir/docker-compose.release.yml.tmpl" > "$output_dir/docker-compose.yml"
for deployment_asset in docker-compose.domain.yml docker-compose.proxy.yml docker-compose.local.yml; do
  cp -- "$script_dir/$deployment_asset" "$output_dir/$deployment_asset"
done
cp -- "$script_dir/Caddyfile.release" "$output_dir/Caddyfile"

awk \
  -v version="$version" \
  -v reference="$agent_reference" \
  '$0 == "DEFAULT_VERSION=\"latest\"" {print "DEFAULT_VERSION=\"" version "\""; next}
   $0 == "DEFAULT_IMAGE_REFERENCE=\"\"" {print "DEFAULT_IMAGE_REFERENCE=\"" reference "\""; next}
   {print}' \
  "$script_dir/install-agent.sh" > "$output_dir/install-agent.sh"
chmod 0755 "$output_dir/install-agent.sh"

cat > "$output_dir/release-notes.md" <<EOF
## Highlights

- Publishes the version-matched JobDock server, agent, and builder as one verified release set.
- Includes digest-pinned deployment assets for reproducible installation and auditing.
- Adds the one-command, checksum-verifying control-plane bootstrap.
- Includes the read-only JobDock doctor with human and machine-readable diagnostics.
- Publishes the matching Python SDK from the same verified release manifest.
- Publishes version-matched JobDock CLI archives for Linux amd64 and arm64.

## Components

| Component | Immutable reference |
| --- | --- |
| Server | \`$server_reference\` |
| Agent | \`$agent_reference\` |
| Builder | \`$builder_reference\` |

## JobDock CLI

\`curl -fsSL https://github.com/$repository/releases/latest/download/install-cli.sh | sudo sh\`

The \`jobdock $version\` CLI targets server API \`v1\`. The installer selects the verified archive for the host architecture.

## Python SDK

\`pip install jobdock-sdk==$sdk_version\`

| Distribution | File | SHA-256 |
| --- | --- | --- |
| Wheel | \`$wheel_filename\` | \`$wheel_sha256\` |
| Source | \`$sdist_filename\` | \`$sdist_sha256\` |

Tag: \`$tag\`  
Commit: \`$commit\`

## Install

\`curl -fsSL https://github.com/$repository/releases/latest/download/install-control-plane.sh | sudo sh -s -- --mode domain --domain dock.example.com\`

## Changes
EOF

(
  cd -- "$output_dir"
  sha256sum release-manifest.json docker-compose.yml docker-compose.domain.yml docker-compose.proxy.yml docker-compose.local.yml Caddyfile install-control-plane.sh install-agent.sh install-cli.sh jobdock-doctor jobdockctl $cli_filenames "$wheel_filename" "$sdist_filename" > SHA256SUMS
)
