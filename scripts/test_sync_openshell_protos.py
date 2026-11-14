"""Unit tests for scripts/sync_openshell_protos.py."""

import importlib.util
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "sync_openshell_protos.py"
spec = importlib.util.spec_from_file_location("sync_openshell_protos", SCRIPT)
sync = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync)


def make_repo(tmp: Path, tag: str = "v0.1.8-rhaiv.0") -> Path:
    """Build a minimal repository tree with vendored protos and a pin."""
    directory = sync.vendored_dir(tmp)
    directory.mkdir(parents=True)
    for name in sync.VENDORED_FILES:
        (directory / name).write_text(f"// {name}\n", encoding="utf-8")
    (tmp / "OPENSHELL_VERSION").write_text(
        f"# comment\nOPENSHELL_TAG={tag}\n", encoding="utf-8"
    )
    return tmp


class StampAndCheck(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = make_repo(Path(self._tmp.name))

    def test_stamp_then_check_passes(self) -> None:
        self.assertEqual(sync.stamp(self.root), 0)
        self.assertEqual(sync.check(self.root), 0)
        pin = sync.parse_pin(self.root)
        self.assertEqual(pin["OPENSHELL_TAG"], "v0.1.8-rhaiv.0")
        for name in sync.VENDORED_FILES:
            self.assertIn(name, pin)

    def test_check_fails_before_stamp(self) -> None:
        # No pin file at all: the check must fail loudly, not pass silently.
        with self.assertRaises(FileNotFoundError):
            sync.check(self.root)

    def test_check_fails_on_hand_edit(self) -> None:
        sync.stamp(self.root)
        target = sync.vendored_dir(self.root) / "extension.proto"
        target.write_text("// edited\n", encoding="utf-8")
        self.assertEqual(sync.check(self.root), 1)

    def test_check_fails_on_missing_file(self) -> None:
        sync.stamp(self.root)
        (sync.vendored_dir(self.root) / "options.proto").unlink()
        self.assertEqual(sync.check(self.root), 1)

    def test_check_fails_on_extra_file(self) -> None:
        sync.stamp(self.root)
        (sync.vendored_dir(self.root) / "extra.proto").write_text("// x\n")
        self.assertEqual(sync.check(self.root), 1)

    def test_check_fails_on_tag_drift(self) -> None:
        sync.stamp(self.root)
        (self.root / "OPENSHELL_VERSION").write_text(
            "OPENSHELL_TAG=v0.2.0-rhaiv.0\n", encoding="utf-8"
        )
        self.assertEqual(sync.check(self.root), 1)


if __name__ == "__main__":
    unittest.main()
