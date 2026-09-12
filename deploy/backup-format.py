#!/usr/bin/env python3
"""Validate private backup bundles without printing their contents."""

import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import sys
import tarfile
import zlib
from datetime import datetime, timezone


def digest(path):
    with path.open("rb") as source:
        checksum = hashlib.sha256()
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            checksum.update(chunk)
        return checksum.hexdigest()


def validate_files(directory):
    for name in ("database.dump", "uploads.tar.gz"):
        path = directory / name
        if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
            raise ValueError("missing backup file")
    # The application stores ordinary files only. Reject links, devices, absolute
    # paths and traversal even when a bundle came from a trusted operator.
    with tarfile.open(directory / "uploads.tar.gz", "r:gz") as archive:
        seen = set()
        for member in archive:
            path = PurePosixPath(member.name)
            if (
                path.is_absolute()
                or ".." in path.parts
                or not (member.isfile() or member.isdir())
                or str(path) in seen
            ):
                raise ValueError("unsafe archive entry")
            seen.add(str(path))


def main():
    action = sys.argv[1]
    directory = Path(sys.argv[2]).resolve()
    if action == "destination":
        repo = Path(sys.argv[3]).resolve()
        if directory == repo or repo in directory.parents or directory.exists():
            raise ValueError("destination must be new and outside the repository")
        return
    validate_files(directory)
    if action == "create":
        metadata = {
            "format": 1,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "source_project": sys.argv[3],
            "app_image_id": sys.argv[4],
            "postgres_major": 17,
            "sha256": {name: digest(directory / name) for name in ("database.dump", "uploads.tar.gz")},
        }
        temporary = directory / "manifest.json.partial"
        with temporary.open("x", encoding="utf-8") as target:
            json.dump(metadata, target, indent=2)
            target.write("\n")
        os.replace(temporary, directory / "manifest.json")
    elif action == "verify":
        manifest = directory / "manifest.json"
        if manifest.is_symlink() or (directory / "INCOMPLETE").exists():
            raise ValueError("incomplete backup")
        metadata = json.loads(manifest.read_text(encoding="utf-8"))
        if metadata["format"] != 1 or metadata["postgres_major"] != 17:
            raise ValueError("unsupported backup format")
        if metadata["source_project"] == sys.argv[3]:
            raise ValueError("restoring to the source project is forbidden")
        if metadata["app_image_id"] != sys.argv[4]:
            raise ValueError("use the exact application image from the backup")
        for name in ("database.dump", "uploads.tar.gz"):
            if digest(directory / name) != metadata["sha256"][name]:
                raise ValueError("checksum mismatch")
    else:
        raise ValueError("unknown operation")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, tarfile.TarError, IndexError, EOFError, zlib.error):
        # Never echo archive member names, database content or operator secrets.
        print("Backup validation failed; check format, checksums, destination and image identity.", file=sys.stderr)
        sys.exit(1)
