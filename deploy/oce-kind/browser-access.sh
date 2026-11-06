#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${OCE_REPO:?Set OCE_REPO}"
: "${OCE_KIND_STATE:?Set OCE_KIND_STATE}"
export KUBECONFIG="$OCE_KIND_STATE/kubeconfig"
[[ $(kubectl config current-context) == kind-hypershell-oce ]]
assets="$OCE_REPO/docs/snippets/openshell-quickstart/kubernetes"
tls="$OCE_KIND_STATE/owner-tls"
mkdir -p "$tls"
if [[ ! -e $tls/ca.crt ]]; then
  openssl req -x509 -newkey rsa:3072 -nodes -days 30 -subj '/CN=OCE local browser CA' \
    -addext basicConstraints=critical,CA:true,pathlen:0 -addext keyUsage=critical,keyCertSign,cRLSign \
    -keyout "$tls/ca.key" -out "$tls/ca.crt" > "$tls/issuance.log" 2>&1
fi
openssl req -new -newkey rsa:2048 -nodes -subj '/CN=OCE local browser' \
  -keyout "$tls/tls.key" -out "$tls/tls.csr" >> "$tls/issuance.log" 2>&1
printf '%s\n' 'basicConstraints=critical,CA:false' 'keyUsage=critical,digitalSignature,keyEncipherment' \
  'extendedKeyUsage=serverAuth' 'subjectAltName=DNS:console.hypershell-oce.oce.localhost,DNS:*.agents.hypershell-oce.oce.localhost' > "$tls/leaf.ext"
openssl x509 -req -in "$tls/tls.csr" -CA "$tls/ca.crt" -CAkey "$tls/ca.key" \
  -CAcreateserial -days 28 -extfile "$tls/leaf.ext" -out "$tls/tls.crt" >> "$tls/issuance.log" 2>&1
openssl verify -CAfile "$tls/ca.crt" -verify_hostname console.hypershell-oce.oce.localhost "$tls/tls.crt"
kubectl -n oce-system create secret tls oce-owner-browser-tls --cert="$tls/tls.crt" --key="$tls/tls.key" --dry-run=client -o json | kubectl apply -f -
kubectl -n oce-system create configmap oce-owner-listener --from-file="$assets/owner-listener.mjs" --dry-run=client -o json | kubectl apply -f -
kubectl -n oce-system get svc openclaw-enterprise-api -o json > "$OCE_KIND_STATE/api-service.json"
kubectl -n kube-system get svc kube-dns -o json > "$OCE_KIND_STATE/dns-service.json"
jq -n --arg image "$(jq -er .controller "$OCE_KIND_STATE/images.json")" \
  --argjson api "$(cat "$OCE_KIND_STATE/api-service.json")" --argjson dns "$(cat "$OCE_KIND_STATE/dns-service.json")" \
  -f "$assets/owner-listener.jq" > "$OCE_KIND_STATE/listener.json"
kubectl apply -f "$OCE_KIND_STATE/listener.json"
kubectl -n oce-system rollout status deployment/oce-owner-listener --timeout=3m
printf 'Run in a separate terminal:\nkubectl --kubeconfig %q --context kind-hypershell-oce -n oce-system port-forward --address 127.0.0.1 deployment/oce-owner-listener 18443:8443\n' "$KUBECONFIG"
