"""Check release gates and the bundle contents without cluster access."""

import copy
import os
import json
import sys
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest
from unittest.mock import patch

from scripts import release_bundle as bundle
from scripts import render_release_bundle_pipeline as renderer


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

    def test_partial_release_preserves_the_complete_snapshot(self):
        release, snapshot = fixtures()
        expected = bundle.make_bundle(release, snapshot)
        release["status"]["artifacts"]["images"] = release["status"]["artifacts"]["images"][:1]
        self.assertEqual(expected, bundle.make_bundle(release, snapshot))

    def test_incomplete_snapshot_and_invalid_artifact_lists_fail(self):
        mutations = (
            lambda r, s: s["spec"]["components"].pop(),
            lambda r, s: s["spec"]["components"].append(copy.deepcopy(s["spec"]["components"][0])),
            lambda r, s: r["status"]["artifacts"]["images"].append(copy.deepcopy(r["status"]["artifacts"]["images"][0])),
            lambda r, s: r["status"]["artifacts"]["images"][0].update(name="unexpected-component"),
            lambda r, s: r["status"]["artifacts"]["images"].clear(),
        )
        for mutate in mutations:
            release, snapshot = fixtures()
            mutate(release, snapshot)
            with self.assertRaises(ValueError):
                bundle.make_bundle(release, snapshot)

    def test_wrong_image_or_source_fails(self):
        mutations = (
            lambda r, s: r["status"]["artifacts"]["images"][0].update(shasum="sha256:" + "a" * 64),
            lambda r, s: r["status"]["artifacts"]["images"][0].update(urls=["quay.io/other/repo:latest"]),
            lambda r, s: s["spec"]["components"][0]["source"]["git"].update(revision="main"),
            lambda r, s: s["spec"]["components"][0]["source"]["git"].update(url="https://example.com/repo"),
            lambda r, s: s["spec"]["components"][1].update(containerImage="quay.io/other/repo@sha256:" + "2" * 64),
            lambda r, s: s["spec"]["components"][1].update(containerImage=bundle.BUILD_PREFIX + bundle.COMPONENTS[1] + "@invalid"),
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

    def test_push_can_retain_merged_pull_request_number(self):
        release, snapshot = fixtures()
        for obj in (release, snapshot):
            obj["metadata"]["annotations"] = {
                "pac.test.appstudio.openshift.io/event-type": "push",
                "pac.test.appstudio.openshift.io/pull-request": "293",
                "pac.test.appstudio.openshift.io/source-branch": "refs/heads/main",
            }
        bundle.make_bundle(release, snapshot)

    def test_pull_request_metadata_requires_matching_push_event(self):
        for event_key in (None, "other.example/event-type"):
            for location in ("release", "snapshot"):
                release, snapshot = fixtures()
                obj = release if location == "release" else snapshot
                obj["metadata"]["annotations"] = {"pac.test.appstudio.openshift.io/pull-request": "293"}
                if event_key:
                    obj["metadata"]["labels"] = {event_key: "push"}
                with self.subTest(event_key=event_key, location=location), self.assertRaises(ValueError):
                    bundle.make_bundle(release, snapshot)


class PipelineBootstrapTests(unittest.TestCase):
    def test_embedded_publisher_matches_source(self):
        pipeline = renderer.PIPELINE.read_text()
        self.assertEqual(pipeline, renderer.render(pipeline, renderer.PUBLISHER.read_text()))

    def run_bootstrap(self, git_status=0, partial_release=False, unavailable_image=""):
        script = textwrap.dedent(renderer.PIPELINE.read_text().split(renderer.SCRIPT_MARKER, 1)[1])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = root / "commands"
            release, snapshot = fixtures()
            if partial_release:
                release["status"]["artifacts"]["images"] = release["status"]["artifacts"]["images"][:1]
            (root / "release.json").write_text(json.dumps(release))
            (root / "snapshot.json").write_text(json.dumps(snapshot))
            stubs = {
                "kubectl": 'case "$2" in releases.appstudio.redhat.com) cat "$FIXTURES/release.json";; '
                           'snapshots.appstudio.redhat.com) cat "$FIXTURES/snapshot.json";; *) exit 1;; esac\n',
                "git": 'exit "$GIT_STATUS"\n',
                "oras": 'case "$1" in resolve) for last; do :; done; '
                        '[ "$last" != "$UNAVAILABLE_IMAGE" ] || exit 1; case "$last" in *@*) '
                        'printf "%s\\n" "${last##*@}";; *) printf "{}\\n" | sha256sum | '
                        'cut -d " " -f 1 | sed "s/^/sha256:/";; esac;; '
                        'push) printf "{}\\n" > manifest.json;; *) exit 1;; esac\n',
                "select-oci-auth": "printf '%s' '{\"auths\":{\"quay.io\":{\"auth\":\"dGVzdDp0ZXN0\"}}}'\n",
                "python3": 'exec "$TEST_PYTHON" "$@"\n',
            }
            for name, body in stubs.items():
                executable = root / name
                executable.write_text('#!/bin/sh\nprintf "%s %s\\n" "${0##*/}" "$*" >> "$COMMAND_LOG"\n' + body)
                executable.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       COMMAND_LOG=str(commands), FIXTURES=str(root), TEST_PYTHON=sys.executable,
                       GIT_STATUS=str(git_status), UNAVAILABLE_IMAGE=unavailable_image, RELEASE=bundle.NAMESPACE + "/release-one",
                       SNAPSHOT=bundle.NAMESPACE + "/snapshot-one", BUNDLE_RESULT=str(root / "result"))
            result = subprocess.run(["bash", "-c", script], env=env, text=True, capture_output=True)
            reference = (root / "result").read_text() if (root / "result").exists() else None
            return result, commands.read_text(), reference

    def test_publishes_without_revision_parameter_or_provenance(self):
        result, commands, reference = self.run_bootstrap()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(reference.startswith(bundle.BUNDLE_REPOSITORY + "@sha256:"))
        self.assertIn("main:refs/remotes/origin/main", commands)
        self.assertNotIn("checkout", commands)
        self.assertNotIn("pipelineruns", commands)
        self.assertIn("python3 - --release", commands)

    def test_partial_release_checks_all_three_released_images(self):
        result, commands, reference = self.run_bootstrap(partial_release=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(reference.startswith(bundle.BUNDLE_REPOSITORY + "@sha256:"))
        for index, name in enumerate(bundle.COMPONENTS):
            self.assertIn("oras resolve " + bundle.RELEASE_PREFIX + name + "@sha256:" + str(index + 1) * 64, commands)

    def test_unavailable_omitted_image_stops_publication(self):
        image = bundle.RELEASE_PREFIX + bundle.COMPONENTS[1] + "@sha256:" + "2" * 64
        result, commands, reference = self.run_bootstrap(partial_release=True, unavailable_image=image)
        self.assertNotEqual(result.returncode, 0)
        self.assertIsNone(reference)
        self.assertIn("oras resolve " + image, commands)
        self.assertNotIn("oras push", commands)
        self.assertNotIn("select-oci-auth", commands)

    def test_failed_source_fetch_stops_publication(self):
        result, commands, reference = self.run_bootstrap(git_status=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertIsNone(reference)
        self.assertNotIn("python3", commands)
        self.assertNotIn("oras", commands)


if __name__ == "__main__":
    unittest.main()
