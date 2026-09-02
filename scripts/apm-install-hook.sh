#!/usr/bin/env bash
# APM install hook invoked by `apm install`. Do not call `apm install` here —
# that would recurse through apm.yml scripts.install.
set -euo pipefail

# Skill dependencies are resolved by apm from apm.yml; no extra steps required.
