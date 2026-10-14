#!/usr/bin/env python3
"""Verify provenance and embedded versions before copying a freshly built IPA."""
import os
import plistlib
import re
import shutil
import sys
import tempfile
import zipfile
from pathlib import Path


def version_parts(raw):
    match = re.fullmatch(r"(\d+\.\d+\.\d+)\+(\d+)", raw)
    if not match:
        raise ValueError("invalid app version")
    return match.groups()


def prepare(work, raw_version):
    version, build = version_parts(raw_version)
    for path in work.glob("*Info.plist"):
        data = plistlib.loads(path.read_bytes())
        data["CFBundleShortVersionString"] = version
        data["CFBundleVersion"] = build
        path.write_bytes(plistlib.dumps(data))


def publish(work, raw_version, destination):
    version, build = version_parts(raw_version)
    candidates = set()
    for line in (work / "build.log").read_text().splitlines():
        if "Wrote to " in line:
            candidate = Path(line.split("Wrote to ", 1)[1].strip())
            if not candidate.is_absolute():
                candidate = work / candidate
            candidates.add(candidate.resolve())
    if len(candidates) != 1:
        raise ValueError("xtool must report exactly one fresh IPA")
    source = candidates.pop()
    if not source.is_relative_to(work.resolve()) or source.suffix != ".ipa" or not source.is_file():
        raise ValueError("IPA is outside this build directory or missing")
    with zipfile.ZipFile(source) as archive:
        if archive.testzip() is not None:
            raise ValueError("IPA is corrupt")
        plists = [name for name in archive.namelist() if re.fullmatch(r"Payload/[^/]+\.app/Info\.plist", name)]
        if len(plists) != 1:
            raise ValueError("IPA must contain one app")
        plists += [name for name in archive.namelist() if re.fullmatch(r"Payload/[^/]+\.app/PlugIns/[^/]+\.appex/Info\.plist", name)]
        for name in plists:
            data = plistlib.loads(archive.read(name))
            if data.get("CFBundleShortVersionString") != version or str(data.get("CFBundleVersion")) != build:
                raise ValueError("IPA contains an app/extension from another version")
    destination.mkdir(parents=True, exist_ok=True)
    output = destination / f"NagaNetwork-{version}-ios.ipa"
    fd, temporary = tempfile.mkstemp(prefix=".naga-ipa-", dir=destination)
    try:
        with os.fdopen(fd, "wb") as target, source.open("rb") as src:
            shutil.copyfileobj(src, target)
            target.flush()
            os.fsync(target.fileno())
        os.replace(temporary, output)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    print(f"IPA: {output}")


if __name__ == "__main__":
    try:
        if sys.argv[1] == "prepare":
            prepare(Path(sys.argv[2]), sys.argv[3])
        elif sys.argv[1] == "publish":
            publish(Path(sys.argv[2]), sys.argv[3], Path(sys.argv[4]))
        else:
            raise ValueError("unknown operation")
    except (ValueError, OSError, zipfile.BadZipFile, plistlib.InvalidFileException) as error:
        print(f"IPA validation failed: {error}", file=sys.stderr)
        sys.exit(1)
