#!/usr/bin/env bash
# Generates the Keycloak theme ConfigMap manifests from source files in
# deploy/kind/prerequisites/keycloak-theme/ and writes them to
# deploy/base/keycloak-theme.yaml.
#
# Usage:  scripts/generate-keycloak-theme.sh
#
# Run this after editing any file under keycloak-theme/.  The Makefile
# target `keycloak-theme` calls this script automatically.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
THEME_DIR="${REPO_ROOT}/deploy/kind/prerequisites/keycloak-theme"
OUTPUT="${REPO_ROOT}/deploy/base/keycloak-theme.yaml"

# Read source files.
theme_properties=$(cat "${THEME_DIR}/theme.properties")
login_css=$(cat "${THEME_DIR}/login.css")
messages_en=$(cat "${THEME_DIR}/messages_en.properties")
logo_b64=$(base64 < "${THEME_DIR}/hypershell-logo.png" | tr -d '\n')
font_b64=$(base64 < "${THEME_DIR}/RedHatText-latin.woff2" | tr -d '\n')

# Write ConfigMaps.
cat > "${OUTPUT}" <<ENDOFCONFIGMAPS
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: keycloak-hypershell-theme
  namespace: keycloak
data:
  theme.properties: |
$(echo "${theme_properties}" | sed 's/^/    /')
  login.css: |
$(echo "${login_css}" | sed 's/^/    /')
  messages_en.properties: |
$(echo "${messages_en}" | sed 's/^/    /')
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: keycloak-hypershell-theme-assets
  namespace: keycloak
binaryData:
  hypershell-logo.png: ${logo_b64}
  RedHatText-latin.woff2: ${font_b64}
ENDOFCONFIGMAPS

# The base64-encoded PNG contains an incidental byte sequence that trips
# the forbidden-terms scanner.  Update the whitelist entry to match the
# current line number so `make check` stays green.
whitelist="${REPO_ROOT}/.forbidden-terms-whitelist.json"
b64_line=$(grep -n 'hypershell-logo.png:' "${OUTPUT}" | tail -1 | cut -d: -f1)
if [[ -n "${b64_line}" ]] && [[ -f "${whitelist}" ]]; then
  python3 - "${whitelist}" "${b64_line}" <<'PYEOF'
import json, sys
whitelist_path, line_num = sys.argv[1], int(sys.argv[2])
wl = json.loads(open(whitelist_path).read())
updated = False
for e in wl:
    if e['filename'] == 'deploy/base/keycloak-theme.yaml':
        e['line'] = line_num
        updated = True
        break
if not updated:
    wl.append({'filename': 'deploy/base/keycloak-theme.yaml',
               'line': line_num,
               'rationale': 'Generated base64-encoded PNG image data contains an incidental byte sequence; regenerate with make keycloak-theme.'})
open(whitelist_path, 'w').write(json.dumps(wl, indent=2) + '\n')
PYEOF
fi

echo "Regenerated theme ConfigMaps in ${OUTPUT##*/}"
