#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_funnel_jira_auth.py
node --test scripts/test_funnel_jira_request.mjs
