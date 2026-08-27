#!/usr/bin/env python3
"""Rewrite kustomize-rendered YAML for an ephemeral OpenShift namespace group.

Maps the overlay's platform namespace (hypershell-system) to OPENSHIFT_NAMESPACE
and the bundled Keycloak namespace (keycloak) to ${OPENSHIFT_NAMESPACE}-keycloak.
ClusterRoleBindings are renamed so two environments on one cluster do not share
a binding name. ClusterRoles stay shared.
"""
from __future__ import annotations

import argparse
import re
import sys

PLATFORM_NS = "hypershell-system"
KEYCLOAK_NS = "keycloak"


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


def prefix_cluster_role_binding_name(doc: str, prefix: str) -> str:
    """Prefix metadata.name only; leave roleRef.name unchanged."""
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


def rewrite_doc(doc: str, platform_ns: str, keycloak_ns: str) -> str:
    kind = kind_of(doc)
    rewritten = doc.replace(PLATFORM_NS, platform_ns)
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
    if kind == "ClusterRoleBinding":
        rewritten = prefix_cluster_role_binding_name(rewritten, f"{platform_ns}-")
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


def keep_doc(
    doc: str,
    *,
    omit_namespaces: bool,
    only_namespace: str | None,
    include_cluster_scoped: bool,
    omit_kinds: set[str] | None = None,
    omit_names: set[str] | None = None,
) -> bool:
    if not doc.strip():
        return False
    kind = kind_of(doc)
    if omit_namespaces and kind == "Namespace":
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
) -> str:
    docs = [rewrite_doc(doc, platform_ns, keycloak_ns) for doc in split_docs(text)]
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
        "--omit-names",
        default="",
        help="Comma-separated metadata.names to drop (e.g. hypershell-sandbox-scc).",
    )
    parser.add_argument(
        "--strip-openshift-uids",
        action="store_true",
        help="Remove runAsUser/runAsGroup/fsGroup so restricted SCC can assign identities.",
    )
    args = parser.parse_args()
    omit_kinds = {k for k in args.omit_kinds.split(",") if k}
    omit_names = {n for n in args.omit_names.split(",") if n}
    rendered = rewrite(
        sys.stdin.read(),
        args.platform_namespace,
        args.keycloak_namespace,
        omit_namespaces=args.omit_namespaces,
        only_namespace=args.only_namespace,
        include_cluster_scoped=args.include_cluster_scoped,
        omit_kinds=omit_kinds or None,
        omit_names=omit_names or None,
    )
    if args.strip_openshift_uids:
        rendered = strip_openshift_fixed_uids(rendered)
    sys.stdout.write(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
