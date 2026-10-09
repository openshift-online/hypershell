#!/usr/bin/env python3
"""Build, capture, attest, and validate the Storybook parity gate."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import threading
from datetime import datetime, timezone
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse
from urllib.request import urlopen


ROOT = Path(__file__).resolve().parents[4]
WEB_CONSOLE = ROOT / "components/web-console"
VIEWPORTS = {"desktop": (1440, 900), "narrow": (390, 844)}
SOURCE_ROOTS = (
    ROOT / "components/web-console/app",
    ROOT / "packages/gateway-management-ui/src",
    ROOT / "packages/operational-dashboard-ui/src",
    ROOT / "specs/web-console",
)
SOURCE_SUFFIXES = {".css", ".json", ".md", ".ts", ".tsx"}
PRODUCTION_ROOTS = (
    ROOT / "components/web-console/app",
    ROOT / "packages/gateway-management-ui/src",
    ROOT / "packages/operational-dashboard-ui/src",
)
PARITY_SHELL_COMPONENTS = (
    "MockupShell",
    "MockupTemplate",
    "Masthead",
    "MastheadBrand",
    "MastheadContent",
    "MastheadMain",
)


def run(*args: str, cwd: Path = ROOT) -> None:
    subprocess.run(args, cwd=cwd, check=True)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def source_fingerprint(excluded_root: Path | None = None) -> str:
    digest = hashlib.sha256()
    for root in SOURCE_ROOTS:
        if not root.exists():
            continue
        for path in sorted(
            item
            for item in root.rglob("*")
            if item.is_file()
            and item.suffix in SOURCE_SUFFIXES
            and (
                excluded_root is None
                or not item.resolve().is_relative_to(excluded_root)
            )
            and "storybook-static" not in item.parts
            and "node_modules" not in item.parts
        ):
            digest.update(str(path.relative_to(ROOT)).encode())
            digest.update(path.read_bytes())
    return digest.hexdigest()


def start_server(directory: Path) -> tuple[ThreadingHTTPServer, threading.Thread]:
    handler = partial(SimpleHTTPRequestHandler, directory=str(directory))
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, thread


def story_url(port: int, story_id: str) -> str:
    return f"http://127.0.0.1:{port}/iframe.html?id={story_id}&viewMode=story"


def require_story(port: int, story_id: str, side: str) -> None:
    with urlopen(f"http://127.0.0.1:{port}/index.json", timeout=5) as response:
        index = json.load(response)
    entries = index.get("entries", {})
    entry = entries.get(story_id) if isinstance(entries, dict) else None
    if not isinstance(entry, dict) or entry.get("type") != "story":
        raise RuntimeError(f"{side} Storybook has no story id: {story_id}")


def require_static_network(har_path: Path, side: str) -> None:
    har = json.loads(har_path.read_text())
    entries = har.get("log", {}).get("entries", [])
    for entry in entries:
        url = entry.get("request", {}).get("url", "")
        parsed = urlparse(url)
        static_path = (
            parsed.path == "/iframe.html"
            or parsed.path == "/index.json"
            or parsed.path.startswith("/assets/")
            or parsed.path.startswith("/vite-inject-")
        )
        if parsed.hostname not in {"127.0.0.1", "localhost"} or not static_path:
            raise RuntimeError(
                f"{side} story made a non-Storybook request: {url}. "
                "Add deterministic fixtures or mock the runtime adapter"
            )


def require_shell_free_mockup(path_text: str) -> None:
    path = (ROOT / path_text).resolve()
    if not path.is_file():
        raise RuntimeError(f"Mockup surface file does not exist: {path_text}")
    text = path.read_text()
    found = [name for name in PARITY_SHELL_COMPONENTS if name in text]
    if found:
        raise RuntimeError(
            "Mockup parity surface contains application-shell components: "
            f"{', '.join(found)}. Move page-owned UI into <Name>PageSurface and "
            "keep shell chrome only in the Mockups/... story"
        )


def require_page_surface_regions(
    path_text: str,
    side: str,
    header_variant: str,
    body_variant: str,
    body_filled: bool,
) -> None:
    path = (ROOT / path_text).resolve()
    if not path.is_file():
        raise RuntimeError(f"{side} page surface does not exist: {path_text}")
    text = path.read_text()
    contracts = (
        ("header", header_variant, False),
        ("body", body_variant, body_filled),
    )
    for region, variant, requires_fill in contracts:
        section = re.search(
            rf"<PageSection\b(?=[^>]*\bdata-page-region=[\"']{region}[\"'])[^>]*>",
            text,
            re.DOTALL,
        )
        if section is None:
            raise RuntimeError(
                f'{side} page surface must own <PageSection data-page-region="{region}">: '
                f"{path_text}"
            )
        opening_tag = section.group(0)
        if re.search(rf"\bvariant=[\"']{variant}[\"']", opening_tag) is None:
            raise RuntimeError(
                f'{side} {region} region must use variant="{variant}": {path_text}'
            )
        has_fill = (
            re.search(
                r"\bisFilled(?:(?=\s|>)|(?:\s*=\s*(?:\{true\}|[\"']true[\"'])))",
                opening_tag,
            )
            is not None
        )
        if requires_fill and not has_fill:
            raise RuntimeError(
                f"{side} body region must set isFilled: {path_text}"
            )
        if (
            region == "body"
            and not requires_fill
            and re.search(r"\bisFilled\b", opening_tag) is not None
        ):
            raise RuntimeError(
                f"{side} body region must omit isFilled: {path_text}"
            )


def capture(args: argparse.Namespace) -> None:
    if not args.production_story.startswith("production-"):
        raise RuntimeError(
            "Production story id must come from a Production/... story"
        )
    if not args.mockup_story.startswith("parity-"):
        raise RuntimeError(
            "Mockup story id must come from a shell-free Parity/... story, "
            "not a full-shell Mockups/... design-review story"
        )
    if ".parity.stories." not in Path(args.mockup_story_file).name:
        raise RuntimeError(
            "--mockup-story-file must identify the matching *.parity.stories.* file"
        )
    boundary_command = [
        sys.executable,
        str(Path(__file__).with_name("check_production_data_boundary.py")),
        "--route",
        args.route,
        "--view",
        args.view,
        "--story",
        args.story_file,
    ]
    for source in args.required_source:
        boundary_command.extend(("--required-source", source))
    run(*boundary_command)
    run(
        sys.executable,
        str(
            ROOT
            / "skills/web-console/ui-spec/scripts/check_mockup_patternfly.py"
        ),
        args.mockup_story_file,
        args.mockup_view,
    )
    require_shell_free_mockup(args.mockup_view)
    body_filled = args.body_fill == "filled"
    require_page_surface_regions(
        args.mockup_view,
        "Mockup",
        args.header_variant,
        args.body_variant,
        body_filled,
    )
    require_page_surface_regions(
        args.view,
        "Production",
        args.header_variant,
        args.body_variant,
        body_filled,
    )
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    manifest_path = output / "visual-parity.json"
    fingerprint = source_fingerprint(output)
    attempt = 1
    history: list[object] = []
    if manifest_path.exists():
        previous = json.loads(manifest_path.read_text())
        attempt = int(previous.get("attempt", 0)) + 1
        previous_history = previous.get("history", [])
        if isinstance(previous_history, list):
            history = previous_history
        if (
            previous.get("status") == "comparison_failed"
            and previous.get("source_fingerprint") == fingerprint
        ):
            raise RuntimeError(
                "Previous comparison failed and no production, story, fixture, "
                "or owning-spec file changed; fix the mismatch before recapture"
            )
        if previous.get("status") == "comparison_failed":
            history = [
                *history,
                {
                    "attempt": previous.get("attempt"),
                    "captured_at": previous.get("captured_at"),
                    "review": previous.get("review"),
                },
            ]
    run("pnpm", "run", "build:storybook", cwd=WEB_CONSOLE)
    run("pnpm", "run", "build:storybook:mockups", cwd=WEB_CONSOLE)

    production_server, production_thread = start_server(WEB_CONSOLE / "storybook-static")
    mockup_server, mockup_thread = start_server(
        ROOT / "specs/web-console/mockups/storybook-static"
    )
    servers = [production_server, mockup_server]
    threads = [production_thread, mockup_thread]
    try:
        require_story(production_server.server_port, args.production_story, "Production")
        require_story(mockup_server.server_port, args.mockup_story, "Mockup")
        artifacts: dict[str, dict[str, object]] = {}
        for side, port, story_id in (
            ("production", production_server.server_port, args.production_story),
            ("mockup", mockup_server.server_port, args.mockup_story),
        ):
            for viewport, (width, height) in VIEWPORTS.items():
                path = output / f"{side}-{viewport}.png"
                har_path = output / f"{side}-{viewport}.har"
                run(
                    "pnpm", "exec", "playwright", "screenshot",
                    "--browser", "chromium",
                    "--viewport-size", f"{width},{height}",
                    "--wait-for-selector", "#storybook-root",
                    "--wait-for-timeout", "500",
                    "--full-page",
                    "--save-har", str(har_path),
                    story_url(port, story_id), str(path),
                    cwd=WEB_CONSOLE,
                )
                require_static_network(har_path, side)
                if path.stat().st_size == 0:
                    raise RuntimeError(f"Empty screenshot: {path}")
                artifacts[f"{side}_{viewport}"] = {
                    "path": str(path),
                    "sha256": sha256(path),
                    "viewport": {"width": width, "height": height},
                }
    finally:
        for server in servers:
            server.shutdown()
            server.server_close()
        for thread in threads:
            thread.join(timeout=5)

    manifest = {
        "schema_version": 2,
        "attempt": attempt,
        "status": "awaiting_review",
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "source_fingerprint": fingerprint,
        "stories": {"production": args.production_story, "mockup": args.mockup_story},
        "builds": {"production": "passed", "mockup": "passed"},
        "artifacts": artifacts,
        "history": history,
        "review": None,
    }
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    print(manifest_path)


def load_manifest(path_text: str) -> tuple[Path, dict[str, object]]:
    path = Path(path_text).resolve()
    return path, json.loads(path.read_text())


def verify_artifacts(manifest: dict[str, object]) -> None:
    artifacts = manifest.get("artifacts")
    if not isinstance(artifacts, dict):
        raise RuntimeError("Manifest has no artifacts")
    required = {
        f"{side}_{viewport}"
        for side in ("production", "mockup")
        for viewport in VIEWPORTS
    }
    if set(artifacts) != required:
        raise RuntimeError(f"Expected exactly {sorted(required)}")
    for name, value in artifacts.items():
        if not isinstance(value, dict):
            raise RuntimeError(f"Invalid artifact entry: {name}")
        path = Path(str(value["path"]))
        if not path.is_file() or path.stat().st_size == 0:
            raise RuntimeError(f"Missing or empty artifact: {path}")
        if sha256(path) != value.get("sha256"):
            raise RuntimeError(f"Artifact changed after capture: {path}")


def parse_difference(value: str) -> dict[str, str]:
    parts = [part.strip() for part in value.split("::")]
    if len(parts) != 5 or any(not part for part in parts):
        raise RuntimeError(
            "Each --difference must use "
            "viewport::element::expected::actual::production-file"
        )
    viewport, element, expected, actual, production_file = parts
    if viewport not in {*VIEWPORTS, "both"}:
        raise RuntimeError("Difference viewport must be desktop, narrow, or both")
    production_path = (ROOT / production_file).resolve()
    if not production_path.is_file() or not any(
        production_path.is_relative_to(root.resolve()) for root in PRODUCTION_ROOTS
    ):
        raise RuntimeError(
            f"Difference fix target must be an existing production UI file: "
            f"{production_file}"
        )
    return {
        "viewport": viewport,
        "element": element,
        "expected": expected,
        "actual": actual,
        "production_file": str(production_path.relative_to(ROOT)),
    }


def review(args: argparse.Namespace) -> None:
    path, manifest = load_manifest(args.manifest)
    verify_artifacts(manifest)
    differences = [parse_difference(value) for value in args.difference]
    if args.result == "fail" and not differences:
        raise RuntimeError(
            "A failed review requires at least one structured --difference"
        )
    if args.result == "pass" and differences:
        raise RuntimeError("A passing review cannot contain unresolved differences")
    manifest["status"] = "complete" if args.result == "pass" else "comparison_failed"
    manifest["review"] = {
        "result": args.result,
        "notes": args.notes,
        "differences": differences,
        "attestation": "All four artifacts were opened with view_image in this task.",
        "reviewed_at": datetime.now(timezone.utc).isoformat(),
    }
    path.write_text(json.dumps(manifest, indent=2) + "\n")
    if args.result == "fail":
        raise RuntimeError(
            f"Visual comparison failed: {args.notes}. Change the implementation "
            "now, then rerun capture and inspect all four new images"
        )
    print(path)


def validate(args: argparse.Namespace) -> None:
    _, manifest = load_manifest(args.manifest)
    verify_artifacts(manifest)
    if manifest.get("builds") != {"production": "passed", "mockup": "passed"}:
        raise RuntimeError("Both Storybook builds must pass")
    review_data = manifest.get("review")
    if (
        manifest.get("status") != "complete"
        or not isinstance(review_data, dict)
        or review_data.get("result") != "pass"
    ):
        raise RuntimeError("Visual parity review is not complete")
    if not str(review_data.get("notes", "")).strip():
        raise RuntimeError("Review notes are required")
    if review_data.get("differences") != []:
        raise RuntimeError("Complete review has unresolved differences")
    history = manifest.get("history", [])
    if not isinstance(history, list):
        raise RuntimeError("Invalid comparison history")
    for attempt in history:
        review_data = attempt.get("review") if isinstance(attempt, dict) else None
        if (
            not isinstance(review_data, dict)
            or review_data.get("result") != "fail"
            or not review_data.get("differences")
        ):
            raise RuntimeError("Failed comparison history lacks a difference ledger")
    print("Complete")


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser()
    commands = result.add_subparsers(dest="command", required=True)
    capture_parser = commands.add_parser("capture")
    capture_parser.add_argument("--production-story", required=True)
    capture_parser.add_argument("--mockup-story", required=True)
    capture_parser.add_argument("--route", required=True)
    capture_parser.add_argument("--view", required=True)
    capture_parser.add_argument("--story-file", required=True)
    capture_parser.add_argument("--mockup-story-file", required=True)
    capture_parser.add_argument("--mockup-view", required=True)
    capture_parser.add_argument("--header-variant", default="default")
    capture_parser.add_argument("--body-variant", default="secondary")
    capture_parser.add_argument(
        "--body-fill", choices=("filled", "unfilled"), default="filled"
    )
    capture_parser.add_argument("--required-source", action="append", required=True)
    capture_parser.add_argument("--output", required=True)
    capture_parser.set_defaults(handler=capture)
    review_parser = commands.add_parser("review")
    review_parser.add_argument("--manifest", required=True)
    review_parser.add_argument("--result", choices=("pass", "fail"), required=True)
    review_parser.add_argument("--notes", required=True)
    review_parser.add_argument("--difference", action="append", default=[])
    review_parser.set_defaults(handler=review)
    validate_parser = commands.add_parser("validate")
    validate_parser.add_argument("--manifest", required=True)
    validate_parser.set_defaults(handler=validate)
    return result


if __name__ == "__main__":
    arguments = parser().parse_args()
    try:
        arguments.handler(arguments)
    except (OSError, RuntimeError, subprocess.CalledProcessError, KeyError, json.JSONDecodeError) as error:
        print(f"visual parity gate failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
