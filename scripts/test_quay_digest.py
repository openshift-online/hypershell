#!/usr/bin/env python3
"""Unit tests for scripts/quay_digest.py (no network)."""
import importlib.util
import unittest
from pathlib import Path
from urllib.parse import parse_qs, urlparse

SPEC = importlib.util.spec_from_file_location(
    "quay_digest", Path(__file__).resolve().parent / "quay_digest.py"
)
mod = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mod)

D1 = "sha256:" + "1" * 64
D2 = "sha256:" + "2" * 64


def query(url: str) -> dict:
    return {k: v[0] for k, v in parse_qs(urlparse(url).query).items()}


class ResolveTest(unittest.TestCase):
    def test_returns_digest_of_exact_tag(self):
        calls = []

        def fetch(url):
            calls.append(url)
            return {"tags": [{"name": "0.1.2", "manifest_digest": D1}]}

        self.assertEqual(mod.resolve("gkrumbach07/openshell-dashboard", "0.1.2", fetch), D1)
        self.assertTrue(calls[0].startswith(
            "https://quay.io/api/v1/repository/gkrumbach07/openshell-dashboard/tag/?"))
        self.assertEqual(query(calls[0]), {"specificTag": "0.1.2", "onlyActiveTags": "true"})

    def test_missing_tag_raises(self):
        with self.assertRaises(LookupError):
            mod.resolve("a/b", "v0.1.2", lambda url: {"tags": []})

    def test_ignores_non_matching_names(self):
        fetch = lambda url: {"tags": [{"name": "0.1.20", "manifest_digest": D1}]}
        with self.assertRaises(LookupError):
            mod.resolve("a/b", "0.1.2", fetch)

    def test_rejects_malformed_digest(self):
        fetch = lambda url: {"tags": [{"name": "x", "manifest_digest": "sha256:abc"}]}
        with self.assertRaises(LookupError):
            mod.resolve("a/b", "x", fetch)


class TagsForTest(unittest.TestCase):
    def test_follows_pages_and_dedups(self):
        pages = {
            "1": {"tags": [{"name": "0.1", "manifest_digest": D1},
                           {"name": "latest", "manifest_digest": D2}], "has_additional": True},
            "2": {"tags": [{"name": "0.1.1", "manifest_digest": D1},
                           {"name": "0.1", "manifest_digest": D1}], "has_additional": False},
        }
        fetch = lambda url: pages[query(url)["page"]]
        self.assertEqual(mod.tags_for("a/b", D1, fetch), ["0.1", "0.1.1"])

    def test_stops_after_max_pages(self):
        fetch = lambda url: {"tags": [], "has_additional": True}
        with self.assertRaises(LookupError):
            list(mod.iter_tags("a/b", fetch))


class MainTest(unittest.TestCase):
    def test_rejects_bad_repository(self):
        with self.assertRaises(SystemExit):
            mod.main(["../etc", "x"])

    def test_requires_tag(self):
        with self.assertRaises(SystemExit):
            mod.main(["a/b"])


if __name__ == "__main__":
    unittest.main()
