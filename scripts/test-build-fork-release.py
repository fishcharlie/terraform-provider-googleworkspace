"""Keep release archives portable, deterministic and restricted to public build inputs."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

SPEC = importlib.util.spec_from_file_location("fork_release", Path(__file__).with_name("build-fork-release.py"))
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class ArchiveTests(unittest.TestCase):
	"""Check archive behavior without compiling or publishing a provider."""

	def test_archives_ignore_host_paths_times_and_preserve_executable_mode(self):
		with tempfile.TemporaryDirectory() as temporary:
			root = Path(temporary)
			binary = root / "terraform-provider-googleworkspace_v0.0.1-fork.1"
			binary.write_bytes(b"synthetic executable")
			first = release.archive(root / "first.zip", binary)
			binary.touch()
			second = release.archive(root / "second.zip", binary)
			self.assertEqual(first, second)
			with zipfile.ZipFile(root / "first.zip") as archive:
				self.assertEqual(set(archive.namelist()), {binary.name, "LICENSE"})
				self.assertEqual(archive.getinfo(binary.name).external_attr >> 16, 0o100755)
				self.assertEqual(archive.read(binary.name), b"synthetic executable")


if __name__ == "__main__":
	unittest.main()
