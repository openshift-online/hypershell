---
name: cloud-hub-ingress-bootstrap
description: >
  Bootstrap the shared tenant-gateway wildcard ingress on a Cloud Hub (once per
  cluster) so the control plane's Gateway API reconciler has a shared Gateway to
  attach GRPCRoutes to. Cloud-agnostic with AWS (verified reference) and IBM Cloud
  (ROKS parity) variants. Use when: "wildcard subdomains for gateways", "shared
  gateway", "GATEWAY_API_GATEWAY_NAME is required", "deploy to IBM Cloud",
  "tenant gateway ingress", "VPC load balancer for gateways".
---

# Cloud Hub Ingress Bootstrap

Prerequisite for HyperShell tenant traffic. The control plane's reconciler
(`components/control-plane/internal/gateway/reconciler.go`) does **not** create a
`Gateway`; it requires a pre-existing shared one and fails closed
(`GATEWAY_API_GATEWAY_NAME is required`) until this bootstrap is done. This skill
stands up that shared Gateway plus its wildcard DNS + TLS so every tenant gets a
`gw-<tenant>.<base-domain>` subdomain behind a single load balancer.

Run this **once per Cloud Hub / gateway-hosting cluster**, before or alongside the
platform deploy (`/deploy-cluster`). It is idempotent - re-applying reconciles.

## Why one shared Gateway (not per-tenant)

A wildcard DNS record resolves to exactly **one** LB. A shared `Gateway` gives one
LB + one wildcard cert + one wildcard DNS record, with hostname-based routing per
tenant `GRPCRoute`. A per-tenant Gateway would need one LB and one DNS record each - incompatible with wildcard subdomains. Authoritative spec + verified manifests:
[`specs/platform/global-architecture.spec.md`](../../../specs/platform/global-architecture.spec.md)
(§ "Tenant Gateway Ingress - Reference Implementation" and "IBM Cloud Parity Plan").

## The only cloud-specific difference

Everything is identical across clouds **except the load balancer**, which the
cloud's CCM auto-provisions from the operator-created `Service type: LoadBalancer`.
No per-cloud LB manifest is written.

| Concern | AWS (ROSA, verified) | IBM Cloud (ROKS, VPC Gen2) |
|---------|----------------------|-----------------------------|
| GatewayClass | `openshift-default` (CIO-managed Istio) | `openshift-default` (verify GA on the ROKS OCP version) |
| Shared Gateway | `openshell-grpc-gateway` / `openshift-ingress` | identical |
| LB provisioned by CCM | AWS Classic ELB (`*.elb.amazonaws.com`) | VPC Load Balancer (`*.lb.appdomain.cloud`) |
| Base domain (example) | `*.openshell.stage.devshift.net` | `*.openshell.<ibm-env>.devshift.net` |
| TLS secret | `wildcard-openshell-stage-devshift-tls` | `wildcard-openshell-<ibm-env>-devshift-tls` |
| ClusterIssuer | `letsencrypt-devshiftnet-dns` (Route53 DNS-01) | **same issuer** - Route53 is central |
| DNS record | static Route53 wildcard CNAME → ELB | static Route53 wildcard CNAME → VPC LB |

Route53 hosts the `devshift.net` zone centrally, so DNS-01 issuance works from any
cloud with **no** cloud-native DNS integration.

## Per-cluster parameters

Set these once; they drive every manifest below.

```bash
export BASE_DOMAIN="openshell.stage.devshift.net"      # AWS example; IBM: openshell.<ibm-env>.devshift.net
export TLS_SECRET="wildcard-openshell-stage-devshift-tls"
export CERT_NAME="wildcard-openshell-stage-devshift"
export CLUSTER="hcmai"                                  # used in the Route53 credential secret name
export ROUTE53_ZONE_ID="Z05758033SZ8IESGUOY8E"         # devshift.net hosted zone
```

## Step 0: Preflight

