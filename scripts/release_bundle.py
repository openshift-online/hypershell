#!/usr/bin/env python3
"""Publish the image set from a successful Konflux managed release."""

from __future__ import annotations

import argparse
import base64
from datetime import datetime, timezone
import hashlib
import json
import os
import tarfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
import re
import subprocess
import tempfile
import time

APPLICATION = "hypershell-main"
NAMESPACE = "hcm-eng-prod-tenant"
SOURCE_URL = "https://github.com/openshift-online/hypershell"
BUILD_PREFIX = f"quay.io/redhat-user-workloads/{NAMESPACE}/{APPLICATION}/"
RELEASE_PREFIX = f"quay.io/redhat-services-prod/{NAMESPACE}/{APPLICATION}/"
COMPONENTS = (
    "hypershell-api-server-main",
    "hypershell-control-plane-main",
    "hypershell-web-console-main",
    "hypershell-cli-main",
)
# Components built by the hypershell-main Konflux application that ship on their
# own rather than through the release bundle. The fleet-dashboard is pinned by its
# own :main image digest (not the bundle), so it is tolerated in a Snapshot or the
# release artifacts and excluded here instead of rejected as unexpected.
EXCLUDED_COMPONENTS = ("hypershell-fleet-dashboard-main",)
BUNDLE_REPOSITORY = BUILD_PREFIX + COMPONENTS[0]
MANIFEST_PATHS = (
    "deploy/hub",
    "deploy/base/keycloak/theme",
    "deploy/gitops-base/components/openshift-dashboard-metrics",
)
MEDIA_TYPE = "application/vnd.hypershell.release.v1+json"
CLI_ASSETS = tuple("hsctl-" + target for target in (
    "linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64.exe"))
GITHUB_REPOSITORY = "openshift-online/hypershell"
_github_token = None


def installation_token():
    """Exchange a short-lived App JWT for a repository-scoped release token."""
    app_id = os.environ.get("GITHUB_APP_ID", "")
    installation_id = os.environ.get("GITHUB_INSTALLATION_ID", "")
    key_path = os.environ.get("GITHUB_PRIVATE_KEY_FILE", "")
    require(re.fullmatch(r"[0-9]+", app_id), "GITHUB_APP_ID must be a numeric App ID")
    require(re.fullmatch(r"[0-9]+", installation_id), "GITHUB_INSTALLATION_ID must be numeric")
    require(key_path and Path(key_path).is_file(), "GitHub App private key file is required")

    def encode(data):
        return base64.urlsafe_b64encode(data).rstrip(b"=")

    now = int(time.time())
    header = encode(json.dumps({"alg": "RS256", "typ": "JWT"}).encode())
    claims = encode(json.dumps({"iat": now - 60, "exp": now + 540, "iss": app_id}).encode())
    unsigned = header + b"." + claims
    signature = subprocess.check_output(
        ["openssl", "dgst", "-sha256", "-sign", key_path], input=unsigned)
    jwt = (unsigned + b"." + encode(signature)).decode()
    request = urllib.request.Request(
        "https://api.github.com/app/installations/" + installation_id + "/access_tokens",
        data=json.dumps({"repositories": [GITHUB_REPOSITORY.split("/")[1]],
                         "permissions": {"contents": "write"}}).encode(),
        method="POST", headers={
            "Authorization": "Bearer " + jwt,
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
            "User-Agent": "hypershell-release-bundle",
            "X-GitHub-Api-Version": "2022-11-28",
        })
    with urllib.request.urlopen(request, timeout=30) as response:
        result = json.load(response)
    require(isinstance(result.get("token"), str) and result["token"],
            "GitHub did not return an installation token")
    return result["token"]


