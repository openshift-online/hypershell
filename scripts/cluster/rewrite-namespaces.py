#!/usr/bin/env python3
"""Rewrite kustomize-rendered YAML for an ephemeral OpenShift namespace group.

Maps the overlay's platform namespace (hypershell-system) to OPENSHIFT_NAMESPACE
and the bundled Keycloak namespace (keycloak) to ${OPENSHIFT_NAMESPACE}-keycloak.

ClusterRole and ClusterRoleBinding names (and ClusterRoleBinding roleRefs to
those ClusterRoles) are prefixed with ${platform}-dev- so each openshift-up
environment has its own copy and does not patch stage's hypershell-controller.
Built-in ClusterRoles (system:*) are not renamed. --keep-role-refs leaves
roleRef pointing at the existing cluster-wide ClusterRole (workaround for
OPENSHIFT_USE_EXISTING_CLUSTERROLE).
"""
from __future__ import annotations

import argparse
import re
import sys

PLATFORM_NS = "hypershell-system"
KEYCLOAK_NS = "keycloak"
DEV_SCOPE = "dev"
CLUSTER_SCOPED_KINDS = frozenset({"ClusterRole", "ClusterRoleBinding"})
# Rewrite the platform namespace as a DNS label, not a substring of a longer
# name (image repos, args). Still matches namespace fields, label selectors,
# env values, and in-cluster DNS like hypershell-api-server.hypershell-system.svc.
PLATFORM_NS_TOKEN = re.compile(
    r"(?<![A-Za-z0-9-])" + re.escape(PLATFORM_NS) + r"(?![A-Za-z0-9-])"
)


def cluster_scoped_prefix(platform_ns: str) -> str:
    return f"{platform_ns}-{DEV_SCOPE}-"


def split_docs(text: str) -> list[str]:
    if text.startswith("---"):
        body = text[3:]
        prefix_sep = True
    else:
        body = text
        prefix_sep = False
    docs = re.split(r"\n---\n", body)
    if prefix_sep and docs and docs[0].strip() == "":
        docs = docs[1:]
    return docs


def kind_of(doc: str) -> str:
    match = re.search(r"^kind:\s*(\S+)\s*$", doc, re.M)
    return match.group(1) if match else ""


def prefix_metadata_name(doc: str, prefix: str) -> str:
    """Prefix metadata.name only; do not touch roleRef or subjects."""
    parts = re.split(r"^(roleRef:|subjects:)", doc, maxsplit=1, flags=re.M)
    head = parts[0]
    rest = "".join(parts[1:]) if len(parts) > 1 else ""

    def repl(match: re.Match[str]) -> str:
        name = match.group(2)
        if name.startswith(prefix):
            return match.group(0)
        return f"{match.group(1)}{prefix}{name}"

    # Do not use \\s*$: $ is end-of-line, but \\s matches newlines and would
    # swallow the newline before roleRef, gluing `name: foo` onto `roleRef:`.
    head = re.sub(r"^(  name:\s*)(\S+)[^\S\n]*$", repl, head, count=1, flags=re.M)
    return head + rest


def prefix_role_ref(doc: str, prefix: str) -> str:
    """Prefix roleRef.name unless it is a built-in system: ClusterRole."""
    parts = re.split(r"^(roleRef:)", doc, maxsplit=1, flags=re.M)
    if len(parts) < 3:
        return doc
    head, marker, rest = parts[0], parts[1], parts[2]

    def repl(match: re.Match[str]) -> str:
        name = match.group(2)
        if name.startswith("system:") or name.startswith(prefix):
            return match.group(0)
        return f"{match.group(1)}{prefix}{name}"

    rest = re.sub(r"^(  name:\s*)(\S+)[^\S\n]*$", repl, rest, count=1, flags=re.M)
    return head + marker + rest


def rewrite_doc(
    doc: str,
    platform_ns: str,
    keycloak_ns: str,
    keep_role_refs: bool = False,
) -> str:
    kind = kind_of(doc)
    rewritten = PLATFORM_NS_TOKEN.sub(platform_ns, doc)
    rewritten = re.sub(
        r"^(\s*namespace:\s*)" + re.escape(KEYCLOAK_NS) + r"\s*$",
        rf"\g<1>{keycloak_ns}",
        rewritten,
        flags=re.M,
    )
    if kind == "Namespace":
        rewritten = re.sub(
            r"^(  name:\s*)" + re.escape(KEYCLOAK_NS) + r"\s*$",
            rf"\g<1>{keycloak_ns}",
            rewritten,
            flags=re.M,
        )
    prefix = cluster_scoped_prefix(platform_ns)
    if kind in CLUSTER_SCOPED_KINDS:
        rewritten = prefix_metadata_name(rewritten, prefix)
    if kind == "ClusterRoleBinding" and not keep_role_refs:
        rewritten = prefix_role_ref(rewritten, prefix)
    return rewritten


def resource_namespace(doc: str) -> str | None:
    """Return metadata.namespace, not subject or pod-template namespaces."""
    match = re.search(r"^metadata:\n((?:  .*\n?)*)", doc, re.M)
    if not match:
        return None
    ns = re.search(r"^  namespace:\s*(\S+)\s*$", match.group(1), re.M)
    return ns.group(1) if ns else None


