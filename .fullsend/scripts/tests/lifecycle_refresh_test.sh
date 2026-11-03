#!/usr/bin/env bash
set -euo pipefail
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s "$(dirname "${BASH_SOURCE[0]}")" -p test_code_state_refresh.py
