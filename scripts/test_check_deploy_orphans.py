import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_deploy_orphans.py")
SPEC = importlib.util.spec_from_file_location("check_deploy_orphans", SCRIPT_PATH)
mod = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mod)


def write(root: Path, rel: str, text: str = "kind: X\n") -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


class CheckDeployOrphansTest(unittest.TestCase):
    def run_check(self, files: dict[str, str]) -> int:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for rel, text in files.items():
                write(root, rel, text)
            mod.ROOT = root
            mod.DEPLOY = root / "deploy"
            mod.BASE = mod.DEPLOY / "base"
            return mod.main()

    def test_referenced_manifest_passes(self):
        self.assertEqual(0, self.run_check({
            "deploy/base/kustomization.yaml": "resources:\n  - app.yaml\n",
            "deploy/base/app.yaml": "kind: X\n",
        }))

    def test_bare_orphan_fails(self):
        self.assertEqual(1, self.run_check({
            "deploy/base/kustomization.yaml": "resources:\n  - app.yaml\n",
            "deploy/base/app.yaml": "kind: X\n",
            "deploy/base/old/deployment.yaml": "kind: X\n",
        }))

    def test_orphan_subtree_with_own_kustomization_fails(self):
        self.assertEqual(1, self.run_check({
            "deploy/base/kustomization.yaml": "resources:\n  - app.yaml\n",
            "deploy/base/app.yaml": "kind: X\n",
            "deploy/base/dead/kustomization.yaml": "resources:\n  - d.yaml\n",
            "deploy/base/dead/d.yaml": "kind: X\n",
        }))

    def test_subtree_referenced_by_overlay_passes(self):
        self.assertEqual(0, self.run_check({
            "deploy/base/kustomization.yaml": "resources:\n  - app.yaml\n",
            "deploy/base/app.yaml": "kind: X\n",
            "deploy/base/kc/kustomization.yaml": "resources:\n  - k.yaml\n",
            "deploy/base/kc/k.yaml": "kind: X\n",
            "deploy/kind/kustomization.yaml": "resources:\n  - ../base/kc/\n",
        }))


if __name__ == "__main__":
    unittest.main()
