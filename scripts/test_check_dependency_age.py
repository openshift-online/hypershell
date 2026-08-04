import datetime as dt
import importlib.util
import io
import json
import subprocess
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from unittest import mock


SCRIPT_PATH = Path(__file__).with_name("check_dependency_age.py")
SPEC = importlib.util.spec_from_file_location("check_dependency_age", SCRIPT_PATH)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)
UTC = dt.timezone.utc


class DependencyAgeTest(unittest.TestCase):
    def test_package_lock_versions_and_exact_root_declarations(self):
        with tempfile.TemporaryDirectory() as directory:
            lockfile = Path(directory) / "package-lock.json"
            lockfile.write_text(
                json.dumps(
                    {
                        "lockfileVersion": 3,
                        "packages": {
                            "": {"devDependencies": {"typescript": "5.9.3"}},
                            "node_modules/typescript": {"version": "5.9.3"},
                            "node_modules/example/node_modules/@scope/child": {
                                "version": "1.2.3"
                            },
                            "node_modules/local": {"version": "1.0.0", "link": True},
                        },
                    }
                ),
                encoding="utf-8",
            )
            self.assertEqual(
                CHECKER.npm_package_versions(lockfile),
                {
                    ("typescript", "5.9.3", str(lockfile)),
                    ("@scope/child", "1.2.3", str(lockfile)),
                },
            )
            self.assertEqual(CHECKER.exact_npm_declaration_failures(lockfile), [])

            data = json.loads(lockfile.read_text(encoding="utf-8"))
            data["packages"][""]["devDependencies"]["typescript"] = "^5.9.3"
            lockfile.write_text(json.dumps(data), encoding="utf-8")
            self.assertIn(
                "not an exact semantic version",
                CHECKER.exact_npm_declaration_failures(lockfile)[0],
            )

    def test_external_local_replace_uses_declared_version(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            module = root / "components" / "service"
            module.mkdir(parents=True)
            go_mod = module / "go.mod"
            go_mod.write_text(
                "module example.test/service\n\n"
                "go 1.26.4\n\n"
                "require example.test/framework v1.2.3\n\n"
                "replace example.test/framework => ../../../framework\n",
                encoding="utf-8",
            )
            go_mod.with_name("go.sum").write_text("", encoding="utf-8")

            edit_json = json.dumps(
                {
                    "Replace": [
                        {
                            "Old": {"Path": "example.test/framework"},
                            "New": {"Path": "../../../framework"},
                        }
                    ]
                }
            )
            graph_json = json.dumps(
                {
                    "Path": "example.test/framework",
                    "Version": "v1.2.3",
                    "Time": "2026-01-01T00:00:00Z",
                }
            )
            calls = []

            def fake_run(arguments, **kwargs):
                calls.append(arguments)
                if arguments[-1] == "-json" and arguments[:3] == ["go", "mod", "edit"]:
                    return subprocess.CompletedProcess(arguments, 0, edit_json, "")
                if arguments[:3] == ["go", "mod", "edit"]:
                    return subprocess.CompletedProcess(arguments, 0, "", "")
                return subprocess.CompletedProcess(arguments, 0, graph_json, "")

            with mock.patch.object(CHECKER.subprocess, "run", side_effect=fake_run):
                self.assertEqual(
                    CHECKER.go_modules(go_mod, root),
                    [
                        (
                            "example.test/framework",
                            "v1.2.3",
                            "2026-01-01T00:00:00Z",
                            str(go_mod),
                        )
                    ],
                )

            self.assertTrue(
                any(
                    "-dropreplace=example.test/framework" in arguments
                    for arguments in calls
                )
            )

    def test_cutoff_boundary_and_exact_allowlist(self):
        cutoff = dt.datetime(2026, 6, 6, tzinfo=UTC)
        source = "package-lock.json"
        self.assertIsNone(
            CHECKER.check_age("npm", "safe", "1.0.0", cutoff, source, cutoff, set())
        )
        published = cutoff + dt.timedelta(seconds=1)
        failure = CHECKER.check_age(
            "npm", "new", "1.0.0", published, source, cutoff, set()
        )
        self.assertIn("after cutoff", failure)
        allowlist = {("npm", "new", "1.0.0")}
        self.assertIsNone(
            CHECKER.check_age(
                "npm", "new", "1.0.0", published, source, cutoff, allowlist
            )
        )
        self.assertIsNotNone(
            CHECKER.check_age(
                "npm", "new", "1.0.1", published, source, cutoff, allowlist
            )
        )

    def test_pseudo_version_time(self):
        self.assertEqual(
            CHECKER.pseudo_version_time("v0.0.0-20260601010203-0123456789abcdef"),
            dt.datetime(2026, 6, 1, 1, 2, 3, tzinfo=UTC),
        )
        self.assertIsNone(CHECKER.pseudo_version_time("v1.2.3"))

    def test_missing_publish_time_fails_closed(self):
        with self.assertRaisesRegex(RuntimeError, "did not include a publish time"):
            CHECKER.go_published_at("example.test/module", "v1.2.3", None)

    def test_allowlist_requires_auditable_fields_and_known_kind(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / CHECKER.ALLOWLIST_PATH).write_text(
                json.dumps(
                    [
                        {"kind": "npm", "name": "ok", "version": "1.0.0"},
                        {
                            "kind": "cargo",
                            "name": "bad",
                            "version": "1.0.0",
                            "reason": "test",
                            "compensatingVerification": "test",
                        },
                        {
                            "kind": 7,
                            "name": "bad-type",
                            "version": "1.0.0",
                            "reason": "test",
                            "compensatingVerification": "test",
                        },
                    ]
                ),
                encoding="utf-8",
            )
            allowed, failures = CHECKER.load_allowlist(root)
            self.assertEqual(allowed, set())
            self.assertEqual(len(failures), 3)

    def test_metadata_transport_requires_https(self):
        with self.assertRaisesRegex(RuntimeError, "must use HTTPS"):
            CHECKER.fetch_json("http://registry.example.test/package")

    def test_invalid_timestamp_is_rejected(self):
        with self.assertRaises(ValueError):
            CHECKER.parse_time("not-a-timestamp")

    def test_now_override_drives_cutoff(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            lockfile = root / "package-lock.json"
            lockfile.write_text(
                json.dumps(
                    {
                        "lockfileVersion": 3,
                        "packages": {
                            "": {"devDependencies": {"example": "1.0.0"}},
                            "node_modules/example": {"version": "1.0.0"},
                        },
                    }
                ),
                encoding="utf-8",
            )
            published = dt.datetime(2026, 6, 7, tzinfo=UTC)
            with mock.patch.object(CHECKER, "npm_published_at", return_value=published):
                with redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                    self.assertEqual(
                        CHECKER.main(
                            [
                                "--root",
                                str(root),
                                "--skip-go",
                                "--now",
                                "2026-06-20T00:00:00Z",
                            ]
                        ),
                        1,
                    )
                    self.assertEqual(
                        CHECKER.main(
                            [
                                "--root",
                                str(root),
                                "--skip-go",
                                "--now",
                                "2026-06-21T00:00:00Z",
                            ]
                        ),
                        0,
                    )

    def test_tool_dependencies_require_exact_versions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / CHECKER.TOOLS_PATH).write_text(
                json.dumps(
                    [
                        {"kind": "npm", "name": "typescript", "version": "5.3.3"},
                        {"kind": "npm", "name": "bad", "version": "^1.0.0"},
                    ]
                ),
                encoding="utf-8",
            )
            tools, failures = CHECKER.load_tools(root)
            self.assertEqual(
                tools,
                [("npm", "typescript", "5.3.3", str(root / CHECKER.TOOLS_PATH))],
            )
            self.assertEqual(len(failures), 1)


if __name__ == "__main__":
    unittest.main()
