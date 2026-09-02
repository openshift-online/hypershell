#!/usr/bin/env python3
"""Check image identity and event-specific component build rules."""

from pathlib import Path
import sys


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
COMPONENTS = {
    "api-server": {
        "dockerfile": "components/api-server/Dockerfile",
        "image": "hypershell-api-server-main",
    },
    "control-plane": {
        "dockerfile": "components/control-plane/Dockerfile",
        "image": "hypershell-control-plane-main",
    },
    "web-console": {
        "dockerfile": "components/web-console/Dockerfile",
        "image": "hypershell-web-console-main",
    },
}
NON_PUSH_RELEASE_FILES = ("VERSION", "CHANGELOG.md", "build-version.env")


def check_repository(root: Path) -> list[str]:
    """Return image build policy errors."""
    errors: list[str] = []
    for component, values in COMPONENTS.items():
        for event in ("push", "pull-request", "merge-queue"):
            relative_path = f".tekton/hypershell-{component}-main-{event}.yaml"
            text = (root / relative_path).read_text(encoding="utf-8")
            if "VCS_REF={{revision}}" not in text:
                errors.append(f"{relative_path} must pass the full revision")
            if "value: build-version.env" not in text:
                errors.append(f"{relative_path} must use build-version.env")
            if event == "push":
                if '"VERSION".pathChanged()' not in text:
                    errors.append(f"{relative_path} must build for a VERSION change")
                expected_tag = f"{values['image']}:{{{{revision}}}}"
            else:
                for release_file in NON_PUSH_RELEASE_FILES:
                    if f'"{release_file}".pathChanged()' in text:
                        errors.append(
                            f"{relative_path} must not build for {release_file}"
                        )
                tag_prefix = (
                    "on-pr-" if event == "pull-request" else "on-merge-queue-"
                )
                expected_tag = f"{values['image']}:{tag_prefix}{{{{revision}}}}"
            if expected_tag not in text:
                errors.append(f"{relative_path} must keep its current revision tag")

        dockerfile_path = root / values["dockerfile"]
        dockerfile = dockerfile_path.read_text(encoding="utf-8")
        if 'org.opencontainers.image.version="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}"' not in dockerfile:
            errors.append(f"{values['dockerfile']} must set the OCI build version")
        if 'org.opencontainers.image.revision="${VCS_REF}"' not in dockerfile:
            errors.append(f"{values['dockerfile']} must set the OCI revision")
        if 'HYPERSHELL_BUILD_VERSION="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}"' not in dockerfile:
            errors.append(f"{values['dockerfile']} must set the runtime build version")

    e2e = (root / ".github/workflows/e2e.yml").read_text(encoding="utf-8")
    if "release_version_changed=false" not in e2e:
        errors.append("the E2E plan must identify a release-version push")
    if '[[ "${EVENT_NAME}" == "push" ]]' not in e2e:
        errors.append("the release image rule must check for a push event")
    if 'grep -qx "VERSION"' not in e2e:
        errors.append("the release image rule must check the VERSION file")
    return errors


def main() -> int:
    errors = check_repository(REPOSITORY_ROOT)
    for error in errors:
        print(f"image build policy error: {error}", file=sys.stderr)
    if errors:
        return 1
    print("Image build policy is valid.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
