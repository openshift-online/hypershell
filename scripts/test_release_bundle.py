"""Check release gates and the bundle contents without cluster access."""

import copy
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


if __name__ == "__main__":
    unittest.main()
