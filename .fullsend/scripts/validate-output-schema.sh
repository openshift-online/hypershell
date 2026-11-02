#!/usr/bin/env bash
# validate-output-schema.sh - Validate agent output against a JSON Schema.
#
# Generic script used by the harness validation_loop (ADR 0022). Works for any
# agent: the schema path is configured per harness through validation_loop.schema
# and delivered as FULLSEND_OUTPUT_SCHEMA. Validation is read-only: it only reads
# the result file and the schema and never changes any label (SDLC-HOOK-06).
#
# Required env vars:
#   FULLSEND_OUTPUT_SCHEMA - path to the JSON Schema file
#
# Optional env vars:
#   FULLSEND_OUTPUT_FILE   - filename to validate (default: agent-result.json)
#
# The script looks for the output file in the iteration output directory. The
# working directory is the iteration dir (set by the runner).

set -euo pipefail

: "${FULLSEND_OUTPUT_SCHEMA:?FULLSEND_OUTPUT_SCHEMA must be set}"

OUTPUT_DIR="output"
if [[ ! -d "${OUTPUT_DIR}" ]]; then
  echo "FAIL: output directory not found"
  exit 1
fi

_output_file="${FULLSEND_OUTPUT_FILE:-agent-result.json}"
_output_file="$(basename "${_output_file}")"
RESULT_FILE="${OUTPUT_DIR}/${_output_file}"
if [[ ! -f "${RESULT_FILE}" ]]; then
  echo "FAIL: ${RESULT_FILE} not found"
  exit 1
fi
echo "Validating: ${RESULT_FILE} against ${FULLSEND_OUTPUT_SCHEMA}"

# Run Python in isolated mode (-I): do not put the iteration directory (the cwd)
# or user site-packages on the import path, so a planted json.py / jsonschema.py
# in the untrusted output tree cannot be imported in place of the stdlib or the
# installed package.
if ! python3 -I -m json.tool "${RESULT_FILE}" > /dev/null 2>&1; then
  echo "FAIL: ${RESULT_FILE} is not valid JSON"
  exit 1
fi

if ! python3 -I -c "import jsonschema" 2>/dev/null; then
  # Tooling fault (exit 3), not an invalid instance: the dependency required by
  # ADR 0022 is missing or not importable under isolated mode on this runner.
  echo "FAIL: python3 jsonschema package is not installed or not importable under python3 -I (required by ADR 0022)"
  exit 3
fi

# Exit codes from the validator below:
#   0 - the instance is valid against the schema
#   1 - the instance is invalid (a real schema validation error)
#   3 - a tooling fault (bad import, malformed/unsupported schema, draft not
#       supported by this jsonschema version). This is distinct from an invalid
#       instance so a version or environment problem is not misread as the agent
#       producing a bad result. The caller still treats any non-zero as "do not
#       advance" (ADR 0022); only the diagnostics differ.
_vstatus=0
python3 -I -c "
import json, sys

try:
    from jsonschema import validate, ValidationError
except Exception as e:  # noqa: BLE001 - any import/tooling failure is non-validation
    print(f'FAIL: jsonschema tooling error (import): {type(e).__name__}: {e}')
    sys.exit(3)

try:
    with open(sys.argv[1]) as f:
        instance = json.load(f)
    with open(sys.argv[2]) as f:
        schema = json.load(f)
except Exception as e:  # noqa: BLE001 - unreadable/unparsable inputs are tooling faults here
    print(f'FAIL: could not read result or schema: {type(e).__name__}: {e}')
    sys.exit(3)

try:
    validate(instance=instance, schema=schema)
    print('PASS: output validated against schema')
except ValidationError as e:
    print(f'FAIL: schema validation error: {e.message}')
    if e.path:
        print(f'  at: {\".\".join(str(p) for p in e.path)}')
    if 'properties' in e.schema:
        allowed = ', '.join(sorted(e.schema['properties'].keys()))
        print(f'  allowed properties: {allowed}')
    sys.exit(1)
except Exception as e:  # noqa: BLE001 - SchemaError, draft/resolution faults, etc.
    print(f'FAIL: schema tooling error: {type(e).__name__}: {e}')
    sys.exit(3)
" "${RESULT_FILE}" "${FULLSEND_OUTPUT_SCHEMA}" || _vstatus=$?

if [[ "${_vstatus}" -ne 0 ]]; then
  exit "${_vstatus}"
fi