def extract_cli(bundle, directory):
    """Read only regular release files from the digest-pinned CLI image."""
    image = next(c["image"] for c in bundle["components"] if c["name"] == "hypershell-cli-main")
    repository = image.split("@")[0]
    manifest = json.loads(run("oras", "manifest", "fetch", image))
    if "manifests" in manifest:
        candidates = [m for m in manifest["manifests"]
                      if m.get("platform", {}).get("os") == "linux"
                      and m.get("platform", {}).get("architecture") == "amd64"]
        require(len(candidates) == 1, "CLI index must contain one Linux amd64 image")
        manifest = json.loads(run("oras", "manifest", "fetch", repository + "@" + candidates[0]["digest"]))
    # The Dockerfile copies /releases in one dedicated layer. Require the whole
    # set in that layer, rather than combining files from older layers.
    for layer in reversed(manifest["layers"]):
        blob = directory / "layer.tar"
        run("oras", "blob", "fetch", "--output", str(blob), repository + "@" + layer["digest"])
        require("sha256:" + hashlib.sha256(blob.read_bytes()).hexdigest() == layer["digest"],
                "CLI layer digest mismatch")
        with tarfile.open(blob) as archive:
            members = [m for m in archive if m.name.removeprefix("./").startswith("releases/")
                       and not m.isdir()]
            if not members:
                continue
            require(sorted(m.name.removeprefix("./") for m in members) ==
                    sorted("releases/" + name for name in CLI_ASSETS), "Incomplete CLI release layer")
            for member in members:
                require(member.isfile() and 0 < member.size < 256 * 1024 * 1024,
                        "CLI asset must be a regular binary file")
                path = directory / member.name.rsplit("/", 1)[1]
                with archive.extractfile(member) as stream:
                    path.write_bytes(stream.read())
                path.chmod(0o755)
        blob.unlink()
        return [directory / name for name in CLI_ASSETS]
    raise ValueError("CLI image has no release binaries")


def github(method, path, data=None, *, binary=False):
    """Use fixed API hosts; credentials never enter command lines or logs."""
    token = _github_token
    require(token, "A GitHub App installation token is required to publish the bundle release")
    host = "https://uploads.github.com" if binary else "https://api.github.com"
    body = data if binary else json.dumps(data).encode() if data is not None else None
    request = urllib.request.Request(host + "/repos/" + GITHUB_REPOSITORY + path,
        data=body, method=method, headers={
            "Authorization": "Bearer " + token,
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/octet-stream" if binary else "application/json",
            "X-GitHub-Api-Version": "2022-11-28",
        })
    with urllib.request.urlopen(request, timeout=120) as response:
        content = response.read()
    return json.loads(content) if content else None


def github_optional(path):
    try:
        return github("GET", path)
    except urllib.error.HTTPError as error:
        if error.code != 404:
            raise
        return None


def release_notes(bundle, reference):
    lines = ["HyperShell release bundle", "", "Bundle: `" + reference + "`", "",
             "| Component | Commit tag | Immutable image |", "| --- | --- | --- |"]
    for component in bundle["components"]:
        lines.append("| " + component["name"] + " | `" + component["tags"][0] +
                     "` | `" + component["image"] + "` |")
    lines.extend(["", "The attached hsctl binaries were extracted from the CLI image above.",
                  "Verify downloads against SHA256SUMS. bundle.json records the complete image set.",
                  "An unchanged CLI build may appear in multiple bundles."])
    return "\n".join(lines) + "\n"


def publish_github(tag, bundle, reference, assets):
    """Resume drafts and validate completed releases without replacing assets."""
    revision = bundle["manifests"]["git"]["revision"]
    ref_path = "/git/ref/tags/" + tag
    ref = github_optional(ref_path)
    if ref is None:
        try:
            github("POST", "/git/refs", {"ref": "refs/tags/" + tag, "sha": revision})
        except urllib.error.HTTPError as error:
            if error.code != 422:
                raise
        ref = github("GET", ref_path)
    require(ref["object"]["type"] == "commit" and ref["object"]["sha"] == revision,
            "GitHub bundle tag points at a different commit")
    body = release_notes(bundle, reference)
    release = github_optional("/releases/tags/" + tag)
    # List also finds unfinished drafts when get-by-tag returns only published releases.
    if release is None:
        page = 1
        while True:
            batch = github("GET", f"/releases?per_page=100&page={page}")
            matches = [r for r in batch if r["tag_name"] == tag]
            require(len(matches) <= 1, "Duplicate GitHub releases for bundle")
            if matches:
                release = matches[0]
                break
            if len(batch) < 100:
                break
            page += 1
    if release is None:
        release = github("POST", "/releases", {"tag_name": tag, "target_commitish": revision,
            "name": tag, "body": body, "draft": True, "make_latest": "false"})
    require(release["body"] == body, "Existing GitHub release describes different bundle contents")
    release_path = "/releases/" + str(release["id"])
    existing = github("GET", release_path + "/assets?per_page=100")
    require(len(existing) <= len(assets), "Unexpected assets on bundle release")
    by_name = {a["name"]: a for a in existing}
    require(len(by_name) == len(existing) and set(by_name) <= {p.name for p in assets},
            "Unexpected or duplicate release asset")
    for path in assets:
        content = path.read_bytes()
        digest = "sha256:" + hashlib.sha256(content).hexdigest()
        current = by_name.get(path.name)
        if current and current.get("state") == "starter" and release["draft"]:
            github("DELETE", "/releases/assets/" + str(current["id"]))
            current = None
        if current:
            require(current.get("digest") == digest and current["size"] == len(content),
                    "Existing GitHub asset differs: " + path.name)
            continue
        require(release["draft"], "Published GitHub release is missing an asset")
        uploaded = github("POST", release_path + "/assets?name=" + urllib.parse.quote(path.name),
                          content, binary=True)
        require(uploaded.get("digest") == digest and uploaded["size"] == len(content),
                "GitHub upload digest mismatch: " + path.name)
    if release["draft"]:
        github("PATCH", release_path, {"draft": False, "make_latest": "false"})


