#!/usr/bin/env python3
"""Publish the image set from a successful Konflux managed release."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

APPLICATION = "hypershell-main"
NAMESPACE = "hcm-eng-prod-tenant"
SOURCE_URL = "https://github.com/openshift-online/hypershell"
BUILD_PREFIX = f"quay.io/redhat-user-workloads/{NAMESPACE}/{APPLICATION}/"
RELEASE_PREFIX = f"quay.io/redhat-services-prod/{NAMESPACE}/{APPLICATION}/"
COMPONENTS = (
    "hypershell-api-server-main",
    "hypershell-control-plane-main",
    "hypershell-web-console-main",
)
BUNDLE_REPOSITORY = BUILD_PREFIX + COMPONENTS[0]
MANIFEST_PATHS = (
    "deploy/hub",
    "deploy/base/keycloak/theme",
    "deploy/gitops-base/components/openshift-dashboard-metrics",
)
MEDIA_TYPE = "application/vnd.hypershell.release.v1+json"


def require(value, message):
    if not value:
        raise ValueError(message)


def indexed(items, *, require_all=True):
    result = {item["name"]: item for item in items}
    require(len(result) == len(items), "Duplicate component names")
    require(set(result) <= set(COMPONENTS), "Unexpected component names: " + ", ".join(sorted(set(result) - set(COMPONENTS))))
    if require_all:
        require(set(result) == set(COMPONENTS), "The snapshot must contain all three components")
    else:
        require(result, "The release has no image artifacts")
    return result


def make_bundle(release, snapshot):
    """Reject incomplete or unsuccessful releases before registry operations."""
    metadata = release["metadata"]
    require(metadata["namespace"] == NAMESPACE, "Unexpected release namespace")
    require(snapshot["metadata"]["namespace"] == NAMESPACE, "Unexpected snapshot namespace")
    require(release["spec"]["releasePlan"] == "hypershell-releaseplan", "Unexpected release plan")
    require(release["spec"]["snapshot"] == snapshot["metadata"]["name"], "Snapshot mismatch")
    require(snapshot["spec"]["application"] == APPLICATION, "Unexpected application")
    conditions = {c["type"]: c for c in release.get("status", {}).get("conditions", [])}
    managed = conditions.get("ManagedPipelineProcessed", {})
    require(managed.get("status") == "True" and managed.get("reason") == "Succeeded",
            "The managed release did not succeed")
    for obj in (release, snapshot):
        push_prefixes, pull_request_prefixes = set(), set()
        for field in ("annotations", "labels"):
            for key, value in obj["metadata"].get(field, {}).items():
                if key.endswith("/event-type"):
                    require(value == "push", "Only push snapshots can produce bundles")
                    push_prefixes.add(key.removesuffix("/event-type"))
                if key.endswith("/pull-request"):
                    pull_request_prefixes.add(key.removesuffix("/pull-request"))
        # Push events can retain the number of the merged pull request.
        require(pull_request_prefixes <= push_prefixes,
                "Pull request metadata requires an explicit push event")
    components = indexed(snapshot["spec"]["components"])
    images = indexed(release["status"].get("artifacts", {}).get("images", []), require_all=False)
    output = []
    for name in COMPONENTS:
        component = components[name]
        build_repository = BUILD_PREFIX + name + "@"
        require(component["containerImage"].startswith(build_repository), "Unexpected snapshot image repository")
        digest = component["containerImage"].removeprefix(build_repository)
        require(re.fullmatch(r"sha256:[0-9a-f]{64}", digest), "Invalid image digest")
        repository = RELEASE_PREFIX + name
        # The managed pipeline can omit components from its artifact list.
        # publish() checks every snapshot digest in the release registry,
        # including images that are not listed in this release.
        if name in images:
            image = images[name]
            require(image["shasum"] == digest, "Released digest does not match the snapshot")
            require(any(url.startswith(repository + ":") for url in image["urls"]),
                    "The release has no expected destination repository")
        source = component["source"]["git"]
        require(source["url"].removesuffix(".git") == SOURCE_URL, "Unexpected source repository")
        require(re.fullmatch(r"[0-9a-f]{40}", source["revision"]), "Invalid source revision")
        output.append({"name": name, "image": repository + "@" + digest,
                       "source": {"git": {"url": SOURCE_URL, "revision": source["revision"]}}})
    created = snapshot["metadata"]["creationTimestamp"]
    stamp = datetime.fromisoformat(created.replace("Z", "+00:00"))
    require(stamp.utcoffset() is not None, "Snapshot time must include a timezone")
    stamp = stamp.astimezone(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    identity = hashlib.sha256(metadata["uid"].encode()).hexdigest()[:16]
    tag = f"release-bundle-{stamp}-{identity}"
    return tag, {
        "schemaVersion": 1,
        "application": APPLICATION,
        "created": created,
        "release": {"namespace": NAMESPACE, "name": metadata["name"], "uid": metadata["uid"]},
        "snapshot": {"name": snapshot["metadata"]["name"], "uid": snapshot["metadata"]["uid"]},
        "components": output,
    }


def run(*args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs).strip()


def resource(kind, reference):
    namespace, name = reference.split("/")
    require(namespace == NAMESPACE, "Unexpected resource namespace")
    require(re.fullmatch(r"[a-z0-9][a-z0-9.-]*", name), "Invalid resource name")
    return json.loads(run("kubectl", "get", kind, name, "-n", namespace, "-o", "json"))


def manifest_source(components, source_directory):
    """Select a fixed Snapshot commit that contains all component revisions."""
    revisions = sorted({component["source"]["git"]["revision"] for component in components})
    for revision in revisions:
        run("git", "merge-base", "--is-ancestor", revision,
            "refs/remotes/origin/main", cwd=source_directory)
    selected = None
    for candidate in revisions:
        for revision in revisions:
            try:
                run("git", "merge-base", "--is-ancestor", revision, candidate,
                    cwd=source_directory)
            except subprocess.CalledProcessError as error:
                if error.returncode != 1:
                    raise
                # Try another Snapshot commit if this one lacks a component.
                break
        else:
            selected = candidate
            break
    require(selected is not None,
            "No Snapshot component commit contains all component revisions")
    for path in MANIFEST_PATHS:
        run("git", "cat-file", "-e", selected + ":" + path + "/kustomization.yaml",
            cwd=source_directory)
    return {"git": {"url": SOURCE_URL, "revision": selected}}


def publish(release, snapshot, source_directory, result_path):
    tag, bundle = make_bundle(release, snapshot)
    bundle["manifests"] = manifest_source(bundle["components"], source_directory)
    for component in bundle["components"]:
        require(run("oras", "resolve", component["image"]) == component["image"].split("@")[1],
                "Released image is not available by digest: " + component["image"])
    # Konflux credentials can be scoped to a repository. ORAS needs a host entry.
    credentials = json.loads(run("select-oci-auth", BUNDLE_REPOSITORY))
    require(credentials.get("auths", {}).get("quay.io"),
            "The build service account has no registry credentials")
    target = BUNDLE_REPOSITORY + ":" + tag
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        auth = root / "auth.json"
        auth.touch(mode=0o600)
        auth.write_text(json.dumps(credentials))
        (root / "bundle.json").write_text(json.dumps(bundle, sort_keys=True) + "\n")
        (root / "config.json").write_text(json.dumps({
            "created": bundle["created"], "architecture": "amd64", "os": "linux",
            "config": {}, "rootfs": {"type": "layers", "diff_ids": []},
        }, sort_keys=True) + "\n")
        # A fixed creation time makes retries of the same Release reproducible.
        run("oras", "push", "--registry-config", str(auth), "--image-spec", "v1.0",
            "--annotation", "org.opencontainers.image.created=" + bundle["created"],
            "--config", "config.json:application/vnd.oci.image.config.v1+json",
            "--export-manifest", "manifest.json", target, "bundle.json:" + MEDIA_TYPE,
            cwd=root)
        digest = "sha256:" + hashlib.sha256((root / "manifest.json").read_bytes()).hexdigest()
        require(run("oras", "resolve", "--registry-config", str(auth), target) == digest,
                "Published bundle digest mismatch")
        Path(result_path).write_text(BUNDLE_REPOSITORY + "@" + digest)
        print("Published " + target + "@" + digest)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", required=True)
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--source-directory", required=True)
    parser.add_argument("--result-path", required=True)
    args = parser.parse_args()
    publish(resource("releases.appstudio.redhat.com", args.release),
            resource("snapshots.appstudio.redhat.com", args.snapshot),
            args.source_directory, args.result_path)


if __name__ == "__main__":
    main()
