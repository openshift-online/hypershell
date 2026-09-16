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


class ManifestSourceTests(unittest.TestCase):
    def setUp(self):
        # Commit hooks export Git paths. Keep fixture operations in the fixture.
        environment = patch.dict(os.environ, {
            key: value for key, value in os.environ.items() if not key.startswith("GIT_")
        }, clear=True)
        environment.start()
        self.addCleanup(environment.stop)
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.git("init", "--quiet", "--initial-branch=main")
        self.git("config", "core.hooksPath", str(self.root / "empty-hooks"))
        self.git("config", "commit.gpgsign", "false")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.com")
        for path in bundle.MANIFEST_PATHS:
            target = self.root / path / "kustomization.yaml"
            target.parent.mkdir(parents=True)
            target.write_text("resources: []\n")
        self.old = self.commit("initial")
        self.middle = self.commit("middle")
        self.new = self.commit("new")
        self.git("update-ref", "refs/remotes/origin/main", self.new)

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True).strip()

    def commit(self, message):
        self.git("add", ".")
        self.git("commit", "--quiet", "--allow-empty", "-m", message)
        return self.git("rev-parse", "HEAD")

    def source(self, revisions):
        components = [{"source": {"git": {"revision": revision}}} for revision in revisions]
        return bundle.manifest_source(components, self.root)

    def test_selects_newest_snapshot_commit_not_current_main(self):
        expected = {"git": {"url": bundle.SOURCE_URL, "revision": self.middle}}
        self.assertEqual(self.source([self.old, self.middle, self.old]), expected)
        self.assertEqual(self.source([self.middle, self.old, self.old]), expected)
        later = self.commit("main advances while a release waits")
        self.git("update-ref", "refs/remotes/origin/main", later)
        self.assertEqual(self.source([self.old, self.middle, self.old]), expected)

    def test_manifest_only_commit_can_advance_with_older_other_images(self):
        (self.root / bundle.MANIFEST_PATHS[0] / "kustomization.yaml").write_text("resources: []\n# changed\n")
        manifest_change = self.commit("manifest change")
        self.git("update-ref", "refs/remotes/origin/main", manifest_change)
        self.assertEqual(self.source([manifest_change, self.old, self.middle])["git"]["revision"], manifest_change)

    def test_missing_manifest_base_stops_selection(self):
        (self.root / bundle.MANIFEST_PATHS[1] / "kustomization.yaml").unlink()
        missing = self.commit("remove required manifest base")
        self.git("update-ref", "refs/remotes/origin/main", missing)
        with self.assertRaises(subprocess.CalledProcessError):
            self.source([self.old, missing])

    def test_unmerged_revision_stops_selection(self):
        self.git("checkout", "--quiet", "-b", "unmerged", self.old)
        unmerged = self.commit("unmerged")
        with self.assertRaises(subprocess.CalledProcessError):
            self.source([self.old, unmerged])

    def test_divergent_component_revisions_stop_selection(self):
        self.git("checkout", "--quiet", "-b", "side", self.old)
        side = self.commit("side")
        self.git("checkout", "--quiet", "main")
        self.git("merge", "--quiet", "--no-ff", "side", "-m", "merge side")
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        with self.assertRaises(subprocess.CalledProcessError):
            self.source([self.new, side])


class PipelineBootstrapTests(unittest.TestCase):
    def test_embedded_publisher_matches_source(self):
        pipeline = renderer.PIPELINE.read_text()
        self.assertEqual(pipeline, renderer.render(pipeline, renderer.PUBLISHER.read_text()))

    def run_bootstrap(self, git_status=0, partial_release=False, unavailable_image="", missing_manifest=False):
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
                "git": 'if [ "$1" = "cat-file" ] && [ "$MISSING_MANIFEST" = "1" ]; then exit 1; fi; exit "$GIT_STATUS"\n',
                "oras": 'case "$1" in resolve) for last; do :; done; '
                        '[ "$last" != "$UNAVAILABLE_IMAGE" ] || exit 1; case "$last" in *@*) '
                        'printf "%s\\n" "${last##*@}";; *) printf "{}\\n" | sha256sum | '
                        'cut -d " " -f 1 | sed "s/^/sha256:/";; esac;; '
                        'push) cp bundle.json "$FIXTURES/published.json"; printf "{}\\n" > manifest.json;; *) exit 1;; esac\n',
                "select-oci-auth": "printf '%s' '{\"auths\":{\"quay.io\":{\"auth\":\"dGVzdDp0ZXN0\"}}}'\n",
                "python3": 'exec "$TEST_PYTHON" "$@"\n',
            }
            for name, body in stubs.items():
                executable = root / name
                executable.write_text('#!/bin/sh\nprintf "%s %s\\n" "${0##*/}" "$*" >> "$COMMAND_LOG"\n' + body)
                executable.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       COMMAND_LOG=str(commands), FIXTURES=str(root), TEST_PYTHON=sys.executable,
                       GIT_STATUS=str(git_status), MISSING_MANIFEST="1" if missing_manifest else "0",
                       UNAVAILABLE_IMAGE=unavailable_image, RELEASE=bundle.NAMESPACE + "/release-one",
                       SNAPSHOT=bundle.NAMESPACE + "/snapshot-one", BUNDLE_RESULT=str(root / "result"))
            result = subprocess.run(["bash", "-c", script], env=env, text=True, capture_output=True)
            if result.returncode == 0:
                published = json.loads((root / "published.json").read_text())
                self.assertEqual(published["manifests"], {"git": {"url": bundle.SOURCE_URL, "revision": "3" * 40}})
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

    def test_missing_manifest_stops_publication_before_registry_access(self):
        result, commands, reference = self.run_bootstrap(missing_manifest=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIsNone(reference)
        self.assertIn("git cat-file -e", commands)
        self.assertNotIn("oras", commands)
        self.assertNotIn("select-oci-auth", commands)

    def test_failed_source_fetch_stops_publication(self):
        result, commands, reference = self.run_bootstrap(git_status=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertIsNone(reference)
        self.assertNotIn("python3", commands)
        self.assertNotIn("oras", commands)


if __name__ == "__main__":
    unittest.main()
