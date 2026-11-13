#!/usr/bin/env python3
"""Select private state for the live kind cluster without changing its kubeconfig."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

state_home = Path(sys.argv[1]).resolve()
override = os.environ.get("OCE_KIND_STATE")
requested = Path(override).resolve() if override else None
clusters = subprocess.check_output(["kind", "get", "clusters"], text=True).splitlines()
if "hypershell-oce" not in clusters:
    # Never replay credentials or receipts from a deleted cluster into its replacement.
    if requested and (not requested.exists() or not any(requested.iterdir())):
        state = requested
    else:
        state = Path(tempfile.mkdtemp(prefix="oce-hypershell-kind-", dir=state_home))
        if requested:
            print(f"Preserving previous state at {requested}; allocating fresh state for the new cluster.",
                  file=sys.stderr)
else:
    with tempfile.TemporaryDirectory(prefix=".oce-discovery-", dir=state_home) as directory:
        kubeconfig = Path(directory) / "kubeconfig"
        with kubeconfig.open("w") as output:
            subprocess.run(["kind", "get", "kubeconfig", "--name", "hypershell-oce"],
                           stdout=output, check=True)
        uid = subprocess.check_output([
            "kubectl", "--kubeconfig", str(kubeconfig), "--context", "kind-hypershell-oce",
            "get", "namespace", "kube-system", "-o", "jsonpath={.metadata.uid}",
            "--request-timeout=15s",
        ], text=True).strip()
    if not uid:
        raise SystemExit("Could not identify the running hypershell-oce cluster.")

    def matches(path):
        record = path / "cluster-uid"
        return (record.is_file() and record.read_text().strip() == uid
                and (path / "kubeconfig").is_file() and (path / "kubeconfig").stat().st_size > 0)

    current = state_home / "oce-hypershell-kind-current"
    if requested and matches(requested):
        state = requested
    elif current.is_symlink() and matches(current):
        state = current.resolve()
    else:
        candidates = {path.resolve() for path in state_home.glob("oce-hypershell-kind*")
                      if path.is_dir() and matches(path)}
        if len(candidates) != 1:
            raise SystemExit(
                ("The selected state is unrecorded or its cluster identity differs from the running cluster. "
                 if requested else "") +
                f"Found {len(candidates)} saved installations matching the running hypershell-oce cluster. "
                "Recover its original state or select it once with OCE_KIND_STATE; "
                "setup will not adopt an unrecorded cluster or guess between copies.")
        state = candidates.pop()
    if requested and state != requested:
        print(f"Previous OCE_KIND_STATE does not match this cluster; using its saved state at {state}.",
              file=sys.stderr)

print(state)
