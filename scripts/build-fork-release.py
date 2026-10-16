#!/usr/bin/env python3
"""Build deterministic, checksummed provider archives without publishing or credentials."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "arm64"))
UPSTREAM = "86f6d55fceb122c1d6b3c69d10b6753fdc49f877"


def archive(destination: Path, binary: Path) -> str:
	"""Exclude machine paths and timestamps so identical inputs have identical archives."""
	with zipfile.ZipFile(destination, "w", compression=zipfile.ZIP_DEFLATED) as package:
		for name, data, mode in ((binary.name, binary.read_bytes(), 0o100755), ("LICENSE", (ROOT / "LICENSE").read_bytes(), 0o100644)):
			info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
			info.create_system = 3
			info.external_attr = mode << 16
			info.compress_type = zipfile.ZIP_DEFLATED
			package.writestr(info, data)
	return hashlib.sha256(destination.read_bytes()).hexdigest()


def main() -> None:
	"""Require a clean reviewed source and local Go toolchain before producing release files."""
	parser = argparse.ArgumentParser(description=__doc__)
	parser.add_argument("--version", required=True)
	parser.add_argument("--output", required=True, type=Path)
	args = parser.parse_args()
	if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+-fork\.[0-9]+", args.version):
		parser.error("Use an explicit fork release version, for example 0.12.1-fork.1.")
	if subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True).strip():
		raise SystemExit("Release source must be clean and committed.")
	commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
	environment = dict(os.environ, GOTOOLCHAIN="local", CGO_ENABLED="0")
	toolchain = subprocess.check_output(["go", "env", "GOVERSION"], env=environment, text=True).strip()
	if toolchain != "go1.27.2":
		raise SystemExit("Use the reviewed Go 1.27.2 toolchain.")
	args.output.mkdir(parents=True, exist_ok=True)
	manifest = {"version": args.version, "commit": commit, "upstream_commit": UPSTREAM, "go_version": toolchain, "archives": {}}
	with tempfile.TemporaryDirectory(prefix="provider-release-") as temporary:
		for operating_system, architecture in TARGETS:
			binary = Path(temporary) / ("terraform-provider-googleworkspace_v" + args.version)
			environment.update(GOOS=operating_system, GOARCH=architecture)
			subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X main.version=" + args.version + " -X main.commit=" + commit, "-o", str(binary), "."], cwd=ROOT, env=environment, check=True)
			filename = f"terraform-provider-googleworkspace_{args.version}_{operating_system}_{architecture}.zip"
			manifest["archives"][filename] = archive(args.output / filename, binary)
	manifest_name = f"terraform-provider-googleworkspace_{args.version}_source.json"
	manifest_path = args.output / manifest_name
	manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
	checksums = dict(manifest["archives"], **{manifest_name: hashlib.sha256(manifest_path.read_bytes()).hexdigest()})
	(args.output / f"terraform-provider-googleworkspace_{args.version}_SHA256SUMS").write_text("".join(digest + "  " + filename + "\n" for filename, digest in sorted(checksums.items())))


if __name__ == "__main__":
	main()
