#!/usr/bin/env bash
set -euo pipefail
umask 077
# shellcheck source=../../scripts/cluster/output.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../scripts/cluster/output.sh"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE}"
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
k=(kubectl --kubeconfig "$KUBECONFIG" --context kind-hypershell-oce)
read -r gateway < "$OCE_KIND_STATE/gateway-namespace"
read -r namespace < "$OCE_KIND_STATE/namespace-id"
read -r agent < "$OCE_KIND_STATE/codex-agent-id"
# OCE's management grants isolate the gateway; preserve access through the existing kind ingress proxy.
"${k[@]}" -n hypershell-system get gateway hypershell-gw -o json > "$OCE_KIND_STATE/kind-gateway.json"
"${k[@]}" -n "$gateway" get service openshell-gateway -o json > "$OCE_KIND_STATE/gateway-service.json"
python3 - <<'PY'
import ipaddress, json, os
from pathlib import Path
state = Path(os.environ['OCE_KIND_STATE'])
gateway = json.loads((state / 'kind-gateway.json').read_text())
service = json.loads((state / 'gateway-service.json').read_text())
peers = []
for address in gateway.get('status', {}).get('addresses', []):
    if address['type'] == 'IPAddress':
        ip = ipaddress.ip_address(address['value'])
        peers.append({'ipBlock': {'cidr': f'{ip}/{ip.max_prefixlen}'}})
if not peers or not service['spec'].get('selector'):
    raise SystemExit('Kind ingress addresses and a concrete gateway Pod selector are required.')
policy = {'apiVersion': 'networking.k8s.io/v1', 'kind': 'NetworkPolicy',
          'metadata': {'name': 'oce-kind-gateway-ingress', 'namespace': service['metadata']['namespace']},
          'spec': {'podSelector': {'matchLabels': service['spec']['selector']},
                   'policyTypes': ['Ingress'], 'ingress': [{'from': peers, 'ports': [
                       {'protocol': 'TCP', 'port': port} for port in (8080, 8090)]}]}}
(state / 'kind-gateway-ingress.json').write_text(json.dumps(policy) + '\n')
PY
"${k[@]}" apply -f "$OCE_KIND_STATE/kind-gateway-ingress.json"
origin=https://console.hypershell-oce.oce.localhost:18443
oce_ready() {
  curl --fail --silent --show-error --max-time 5 --noproxy '*' \
    --cacert "$OCE_KIND_STATE/owner-tls/ca.crt" \
    --resolve console.hypershell-oce.oce.localhost:18443:127.0.0.1 \
    "$origin/api/auth/session" | jq -e '.data==null' >/dev/null
}
if ! oce_ready 2>/dev/null; then
  # Never replace an unrelated listener. A reusable forward must validate against this installation's CA.
  python3 - <<'PY'
import socket
with socket.socket() as listener:
    try:
        listener.bind(('127.0.0.1', 18443))
    except OSError:
        raise SystemExit('Port 18443 is occupied or OCE TLS is not ready. Inspect the listener before retrying.')
PY
  nohup "${k[@]}" -n oce-system port-forward --address 127.0.0.1 \
    deployment/oce-owner-listener 18443:8443 > "$OCE_KIND_STATE/console-forward.log" 2>&1 < /dev/null &
  forward=$!
  printf '%s\n' "$forward" > "$OCE_KIND_STATE/console-forward.pid"
  ready=false
  for _ in {1..30}; do
    if oce_ready 2>/dev/null; then ready=true; break; fi
    kill -0 "$forward" 2>/dev/null || break
    sleep 2
  done
  if [[ $ready != true ]]; then
    kill "$forward" 2>/dev/null || true
    wait "$forward" 2>/dev/null || true
    printf 'OCE browser forward failed; inspect %s/console-forward.log.\n' "$OCE_KIND_STATE" >&2
    exit 1
  fi
fi

keycloak=$("${k[@]}" -n keycloak get deployment keycloak -o json |
  jq -er '.spec.template.spec.containers[].env[]|select(.name=="KC_HOSTNAME")|.value')
