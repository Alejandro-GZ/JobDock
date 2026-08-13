"""Hatch metadata hook for the product-derived SDK version."""

from pathlib import Path
from runpy import run_path

from hatchling.metadata.plugin.interface import MetadataHookInterface


class CustomMetadataHook(MetadataHookInterface):
    def update(self, metadata: dict) -> None:
        versioning = run_path(str(Path(__file__).with_name("versioning.py")), run_name="jobdock_sdk_versioning")
        metadata["version"] = versioning["resolve_build_version"]()
