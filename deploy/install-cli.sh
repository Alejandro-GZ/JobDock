#!/bin/sh
set -eu

REPOSITORY="${JOBDOCK_RELEASE_REPOSITORY:-Alejandro-GZ/JobDock}"
RELEASES_URL="${JOBDOCK_RELEASES_URL:-https://github.com/$REPOSITORY/releases}"
VERSION=""
BIN_DIR="${JOBDOCK_CLI_BIN_DIR:-/usr/local/bin}"

usage() {
  cat <<'EOF'
Install a verified precompiled JobDock CLI.

Usage: install-cli.sh [--version VERSION] [--bin-dir PATH]

The current stable release is used when --version is omitted. Linux amd64 and arm64 are
supported by this release. The archive is verified against the release
SHA256SUMS before it is extracted or installed.
EOF
}
fail() { printf 'JobDock CLI installation failed: %s\n' "$*" >&2; exit 1; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || fail "--version requires a value"; VERSION=${2#v}; shift 2;;
    --bin-dir) [ "$#" -ge 2 ] || fail "--bin-dir requires a value"; BIN_DIR=$2; shift 2;;
    --help|-h) usage; exit 0;;
    *) fail "unknown option: $1";;
  esac
done

[ "$(uname -s 2>/dev/null || true)" = Linux ] || fail "Linux is required"
case "$(uname -m 2>/dev/null || true)" in x86_64|amd64) platform=linux_amd64;; aarch64|arm64) platform=linux_arm64;; *) fail "this release supports Linux amd64 and arm64 only";; esac
for command_name in curl sha256sum mktemp tar install awk grep find wc tr; do command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"; done

if [ -z "$VERSION" ]; then
  latest_url=$(curl --fail --location --silent --show-error --output /dev/null --write-out '%{url_effective}' "$RELEASES_URL/latest") || fail "could not resolve the current stable release"
  tag=${latest_url##*/}
  VERSION=${tag#v}
else
  tag="v$VERSION"
fi
printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$' || fail "version must be semantic"

archive="jobdock-cli_${VERSION}_${platform}.tar.gz"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
release_url="$RELEASES_URL/download/$tag"
curl --fail --location --silent --show-error --output "$temporary/SHA256SUMS" "$release_url/SHA256SUMS" || fail "could not download SHA256SUMS"
curl --fail --location --silent --show-error --output "$temporary/$archive" "$release_url/$archive" || fail "could not download $archive"
awk -v expected="$archive" '$2 == expected || $2 == "*" expected {print}' "$temporary/SHA256SUMS" > "$temporary/archive.sha256"
[ -s "$temporary/archive.sha256" ] || fail "SHA256SUMS does not cover $archive"
(cd "$temporary" && sha256sum --check archive.sha256 >/dev/null) || fail "archive checksum verification failed"

archive_listing=$(tar -tzf "$temporary/$archive") || fail "archive could not be inspected"
[ "$archive_listing" = jobdock ] || fail "archive contains unexpected paths"
mkdir -p "$temporary/extracted"
tar -xzf "$temporary/$archive" -C "$temporary/extracted"
[ -f "$temporary/extracted/jobdock" ] && [ ! -L "$temporary/extracted/jobdock" ] || fail "archive does not contain a regular jobdock executable"
[ "$(find "$temporary/extracted" -mindepth 1 -maxdepth 1 | wc -l | tr -d ' ')" = 1 ] || fail "archive contains unexpected files"
install -d -m 0755 "$BIN_DIR"
install -m 0755 "$temporary/extracted/jobdock" "$BIN_DIR/jobdock"
printf 'Installed JobDock CLI %s at %s/jobdock\n' "$VERSION" "$BIN_DIR"
