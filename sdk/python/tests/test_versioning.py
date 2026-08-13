from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

import pytest

from versioning import VersionError, resolve_build_version, semver_tag_to_pep440

SDK_ROOT = Path(__file__).resolve().parents[1]


@pytest.mark.parametrize(
    ("tag", "expected"),
    [
        ("v0.3.0", "0.3.0"),
        ("v0.3.0-rc.1", "0.3.0rc1"),
        ("v1.2.3-beta.2", "1.2.3b2"),
        ("v1.2.3-preview.4", "1.2.3.dev4+preview"),
        ("v1.2.3-7", "1.2.3.dev7"),
    ],
)
def test_semver_tag_translates_deterministically_to_pep440(tag: str, expected: str) -> None:
    assert semver_tag_to_pep440(tag) == expected


def test_release_inputs_must_describe_the_same_product_version(tmp_path: Path) -> None:
    with pytest.raises(VersionError, match="does not match release tag"):
        resolve_build_version(tmp_path, {"JOBDOCK_RELEASE_TAG": "v0.3.0", "JOBDOCK_PRODUCT_VERSION": "0.4.0"})
    with pytest.raises(VersionError, match="does not match"):
        resolve_build_version(tmp_path, {"GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v0.3.0", "JOBDOCK_RELEASE_TAG": "v0.4.0"})
    with pytest.raises(VersionError, match="SDK version"):
        resolve_build_version(tmp_path, {"JOBDOCK_RELEASE_TAG": "v0.3.0-rc.1", "JOBDOCK_SDK_VERSION": "0.3.0"})


def test_non_tag_build_has_explicit_development_version(tmp_path: Path) -> None:
    assert resolve_build_version(tmp_path, {}).startswith("0.0.0.dev0")


def test_built_wheel_metadata_and_public_version_match_release_tag(tmp_path: Path) -> None:
    wheel_dir = tmp_path / "wheel"
    env = {**os.environ, "JOBDOCK_RELEASE_TAG": "v0.3.0-rc.1", "JOBDOCK_PRODUCT_VERSION": "0.3.0-rc.1"}
    subprocess.run(
        [sys.executable, "-m", "pip", "wheel", ".", "--no-deps", "--wheel-dir", str(wheel_dir)],
        cwd=SDK_ROOT,
        env=env,
        check=True,
        capture_output=True,
        text=True,
    )
    wheel = next(wheel_dir.glob("jobdock_sdk-0.3.0rc1-*.whl"))
    target = tmp_path / "installed"
    subprocess.run([sys.executable, "-m", "pip", "install", "--no-deps", "--target", str(target), str(wheel)], check=True, capture_output=True, text=True)
    output = subprocess.check_output(
        [sys.executable, "-c", "import jobdock; print(jobdock.__version__)"],
        env={**os.environ, "PYTHONPATH": str(target)},
        text=True,
    ).strip()
    assert output == "0.3.0rc1"
