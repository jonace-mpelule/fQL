"""Verify checksums and smoke-test the native CLI without Go or external tools."""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import tarfile
import tempfile
import zipfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", required=True)
    parser.add_argument("--directory", default="dist")
    parser.add_argument("--binary-only", action="store_true")
    args = parser.parse_args()
    directory = Path(args.directory)
    records = (directory / "SHA256SUMS").read_text().splitlines()
    if len(records) != 7:
        raise RuntimeError("Expected five archives and two installers")
    archive_names = set()
    for record in records:
        expected, name = record.split("  ", 1)
        if Path(name).name != name or name in archive_names:
            raise RuntimeError("Invalid or duplicate archive name")
        archive_names.add(name)
        data = (directory / name).read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f"Checksum mismatch: {name}")
        if name in ("install.sh", "install.ps1"):
            continue
        if name.endswith(".zip"):
            with zipfile.ZipFile(directory / name) as archive:
                if archive.namelist() != ["fql.exe"] or not archive.read("fql.exe").startswith(b"MZ"):
                    raise RuntimeError("Invalid Windows archive")
        else:
            with tarfile.open(directory / name) as archive:
                if archive.getnames() != ["fql"]:
                    raise RuntimeError("Unexpected archive contents")
                binary = archive.extractfile("fql").read()
                if "_linux_" in name and not binary.startswith(b"\x7fELF"):
                    raise RuntimeError("Invalid Linux executable")
    if not {"install.sh", "install.ps1"}.issubset(archive_names):
        raise RuntimeError("Installers are missing")
    target_os = {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}[platform.system()]
    target_arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}[platform.machine().lower()]
    extension = ".zip" if target_os == "windows" else ".tar.gz"
    name = f"fql_{args.version}_{target_os}_{target_arch}{extension}"
    filename = "fql.exe" if target_os == "windows" else "fql"
    with tempfile.TemporaryDirectory(prefix="fql-install-") as temporary:
        temporary = Path(temporary)
        if extension == ".zip":
            with zipfile.ZipFile(directory / name) as archive:
                binary_data = archive.read(filename)
        else:
            with tarfile.open(directory / name) as archive:
                binary_data = archive.extractfile(filename).read()
        binary = temporary / filename
        binary.write_bytes(binary_data)
        binary.chmod(0o755)
        environment = dict(os.environ, PATH="")
        actual = subprocess.check_output([str(binary), "version"], env=environment, text=True)
        if actual != f"fql {args.version}\n":
            raise RuntimeError("Wrong build version")
        project = str(temporary / "app")
        for command in ([] if args.binary_only else [["init"], ["validate"], ["format"], ["format", "--check"], ["generate"], ["generate", "--check"], ["diff", "--check"]]):
            subprocess.run([str(binary), *command, "--project", project], env=environment,
                           check=True, capture_output=True, text=True, timeout=30)
    print("Verified release assets and native binary version" if args.binary_only else "Verified five archives and completed a native install/init/validate/generate/check with empty PATH")


if __name__ == "__main__":
    main()