suffix=$(python3 - "$keycloak" <<'PY'
import sys
from urllib.parse import urlsplit
url = urlsplit(sys.argv[1] if '://' in sys.argv[1] else 'https://' + sys.argv[1])
print(':' + str(url.port) if url.port and url.port != 443 else '')
PY
)
gateway_host=$("${k[@]}" -n "$gateway" get grpcroute openshell-gateway -o json | jq -er '.spec.hostnames[0]')
port=${suffix#:}
port=${port:-443}
"${k[@]}" -n hypershell-system get secret hypershell-ca-secret -o json |
  jq -er '.data["ca.crt"]' | base64 -d > "$OCE_KIND_STATE/hypershell-ca.crt"
gateway_ready() {
  # An empty protobuf message is a five-byte gRPC frame. Check trailers as well as HTTP status.
  printf '\0\0\0\0\0' | curl --http2 --fail --silent --show-error --max-time 5 --noproxy '*' \
    --cacert "$OCE_KIND_STATE/hypershell-ca.crt" --resolve "${gateway_host}:${port}:127.0.0.1" \
    --header 'Content-Type: application/grpc' --data-binary @- \
    --dump-header "$OCE_KIND_STATE/gateway-health.headers" --output "$OCE_KIND_STATE/gateway-health.bin" \
    "https://${gateway_host}${suffix}/openshell.v1.OpenShell/Health" || return 1
  tr -d '\r' < "$OCE_KIND_STATE/gateway-health.headers" | grep -qx 'grpc-status: 0'
}
for attempt in {1..30}; do
  if gateway_ready; then break; fi
  [[ $attempt != 30 ]] || { echo 'External OpenShell gRPC health failed; inspect the kind ingress proxy and gateway policies.' >&2; exit 1; }
  sleep 2
done
curl --fail --silent --show-error --max-time 15 --noproxy '*' \
  --cacert "$OCE_KIND_STATE/hypershell-ca.crt" \
  --resolve "console.hypershell.localhost:${port}:127.0.0.1" \
  "https://console.hypershell.localhost${suffix}/auth/login" >/dev/null
print_summary() {
  echo
  header "HyperShell + OCE are ready!"
  success "Codex answered OCE_READY using gpt-6-luna."
  echo
  header "HyperShell"
  info "Web console:  https://console.hypershell.localhost${suffix}"
  info "HTTP API:     https://api.hypershell.localhost${suffix}"
  info "Login:        admin / admin (or developer / developer)"
  echo
  header "OpenShell gateway"
  info "API endpoint: https://${gateway_host}${suffix}"
  info "Name:         dev-gateway"
  info "Namespace:    ${gateway}"
  info "CLI TLS:      SSL_CERT_FILE=${OCE_KIND_STATE}/hypershell-ca.crt"
  echo
  header "OCE and Codex"
  info "Console:      ${origin}/console/"
  info "Test agent:   ${origin}/console/agents/${agent}?namespace=${namespace}"
  info "Login:        admin@oce.localhost"
  info "Password file: ${OCE_KIND_STATE}/initial-admin-password"
  info "Open the test agent, then choose Open Agent UI to chat."
  echo
  header "Local access"
  info "Trust these CAs in your browser:"
  info "  HyperShell: ${OCE_KIND_STATE}/hypershell-ca.crt"
  info "  OCE:        ${OCE_KIND_STATE}/owner-tls/ca.crt"
  info "Kubeconfig:   ${KUBECONFIG}"
  info "OCE forward:  127.0.0.1:18443 (loopback only)"
  info "Remote host:  SSH-forward ports ${port} and 18443 to your laptop."
  echo
  info "Saved summary: ${OCE_KIND_STATE}/access.txt"
  info "Rerun the same make command to resume setup or restart the OCE forward."
  echo
}

# Keep the saved copy plain without disabling color on the terminal.
# shellcheck disable=SC2034 # Colors are read by the shared output helpers.
(BOLD='' BLUE='' CYAN='' GREEN='' YELLOW='' RED='' NC=''; print_summary) > "$OCE_KIND_STATE/access.txt"
print_summary
