#!/usr/bin/env bash
set -euo pipefail
# Fullsend runs validation from the iteration directory after downloading the
# sandbox output into ./output (the sandbox's FULLSEND_OUTPUT_DIR is not a host path).
# A completed model turn is insufficient if it reports partial sync failures.
jq -e '(.status == "success") and (.errors | type == "array" and length == 0) and (.records | type == "array")' \
  output/result.json >/dev/null