```bash
oc whoami --show-server                                 # confirm the target Cloud Hub
oc get gatewayclass openshift-default                   # MUST exist; if absent, Gateway API not enabled (see Troubleshooting)
oc get crd | grep gateway.networking.k8s.io             # expect gateways/grpcroutes/httproutes CRDs
oc -n openshift-ingress get deployment cert-manager -o name 2>/dev/null \
  || oc get clusterissuer 2>/dev/null                   # confirm cert-manager present
```

If `gatewayclass openshift-default` is missing, stop. The built-in, CIO-managed
Gateway API is **GA only on OCP >= 4.19** - check `oc get clusterversion version
-o jsonpath='{.status.desired.version}'`. On OCP < 4.19 (e.g. the original IBM
`hypershell-cluster` on 4.17, confirmed 2026-08-15) this GatewayClass does not
exist and cannot be safely enabled on a managed control plane; provision a
>= 4.19 cluster with the [`ibm-cluster`](../ibm-cluster/SKILL.md) skill instead
(new Cloud Hub `hysh-ibm-01` on 4.21.27 was created for exactly this reason).

## Step 1: Seed the Route53 credential secret

cert-manager solves DNS-01 against Route53 for `devshift.net` regardless of cloud.

```bash
oc -n openshift-ingress create secret generic certmgr-${CLUSTER}-devshift-net-sa \
  --from-literal=aws_access_key_id="$AWS_ACCESS_KEY_ID" \
  --from-literal=aws_secret_access_key="$AWS_SECRET_ACCESS_KEY" \
  --dry-run=client -o yaml | oc apply -f -
```

## Step 2: Apply the ClusterIssuer (same on every cloud)

Manifest 4 in the spec. `hostedZoneID` = `$ROUTE53_ZONE_ID`; the two secret refs
point at `certmgr-${CLUSTER}-devshift-net-sa`. Apply, then:

```bash
oc get clusterissuer letsencrypt-devshiftnet-dns -o jsonpath='{.status.conditions[*].type}={.status.conditions[*].status}{"\n"}'
```

## Step 3: Apply the wildcard Certificate

Manifest 3 pattern, with `metadata.name=$CERT_NAME`, `spec.secretName=$TLS_SECRET`,
`spec.dnsNames=["*.${BASE_DOMAIN}"]`, in `openshift-ingress`. Wait for issuance:

```bash
oc -n openshift-ingress wait --for=condition=Ready certificate/${CERT_NAME} --timeout=300s
oc -n openshift-ingress get secret ${TLS_SECRET}     # must exist before the Gateway can Terminate TLS
```

## Step 4: Apply the shared Gateway

Manifest 1 pattern in `openshift-ingress`: `gatewayClassName: openshift-default`,
one `grpc` HTTPS/443 listener with `hostname: "*.${BASE_DOMAIN}"`,
`tls.mode: Terminate` → `certificateRefs: [{kind: Secret, name: $TLS_SECRET}]`,
`allowedRoutes.namespaces.from: All`. Applying it makes the Cluster Ingress
Operator create the Istio ingress `Service type: LoadBalancer`; the CCM then
provisions the cloud LB automatically.

```bash
oc -n openshift-ingress wait --for=condition=Programmed gateway/openshell-grpc-gateway --timeout=300s
LB=$(oc -n openshift-ingress get gateway openshell-grpc-gateway -o jsonpath='{.status.addresses[0].value}')
echo "LB hostname: $LB"     # AWS: *.elb.amazonaws.com  |  IBM: *.lb.appdomain.cloud
```

## Step 5: Create the wildcard DNS record

There is **no external-dns** - the Gateway reports DNS management `Unmanaged`.
Create the static Route53 record once, after the LB hostname is known:

```
*.${BASE_DOMAIN}.   CNAME   $LB
```

Verify resolution:

```bash
dig +short "gw-probe.${BASE_DOMAIN}"    # should return the LB / its A records
```

## Step 6: Point the control plane at the Gateway

Set on the `hypershell-controller` Deployment (see `/deploy-cluster`):

```bash
oc -n hypershell-api set env deployment/hypershell-controller \
  GATEWAY_API_GATEWAY_NAME=openshell-grpc-gateway \
  GATEWAY_API_GATEWAY_NAMESPACE=openshift-ingress \
  GATEWAY_API_BASE_DOMAIN="${BASE_DOMAIN}"
oc -n hypershell-api rollout status deployment/hypershell-controller
```

