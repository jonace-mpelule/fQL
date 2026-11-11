"""Verify supported release archives and smoke-test the native CLI without Go."""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import tarfile
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", required=True)
    parser.add_argument("--directory", default="dist")
    parser.add_argument("--binary-only", action="store_true")
    args = parser.parse_args()
    directory = Path(args.directory)
    expected_names = {f"fql_{args.version}_{system}_{arch}.tar.gz"
                      for system in ("darwin", "linux") for arch in ("arm64", "amd64")}
    expected_names.add("install.sh")
    records = (directory / "SHA256SUMS").read_text().splitlines()
    if len(records) != len(expected_names):
        raise RuntimeError("Expected four archives and one installer")
    seen = set()
    for record in records:
        expected, name = record.split("  ", 1)
        if name not in expected_names or name in seen:
            raise RuntimeError("Unexpected or duplicate release asset")
        seen.add(name)
        data = (directory / name).read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f"Checksum mismatch: {name}")
        if name == "install.sh":
            continue
        with tarfile.open(directory / name) as archive:
            if archive.getnames() != ["fql"]:
                raise RuntimeError("Unexpected archive contents")
            binary = archive.extractfile("fql").read()
            if "_linux_" in name and not binary.startswith(b"\x7fELF"):
                raise RuntimeError("Invalid Linux executable")
    if seen != expected_names:
        raise RuntimeError("Release assets are incomplete")
    target_os = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    if target_os is None:
        raise RuntimeError("Native verification supports macOS and Linux only")
    target_arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}[platform.machine().lower()]
    name = f"fql_{args.version}_{target_os}_{target_arch}.tar.gz"
    with tempfile.TemporaryDirectory(prefix="fql-install-") as temporary:
        temporary = Path(temporary)
        with tarfile.open(directory / name) as archive:
            binary_data = archive.extractfile("fql").read()
        binary = temporary / "fql"
        binary.write_bytes(binary_data)
        binary.chmod(0o755)
        environment = dict(os.environ, PATH="")
        actual = subprocess.check_output([str(binary), "version"], env=environment, text=True)
        if actual != f"fql {args.version}\n":
            raise RuntimeError("Wrong build version")
        project = str(temporary / "app")
        commands = [] if args.binary_only else [["init"], ["validate"], ["format"], ["format", "--check"], ["generate"], ["generate", "--check"], ["diff", "--check"]]
        for command in commands:
            subprocess.run([str(binary), *command, "--project", project], env=environment,
                           check=True, capture_output=True, text=True, timeout=30)
    print("Verified release assets and native binary version" if args.binary_only else "Verified four archives and completed native CLI smoke checks with empty PATH")


if __name__ == "__main__":
    main()
