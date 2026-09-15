"""Check release gates and the bundle contents without cluster access."""

import copy
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest
from unittest.mock import patch

from scripts import release_bundle as bundle


def fixtures():
    components, images = [], []
    for index, name in enumerate(bundle.COMPONENTS):
        digest = "sha256:" + str(index + 1) * 64
        components.append({"name": name, "containerImage": bundle.BUILD_PREFIX + name + "@" + digest,
                           "source": {"git": {"url": bundle.SOURCE_URL, "revision": str(index + 1) * 40}}})
        images.append({"name": name, "shasum": digest,
                       "urls": [bundle.RELEASE_PREFIX + name + ":latest"]})
    snapshot = {"metadata": {"name": "snapshot-one", "namespace": bundle.NAMESPACE,
                             "uid": "snapshot-uid", "creationTimestamp": "2026-09-15T12:00:00Z"},
                "spec": {"application": bundle.APPLICATION, "components": components}}
    release = {"metadata": {"name": "release-one", "namespace": bundle.NAMESPACE, "uid": "release-uid"},
               "spec": {"snapshot": "snapshot-one", "releasePlan": "hypershell-releaseplan"},
               "status": {"conditions": [{"type": "ManagedPipelineProcessed", "status": "True",
                                          "reason": "Succeeded"}], "artifacts": {"images": images}}}
    return release, snapshot


class ReleaseBundleTests(unittest.TestCase):
    def test_preserves_each_component_revision_and_released_digest(self):
        release, snapshot = fixtures()
        tag, result = bundle.make_bundle(release, snapshot)
        self.assertTrue(tag.startswith("release-bundle-20260915T120000000000Z-"))
        for item, source in zip(result["components"], snapshot["spec"]["components"]):
            self.assertEqual(item["source"], source["source"])
            self.assertEqual(item["image"], source["containerImage"].replace(bundle.BUILD_PREFIX, bundle.RELEASE_PREFIX))
        self.assertEqual((tag, result), bundle.make_bundle(release, snapshot))

    def test_failed_pending_and_skipped_releases_cannot_reach_registry(self):
        for status, reason in (("False", "Failed"), ("Unknown", "Progressing"), ("True", "Skipped")):
            release, snapshot = fixtures()
            release["status"]["conditions"][0].update(status=status, reason=reason)
            with self.subTest(status=status, reason=reason), patch.object(bundle, "run") as run:
                with self.assertRaises(ValueError):
                    bundle.publish(release, snapshot, ".", "/unused")
                run.assert_not_called()

    def test_missing_or_duplicate_components_fail(self):
        for location in ("snapshot", "release"):
            for duplicate in (False, True):
                release, snapshot = fixtures()
                items = snapshot["spec"]["components"] if location == "snapshot" else release["status"]["artifacts"]["images"]
                if duplicate:
                    items.append(copy.deepcopy(items[0]))
                else:
                    items.pop()
                with self.subTest(location=location, duplicate=duplicate), self.assertRaises(ValueError):
                    bundle.make_bundle(release, snapshot)

    def test_wrong_image_or_source_fails(self):
        mutations = (
            lambda r, s: r["status"]["artifacts"]["images"][0].update(shasum="sha256:" + "a" * 64),
            lambda r, s: r["status"]["artifacts"]["images"][0].update(urls=["quay.io/other/repo:latest"]),
            lambda r, s: s["spec"]["components"][0]["source"]["git"].update(revision="main"),
            lambda r, s: s["spec"]["components"][0]["source"]["git"].update(url="https://example.com/repo"),
            lambda r, s: r["spec"].update(snapshot="other"),
            lambda r, s: s["spec"].update(application="other"),
        )
        for mutate in mutations:
            release, snapshot = fixtures()
            mutate(release, snapshot)
            with self.assertRaises(ValueError):
                bundle.make_bundle(release, snapshot)

    def test_pull_request_and_merge_queue_events_fail(self):
        for field in ("annotations", "labels"):
            for event in ("pull_request", "merge_group"):
                release, snapshot = fixtures()
                snapshot["metadata"][field] = {"pac.test.appstudio.openshift.io/event-type": event}
                with self.subTest(field=field, event=event), self.assertRaises(ValueError):
                    bundle.make_bundle(release, snapshot)

    def test_old_snapshot_does_not_get_a_new_timestamp_on_retry(self):
        release, snapshot = fixtures()
        first, _ = bundle.make_bundle(release, snapshot)
        release["metadata"]["uid"] = "retry-uid"
        retry, _ = bundle.make_bundle(release, snapshot)
        self.assertEqual(first.split("-")[2], retry.split("-")[2])
        snapshot["metadata"]["creationTimestamp"] = "2026-09-16T12:00:00Z"
        newer, _ = bundle.make_bundle(release, snapshot)
        self.assertGreater(newer, retry)


class PipelineBootstrapTests(unittest.TestCase):
    def run_bootstrap(self, revision, read_status=0):
        pipeline = Path(__file__).resolve().parents[1] / "pipelines/release-bundle/pipeline.yaml"
        script = textwrap.dedent(pipeline.read_text().split("            script: |\n", 1)[1])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = root / "commands"
            stubs = {
                "kubectl": 'printf "%s\\n" "$*" >> "$COMMAND_LOG"\n'
                           'printf "%s" "$RESOLVED_REVISION"\nexit "$READ_STATUS"\n',
                "git": 'printf "git %s\\n" "$*" >> "$COMMAND_LOG"\n',
                "python3": 'printf "publisher started\\n" >> "$COMMAND_LOG"\n',
            }
            for name, body in stubs.items():
                executable = root / name
                executable.write_text("#!/bin/sh\n" + body)
                executable.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       COMMAND_LOG=str(commands), RESOLVED_REVISION=revision,
                       READ_STATUS=str(read_status), PIPELINE_RUN="final-one",
                       PIPELINE_NAMESPACE=bundle.NAMESPACE, RELEASE="release-one",
                       SNAPSHOT="snapshot-one", BUNDLE_RESULT=str(root / "result"))
            result = subprocess.run(["bash", "-c", script], env=env, text=True, capture_output=True)
            return result, commands.read_text()

    def test_script_uses_pipeline_commit_instead_of_main(self):
        revision = "a" * 40
        result, commands = self.run_bootstrap(revision)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("get pipelineruns.tekton.dev final-one -n " + bundle.NAMESPACE, commands)
        self.assertIn("{.status.provenance.refSource.digest.sha1}", commands)
        self.assertIn("main:refs/remotes/origin/main " + revision, commands)
        self.assertIn("checkout --quiet " + revision + " -- scripts/release_bundle.py", commands)
        self.assertIn("publisher started", commands)

    def test_missing_or_invalid_commit_stops_before_fetch(self):
        for revision in ("", "main", "a" * 39, "$(touch /tmp/unexpected)"):
            with self.subTest(revision=revision):
                result, commands = self.run_bootstrap(revision)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("no valid resolved Git commit", result.stderr)
                self.assertNotIn("git ", commands)
                self.assertNotIn("publisher started", commands)

    def test_pipeline_read_failure_stops_publication(self):
        result, commands = self.run_bootstrap("a" * 40, read_status=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("git ", commands)
        self.assertNotIn("publisher started", commands)


if __name__ == "__main__":
    unittest.main()