def require(value, message):
    if not value:
        raise ValueError(message)


def indexed(items, *, require_all=True):
    items = [item for item in items if item["name"] not in EXCLUDED_COMPONENTS]
    result = {item["name"]: item for item in items}
    require(len(result) == len(items), "Duplicate component names")
    require(set(result) <= set(COMPONENTS), "Unexpected component names: " + ", ".join(sorted(set(result) - set(COMPONENTS))))
    if require_all:
        require(set(result) == set(COMPONENTS), "The snapshot must contain all four components")
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
                       "tags": [repository + ":" + source["revision"]],
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
        for tagged in component["tags"]:
            require(run("oras", "resolve", tagged) == component["image"].split("@")[1],
                    "Released tag does not match the Snapshot: " + tagged)
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
        assets = extract_cli(bundle, root)
        (root / "bundle.json").write_text(json.dumps(bundle, sort_keys=True) + "\n")
        (root / "config.json").write_text(json.dumps({
            "created": bundle["created"], "architecture": "amd64", "os": "linux",
            "config": {}, "rootfs": {"type": "layers", "diff_ids": []},
        }, sort_keys=True) + "\n")
        # Preserve a previously published bundle across retries and publisher upgrades.
        probe = subprocess.run(["oras", "resolve", "--registry-config", str(auth), target],
                               text=True, capture_output=True)
        if probe.returncode == 0:
            digest = probe.stdout.strip()
            existing = root / "existing"
            existing.mkdir()
            run("oras", "pull", "--registry-config", str(auth), "--output", str(existing),
                BUNDLE_REPOSITORY + "@" + digest)
            require((existing / "bundle.json").read_bytes() == (root / "bundle.json").read_bytes(),
                    "Existing bundle tag contains different content; refusing to overwrite")
        else:
            require(any(marker in probe.stderr.lower() for marker in ("not found", "manifest_unknown")),
                    "Unable to check existing bundle: " + probe.stderr)
            run("oras", "push", "--registry-config", str(auth), "--image-spec", "v1.0",
                "--annotation", "org.opencontainers.image.created=" + bundle["created"],
                "--config", "config.json:application/vnd.oci.image.config.v1+json",
                "--export-manifest", "manifest.json", target, "bundle.json:" + MEDIA_TYPE,
                cwd=root)
            digest = "sha256:" + hashlib.sha256((root / "manifest.json").read_bytes()).hexdigest()
        require(run("oras", "resolve", "--registry-config", str(auth), target) == digest,
                "Published bundle digest mismatch")
        reference = BUNDLE_REPOSITORY + "@" + digest
        assets.append(root / "bundle.json")
        checksums = root / "SHA256SUMS"
        checksums.write_text("".join(
            hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.name + "\n"
            for path in sorted(assets)))
        assets.append(checksums)
        publish_github(tag, bundle, reference, assets)
        Path(result_path).write_text(reference)
        print("Published " + target + "@" + digest)


def main():
    global _github_token
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", required=True)
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--source-directory", required=True)
    parser.add_argument("--result-path", required=True)
    args = parser.parse_args()
    _github_token = installation_token()
    publish(resource("releases.appstudio.redhat.com", args.release),
            resource("snapshots.appstudio.redhat.com", args.snapshot),
            args.source_directory, args.result_path)


if __name__ == "__main__":
    main()
