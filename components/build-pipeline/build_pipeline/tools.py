"""Agent tools for LLM-backed steps.

Real filesystem and shell tools scoped to the repository root. LangChain is
imported lazily inside ``build_tools`` so the module imports without it.
"""

from __future__ import annotations

import subprocess
from pathlib import Path


def _within(root: Path, rel: str) -> Path:
    resolved = (root / rel).resolve()
    if resolved != root and root not in resolved.parents:
        raise ValueError(f"path {rel!r} escapes the repository root")
    return resolved


def build_tools(repo_root: str | Path, shell_timeout: float = 1800.0) -> list:
    """Return LangChain tools bound to ``repo_root``."""
    from langchain_core.tools import tool

    root = Path(repo_root).resolve()

    @tool
    def read_file(path: str) -> str:
        """Read a UTF-8 text file at a repo-root-relative path."""
        return _within(root, path).read_text()

    @tool
    def write_file(path: str, content: str) -> str:
        """Write (create/overwrite) a UTF-8 text file at a repo-root-relative path."""
        fp = _within(root, path)
        fp.parent.mkdir(parents=True, exist_ok=True)
        fp.write_text(content)
        return f"wrote {path} ({len(content)} bytes)"

    @tool
    def run_shell(command: str, cwd: str = ".") -> str:
        """Run a shell command from a repo-root-relative cwd; returns exit code and output."""
        proc = subprocess.run(
            command, shell=True, cwd=str(_within(root, cwd)),
            capture_output=True, text=True, timeout=shell_timeout,
        )
        return f"exit={proc.returncode}\n--stdout--\n{proc.stdout}\n--stderr--\n{proc.stderr}"[-6000:]

    return [read_file, write_file, run_shell]
