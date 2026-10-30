import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_migration_bodies.py")
SPEC = importlib.util.spec_from_file_location("check_migration_bodies", SCRIPT_PATH)
mod = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mod)


MIGRATION_TEMPLATE = """package example

func migration() *gormigrate.Migration {{
\treturn &gormigrate.Migration{{
\t\tID: "{mig_id}",
\t\tMigrate: func(tx *gorm.DB) error {{
\t\t\t{body}
\t\t}},
\t}}
}}
"""


class CheckMigrationBodiesTest(unittest.TestCase):
    def setup(self, tmp: Path) -> None:
        mod.ROOT = tmp
        mod.PLUGINS = tmp / "components" / "api-server" / "plugins"
        mod.LOCK = tmp / "components" / "api-server" / "migration-bodies.lock"

    def write_migration(self, tmp: Path, name: str, mig_id: str, body: str) -> None:
        path = mod.PLUGINS / name / "migration.go"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(MIGRATION_TEMPLATE.format(mig_id=mig_id, body=body), encoding="utf-8")

    def test_matching_lock_passes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.setup(root)
            self.write_migration(root, "foo", "2026010100000001", "return createFoo(tx)")
            self.assertEqual(0, mod.main(["--update"]))
            self.assertEqual(0, mod.main([]))

    def test_changed_body_same_id_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.setup(root)
            self.write_migration(root, "foo", "2026010100000001", "return createFoo(tx)")
            self.assertEqual(0, mod.main(["--update"]))
            # Edit the body in place under the same ID: this is the #862 defect.
            self.write_migration(root, "foo", "2026010100000001", "return createFooFixed(tx)")
            self.assertEqual(1, mod.main([]))

    def test_comment_and_whitespace_change_passes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.setup(root)
            self.write_migration(root, "foo", "2026010100000001", "return createFoo(tx)")
            self.assertEqual(0, mod.main(["--update"]))
            # Reformat and add a comment: the normalized body is unchanged.
            self.write_migration(
                root, "foo", "2026010100000001", "// a note\n\t\t\treturn    createFoo(tx)"
            )
            self.assertEqual(0, mod.main([]))

    def test_new_id_requires_lock_update(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.setup(root)
            self.write_migration(root, "foo", "2026010100000001", "return createFoo(tx)")
            self.assertEqual(0, mod.main(["--update"]))
            # Adding a convergence migration under a NEW ID is the correct fix,
            # but the lock must be regenerated to record it.
            self.write_migration(root, "bar", "2026010100000002", "return convergeFoo(tx)")
            self.assertEqual(1, mod.main([]))
            self.assertEqual(0, mod.main(["--update"]))
            self.assertEqual(0, mod.main([]))

    def test_removed_migration_requires_lock_update(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.setup(root)
            self.write_migration(root, "foo", "2026010100000001", "return createFoo(tx)")
            self.write_migration(root, "bar", "2026010100000002", "return createBar(tx)")
            self.assertEqual(0, mod.main(["--update"]))
            (mod.PLUGINS / "bar" / "migration.go").unlink()
            self.assertEqual(1, mod.main([]))

    def test_duplicate_id_differing_body_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.setup(root)
            self.write_migration(root, "foo", "2026010100000001", "return createFoo(tx)")
            self.write_migration(root, "bar", "2026010100000001", "return createBar(tx)")
            with self.assertRaises(ValueError):
                mod.main([])


if __name__ == "__main__":
    unittest.main()
