"""Derive the Python SDK version from the JobDock product release tag."""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path

SEMVER_TAG = re.compile(
    r"^v(?P<major>0|[1-9][0-9]*)\.(?P<minor>0|[1-9][0-9]*)\.(?P<patch>0|[1-9][0-9]*)"
    r"(?:-(?P<prerelease>[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$"
)
KNOWN_PRERELEASES = {"a": "a", "alpha": "a", "b": "b", "beta": "b", "rc": "rc"}


class VersionError(ValueError):
    """Raised when release version inputs are invalid or inconsistent."""


def semver_tag_to_pep440(tag: str) -> str:
    match = SEMVER_TAG.fullmatch(tag.strip())
    if not match:
        raise VersionError("release tag must be SemVer, for example v0.3.0 or v0.3.0-rc.1")
    base = ".".join(match.group(name) for name in ("major", "minor", "patch"))
    prerelease = match.group("prerelease")
    if not prerelease:
        return base
    identifiers = prerelease.split(".")
    for identifier in identifiers:
        if identifier.isdigit() and len(identifier) > 1 and identifier.startswith("0"):
            raise VersionError("numeric prerelease identifiers must not contain leading zeroes")
    label = identifiers[0].lower()
    number = int(identifiers[1]) if len(identifiers) > 1 and identifiers[1].isdigit() else 0
    remainder = identifiers[2:] if len(identifiers) > 1 and identifiers[1].isdigit() else identifiers[1:]
    if label in KNOWN_PRERELEASES:
        version = f"{base}{KNOWN_PRERELEASES[label]}{number}"
        return version + _local_suffix(remainder)
    if label.isdigit():
        return f"{base}.dev{int(label)}" + _local_suffix(identifiers[1:])
    return f"{base}.dev{number}" + _local_suffix([label, *remainder])


def resolve_build_version(root: Path | None = None, environ: dict[str, str] | None = None) -> str:
    env = os.environ if environ is None else environ
    tag = env.get("JOBDOCK_RELEASE_TAG", "").strip()
    github_ref_type = env.get("GITHUB_REF_TYPE", "").strip()
    github_ref_name = env.get("GITHUB_REF_NAME", "").strip()
    if github_ref_type == "tag":
        if tag and tag != github_ref_name:
            raise VersionError(f"JOBDOCK_RELEASE_TAG {tag!r} does not match GitHub tag {github_ref_name!r}")
        tag = github_ref_name
    elif github_ref_type and tag:
        raise VersionError("a release version cannot be built from a non-tag GitHub ref")
    if not tag:
        tag = _exact_git_tag(root or Path(__file__).resolve().parents[2])
    if tag:
        python_version = semver_tag_to_pep440(tag)
        product_version = env.get("JOBDOCK_PRODUCT_VERSION", "").strip()
        if product_version and product_version != tag.removeprefix("v"):
            raise VersionError(f"product version {product_version!r} does not match release tag {tag!r}")
        expected = env.get("JOBDOCK_SDK_VERSION", "").strip()
        if expected and expected != python_version:
            raise VersionError(f"SDK version {expected!r} does not match {tag!r} ({python_version})")
        return python_version
    return _development_version(root or Path(__file__).resolve().parents[2])


def _exact_git_tag(root: Path) -> str:
    result = subprocess.run(
        ["git", "describe", "--tags", "--exact-match", "--match", "v[0-9]*"],
        cwd=root,
        capture_output=True,
        text=True,
        check=False,
    )
    return result.stdout.strip() if result.returncode == 0 else ""


def _development_version(root: Path) -> str:
    result = subprocess.run(
        ["git", "rev-parse", "--short=12", "HEAD"], cwd=root, capture_output=True, text=True, check=False
    )
    revision = re.sub(r"[^0-9a-f]", "", result.stdout.lower())
    return f"0.0.0.dev0+g{revision}" if revision else "0.0.0.dev0"


def _local_suffix(identifiers: list[str]) -> str:
    normalized = [re.sub(r"[^0-9a-z]+", ".", item.lower()).strip(".") for item in identifiers]
    normalized = [item for item in normalized if item]
    return "+" + ".".join(normalized) if normalized else ""


if __name__ == "__main__":
    print(resolve_build_version())
