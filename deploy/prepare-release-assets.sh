#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  printf 'Usage: %s RELEASE_MANIFEST OUTPUT_DIRECTORY\n' "$0" >&2
  exit 2
fi

manifest="$1"
output_dir="$2"
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { printf 'sha256sum is required\n' >&2; exit 1; }
[ -f "$manifest" ] || { printf 'Release manifest not found: %s\n' "$manifest" >&2; exit 1; }

jq -e '
  .schema_version == 1 and
  (.version | type == "string" and length > 0) and
  (.tag == ("v" + .version)) and
  (.commit | test("^[0-9a-f]{40}$")) and
  (.images | length == 3) and
  ([.images[].image] | unique | length == 3) and
  (all(.images[]; .digest | test("^sha256:[0-9a-f]{64}$"))) and
  (all(.images[]; .reference == (.image + "@" + .digest)))
' "$manifest" >/dev/null

version=$(jq -er '.version' "$manifest")
tag=$(jq -er '.tag' "$manifest")
commit=$(jq -er '.commit' "$manifest")
server_reference=$(jq -er '.images[] | select(.image | endswith("/jobdock-server")) | .reference' "$manifest")
agent_reference=$(jq -er '.images[] | select(.image | endswith("/jobdock-agent")) | .reference' "$manifest")
builder_reference=$(jq -er '.images[] | select(.image | endswith("/jobdock-builder")) | .reference' "$manifest")

[ ! -e "$output_dir" ] || { printf 'Output path already exists: %s\n' "$output_dir" >&2; exit 1; }
mkdir -p -- "$output_dir"
cp -- "$manifest" "$output_dir/release-manifest.json"

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

## Components

| Component | Immutable reference |
| --- | --- |
| Server | \`$server_reference\` |
| Agent | \`$agent_reference\` |
| Builder | \`$builder_reference\` |

Tag: \`$tag\`  
Commit: \`$commit\`

## Changes
EOF

(
  cd -- "$output_dir"
  sha256sum release-manifest.json docker-compose.yml install-agent.sh > SHA256SUMS
)
