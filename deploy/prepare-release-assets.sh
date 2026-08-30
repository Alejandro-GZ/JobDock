#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  printf 'Usage: %s RELEASE_MANIFEST OUTPUT_DIRECTORY SDK_DISTRIBUTION_DIRECTORY\n' "$0" >&2
  exit 2
fi

manifest="$1"
output_dir="$2"
sdk_dir="$3"
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
  (.images | length == 3) and
  ([.images[].image] | unique | length == 3) and
  (all(.images[]; .digest | test("^sha256:[0-9a-f]{64}$"))) and
  (all(.images[]; .reference == (.image + "@" + .digest))) and
  (.sdk.name == "jobdock-sdk") and
  (.sdk.version | type == "string" and test("^[0-9]+\\.[0-9]+\\.[0-9]+([ab]|rc|\\.dev)?[0-9]*(\\+[0-9A-Za-z.-]+)?$")) and
  (all(.sdk.wheel, .sdk.sdist; (.filename | type == "string" and test("^[0-9A-Za-z][0-9A-Za-z._-]*$")) and (.sha256 | test("^[0-9a-f]{64}$"))))
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

[ ! -e "$output_dir" ] || { printf 'Output path already exists: %s\n' "$output_dir" >&2; exit 1; }
mkdir -p -- "$output_dir"
cp -- "$manifest" "$output_dir/release-manifest.json"
cp -- "$sdk_dir/$wheel_filename" "$output_dir/$wheel_filename"
cp -- "$sdk_dir/$sdist_filename" "$output_dir/$sdist_filename"
awk \
  -v version="$version" \
  '$0 == "DEFAULT_VERSION=\"\"" {print "DEFAULT_VERSION=\"" version "\""; next}
   {print}' \
  "$script_dir/install-control-plane.sh" > "$output_dir/install-control-plane.sh"
chmod 0755 "$output_dir/install-control-plane.sh"

awk \
  -v server="$server_reference" \
  -v builder="$builder_reference" \
  '{gsub(/@@JOBDOCK_SERVER_REFERENCE@@/, server); gsub(/@@JOBDOCK_BUILDER_REFERENCE@@/, builder); print}' \
  "$script_dir/docker-compose.release.yml.tmpl" > "$output_dir/docker-compose.yml"

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
- Publishes the matching Python SDK from the same verified release manifest.

## Components

| Component | Immutable reference |
| --- | --- |
| Server | \`$server_reference\` |
| Agent | \`$agent_reference\` |
| Builder | \`$builder_reference\` |

## Python SDK

\`pip install jobdock-sdk==$sdk_version\`

| Distribution | File | SHA-256 |
| --- | --- | --- |
| Wheel | \`$wheel_filename\` | \`$wheel_sha256\` |
| Source | \`$sdist_filename\` | \`$sdist_sha256\` |

Tag: \`$tag\`  
Commit: \`$commit\`

## Install

\`curl -fsSL https://github.com/$repository/releases/latest/download/install-control-plane.sh | sudo sh\`

## Changes
EOF

(
  cd -- "$output_dir"
  sha256sum release-manifest.json docker-compose.yml install-control-plane.sh install-agent.sh "$wheel_filename" "$sdist_filename" > SHA256SUMS
)
