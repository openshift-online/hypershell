#!/usr/bin/env python3
"""Resolve public Quay image tags to manifest digests through the Quay v1 tag API.

The update-openshell agent sandboxes reach quay.io only through python3, with
read-only access to /api/v1/repository/ (see .fullsend/profiles/hypershell-quay-ro.yaml
and the agent-runtime-slim-quay SandboxTemplate). curl and skopeo cannot reach
quay.io there, so this script uses the standard library only.

Usage:
    python3 scripts/quay_digest.py <namespace/repo> <tag>          # print the tag's digest
    python3 scripts/quay_digest.py --list <namespace/repo>         # newest tags: "<name> <digest>"
    python3 scripts/quay_digest.py --tags-for <digest> <namespace/repo>  # tags that ever named a digest

Examples:
    python3 scripts/quay_digest.py opendatahub/odh-openshell-gateway v0.1.5-rhaiv.1
    python3 scripts/quay_digest.py gkrumbach07/openshell-dashboard 0.1.2
"""
import argparse
import json
import re
import sys
import urllib.parse
import urllib.request
from typing import Callable, Iterator

API = "https://quay.io/api/v1/repository"
REPO_RE = re.compile(r"^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._/-]*$")
DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
MAX_PAGES = 50

Fetch = Callable[[str], dict]


def fetch_json(url: str) -> dict:
    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def tag_url(repo: str, **params: object) -> str:
    return f"{API}/{repo}/tag/?{urllib.parse.urlencode(params)}"


def resolve(repo: str, tag: str, fetch: Fetch = fetch_json) -> str:
    """Return the manifest digest of an active tag, or raise LookupError."""
    data = fetch(tag_url(repo, specificTag=tag, onlyActiveTags="true"))
    for entry in data.get("tags", []):
        if entry.get("name") == tag:
            digest = entry.get("manifest_digest", "")
            if not DIGEST_RE.match(digest):
                raise LookupError(f"{repo}:{tag} has no valid manifest digest: {digest!r}")
            return digest
    raise LookupError(f"{repo}:{tag} not found (Quay tags often drop the leading 'v')")


def iter_tags(repo: str, fetch: Fetch = fetch_json, active_only: bool = False) -> Iterator[dict]:
    """Yield tag entries newest first, following has_additional pages."""
    for page in range(1, MAX_PAGES + 1):
        params: dict[str, object] = {"limit": 100, "page": page}
        if active_only:
            params["onlyActiveTags"] = "true"
        data = fetch(tag_url(repo, **params))
        yield from data.get("tags", [])
        if not data.get("has_additional"):
            return
    raise LookupError(f"{repo}: more than {MAX_PAGES} pages of tags; narrow the query")


def tags_for(repo: str, digest: str, fetch: Fetch = fetch_json) -> list[str]:
    """Return every tag name (including moved ones) that pointed at digest."""
    seen: list[str] = []
    for entry in iter_tags(repo, fetch):
        name = entry.get("name", "")
        if entry.get("manifest_digest") == digest and name not in seen:
            seen.append(name)
    return seen


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--list", action="store_true", help="list active tags, newest first")
    mode.add_argument("--tags-for", metavar="DIGEST", help="list tags that named DIGEST")
    parser.add_argument("repo", help="Quay repository, e.g. opendatahub/odh-openshell-gateway")
    parser.add_argument("tag", nargs="?", help="tag to resolve")
    args = parser.parse_args(argv)

    if not REPO_RE.match(args.repo):
        parser.error(f"invalid repository: {args.repo!r}")
    try:
        if args.list:
            for entry in iter_tags(args.repo, active_only=True):
                print(entry.get("name", ""), entry.get("manifest_digest", ""))
        elif args.tags_for:
            if not DIGEST_RE.match(args.tags_for):
                parser.error(f"invalid digest: {args.tags_for!r}")
            for name in tags_for(args.repo, args.tags_for):
                print(name)
        else:
            if not args.tag:
                parser.error("a tag is required unless --list or --tags-for is given")
            print(resolve(args.repo, args.tag))
    except (LookupError, OSError, ValueError) as err:
        print(f"quay_digest: {err}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