def metadata_name(doc: str) -> str | None:
    match = re.search(r"^metadata:\n((?:  .*\n?)*)", doc, re.M)
    if not match:
        return None
    name = re.search(r"^  name:\s*(\S+)\s*$", match.group(1), re.M)
    return name.group(1) if name else None


def unprefixed_cluster_scoped(text: str, prefix: str) -> list[str]:
    """Return ClusterRole/ClusterRoleBinding ids whose metadata.name lacks prefix."""
    bad: list[str] = []
    for doc in split_docs(text):
        kind = kind_of(doc)
        if kind not in CLUSTER_SCOPED_KINDS:
            continue
        name = metadata_name(doc) or ""
        if not name.startswith(prefix):
            bad.append(f"{kind}/{name or '(missing name)'}")
    return bad


def keep_doc(
    doc: str,
    *,
    omit_namespaces: bool,
    only_namespace: str | None,
    include_cluster_scoped: bool,
    omit_kinds: set[str] | None = None,
    omit_names: set[str] | None = None,
    only_kinds: set[str] | None = None,
) -> bool:
    if not doc.strip():
        return False
    kind = kind_of(doc)
    if omit_namespaces and kind == "Namespace":
        return False
    if only_kinds and kind not in only_kinds:
        return False
    if omit_kinds and kind in omit_kinds:
        return False
    if omit_names:
        name = metadata_name(doc)
        if name and name in omit_names:
            return False
    if only_namespace is None:
        return True
    ns = resource_namespace(doc)
    if ns is None:
        return include_cluster_scoped
    return ns == only_namespace


def rewrite(
    text: str,
    platform_ns: str,
    keycloak_ns: str,
    omit_namespaces: bool = False,
    only_namespace: str | None = None,
    include_cluster_scoped: bool = False,
    omit_kinds: set[str] | None = None,
    omit_names: set[str] | None = None,
    only_kinds: set[str] | None = None,
    keep_role_refs: bool = False,
) -> str:
    docs = [
        rewrite_doc(doc, platform_ns, keycloak_ns, keep_role_refs=keep_role_refs)
        for doc in split_docs(text)
    ]
    docs = [
        doc
        for doc in docs
        if keep_doc(
            doc,
            omit_namespaces=omit_namespaces,
            only_namespace=only_namespace,
            include_cluster_scoped=include_cluster_scoped,
            omit_kinds=omit_kinds,
            omit_names=omit_names,
            only_kinds=only_kinds,
        )
    ]
    rendered = "\n---\n".join(docs)
    if rendered and not rendered.endswith("\n"):
        rendered += "\n"
    return rendered


OPENSHIFT_UID_FIELDS_RE = re.compile(
    r"^[ \t]+(?:runAsUser|runAsGroup|fsGroup|fsGroupChangePolicy):[^\n]*\n",
    re.M,
)


def strip_openshift_fixed_uids(text: str) -> str:
    """Drop pinned UIDs/GIDs so OpenShift restricted SCC can assign the range."""
    return OPENSHIFT_UID_FIELDS_RE.sub("", text)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform-namespace", required=True)
    parser.add_argument("--keycloak-namespace", required=True)
    parser.add_argument(
        "--omit-namespaces",
        action="store_true",
        help="Drop Namespace documents. Projects are created with oc new-project.",
    )
    parser.add_argument(
        "--only-namespace",
        help="Keep only documents whose metadata.namespace matches this value.",
    )
    parser.add_argument(
        "--include-cluster-scoped",
        action="store_true",
        help="When --only-namespace is set, also keep documents with no namespace (ClusterRole, ClusterRoleBinding).",
    )
    parser.add_argument(
        "--omit-kinds",
        default="",
        help="Comma-separated kinds to drop (e.g. ClusterRole,ClusterRoleBinding,Cluster).",
    )
    parser.add_argument(
        "--only-kinds",
        default="",
        help="Comma-separated kinds to keep (e.g. ClusterRole,ClusterRoleBinding).",
    )
    parser.add_argument(
        "--omit-names",
        default="",
        help="Comma-separated metadata.names to drop (e.g. hypershell-sandbox-scc).",
    )
    parser.add_argument(
        "--keep-role-refs",
        action="store_true",
        help="Prefix ClusterRoleBinding names but leave roleRef pointing at the existing ClusterRole (hypershell-controller).",
    )
    parser.add_argument(
        "--strip-openshift-uids",
        action="store_true",
        help="Remove runAsUser/runAsGroup/fsGroup so restricted SCC can assign identities.",
    )
    args = parser.parse_args()
    omit_kinds = {k for k in args.omit_kinds.split(",") if k}
    omit_names = {n for n in args.omit_names.split(",") if n}
    only_kinds = {k for k in args.only_kinds.split(",") if k}
    rendered = rewrite(
        sys.stdin.read(),
        args.platform_namespace,
        args.keycloak_namespace,
        omit_namespaces=args.omit_namespaces,
        only_namespace=args.only_namespace,
        include_cluster_scoped=args.include_cluster_scoped,
        omit_kinds=omit_kinds or None,
        omit_names=omit_names or None,
        only_kinds=only_kinds or None,
        keep_role_refs=args.keep_role_refs,
    )
    if args.strip_openshift_uids:
        rendered = strip_openshift_fixed_uids(rendered)
    sys.stdout.write(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