Defaults in code: `GATEWAY_API_GATEWAY_NAMESPACE` → `openshift-ingress`;
`GATEWAY_API_GATEWAY_NAME` is **required** (reconcile fails without it).

## Step 7: Verify end to end

```bash
# provision a test tenant gateway via the API, then:
NS=$(oc get ns -l hypershell.redhat.io/managed=true -o name | head -1 | cut -d/ -f2)
oc -n "$NS" get grpcroute openshell-gateway -o jsonpath='{.status.parents[*].conditions[*].type}={.status.parents[*].conditions[*].status}{"\n"}'
# expect Accepted=True and ResolvedRefs=True
openshell login "gw-${NS}.${BASE_DOMAIN}:443"   # CLI connects over the wildcard subdomain
```

## Cloud variants at a glance

### AWS (reference - already live on `hcmai` / hypershell-stage)
- Verified working; this skill is derived from it. LB is an AWS Classic ELB.
- Nothing to change beyond the parameters above.

### IBM Cloud / ROKS - do NOT run this skill (use Route ingress mode instead)

ROKS **cannot run** the CIO-managed Gateway API (HyperShift-hosted; OSSM images
unpullable; IDMS denied on the HostedCluster - verified on `hysh-ibm-01`, OCP
4.21.27, 2026-08-15). This bootstrap (shared Gateway) does not apply there.
Instead, the IBM Cloud Hub uses the **Route ingress mode**: the control plane
emits passthrough `Route`s on IBM's free `*.containers.appdomain.cloud` wildcard,
selected by `GATEWAY_INGRESS_MODE=route` via the `deploy/ibm` overlay - no shared
Gateway, wildcard cert, ClusterIssuer, or Route53. See the
[`ibm-cluster`](../ibm-cluster/SKILL.md) skill (Step 5) and
[`global-architecture.spec.md`](../../../specs/platform/global-architecture.spec.md)
(§ "IBM Cloud Cloud Hub - Route ingress mode").

Only run this skill on IBM if IBM later makes the Gateway API functional on the
HostedCluster; then unset `GATEWAY_INGRESS_MODE` to return to `gateway-api` mode.

## Troubleshooting

### `GATEWAY_API_GATEWAY_NAME is required` in controller logs
The shared Gateway isn't wired up. Complete Steps 4 and 6.

### Gateway API not installed (no `openshift-default` GatewayClass)
`oc get gatewayclass` empty and `oc get crd | grep gateway.networking.k8s.io`
returns nothing. The built-in CIO-managed Gateway API is **GA only on OCP >= 4.19**.
Check the version; if < 4.19 (verified on IBM `hypershell-cluster` @ 4.17.56,
2026-08-15), do **not** try to enable a Tech-Preview feature gate on a managed
control plane - provision a >= 4.19 cluster via the
[`ibm-cluster`](../ibm-cluster/SKILL.md) skill. Treat as a prerequisite blocker,
not a runtime error.

### Gateway stuck `Programmed=False`
Usually the TLS secret is missing - confirm Step 3 finished (`$TLS_SECRET` exists
in `openshift-ingress`) before the Gateway can terminate TLS.

### Certificate never becomes Ready
Check the DNS-01 challenge: `oc -n openshift-ingress get challenges,orders`. Common
causes: wrong `hostedZoneID`, or the `certmgr-${CLUSTER}-devshift-net-sa` creds lack
Route53 write access to the `devshift.net` zone.

### GRPCRoute `Accepted=False`
Hostname outside the listener wildcard, or `sectionName` mismatch. The route's
`parentRef` must be `{name: openshell-grpc-gateway, namespace: openshift-ingress,
sectionName: grpc}` and its hostname must fall under `*.${BASE_DOMAIN}`.

## Teardown (per cluster)

```bash
oc -n openshift-ingress delete gateway openshell-grpc-gateway
oc -n openshift-ingress delete certificate ${CERT_NAME}
oc -n openshift-ingress delete secret ${TLS_SECRET}
# then remove the wildcard Route53 record
```
