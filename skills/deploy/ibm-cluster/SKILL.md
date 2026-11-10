---
name: ibm-cluster
description: >
  Provision (and later retire) an IBM Cloud ROKS OpenShift cluster on VPC Gen2 to
  serve as a HyperShell Cloud Hub. Ensures a version with a working built-in,
  CIO-managed Gateway API (ROKS 4.22 or later) so tenant-gateway manifests match the
  AWS reference.
  Use when: "create IBM cluster", "new ROKS cluster", "provision cloud hub on IBM",
  "upgrade IBM OpenShift", "hysh-ibm".
---

# IBM Cloud (ROKS) Cluster Provisioning

Stands up a VPC Gen2 OpenShift cluster on IBM Cloud as a Cloud Hub. After it is
`normal`, deploy platform services with [`deploy-cluster`](../deploy-cluster/SKILL.md)
and tenant ingress with [`cloud-hub-ingress-bootstrap`](../cloud-hub-ingress-bootstrap/SKILL.md).

> **Validated on ROKS 4.22 (`hypc-ibm-00`, `hypc-ibm-03`):** tenant gateways are
> published through the Gateway API on the shared `openshift-ingress/openshell-grpc-gateway`.
> The developer flow requires the workspace-membership grant in §5.9; the
> sandbox↔gateway internal TLS and the Deployment-only workload in §5.5.

## Why the OpenShift version matters (critical)

The tenant-gateway ingress uses the Kubernetes Gateway API. The **built-in,
Cluster-Ingress-Operator-managed** Gateway API (`openshift-default` GatewayClass,
matching the AWS reference) is **GA only on OpenShift >= 4.19**, and on ROKS it
works only from **4.22**: on 4.21 the Ingress Operator's `istiod` image cannot be
pulled (see Step 4). On older ROKS (e.g. 4.17) you would have to hand-install
OSSM 3 / Sail and accept a divergent `istio` GatewayClass. **Always provision ROKS
>= 4.22** (`4.22_openshift`, the latest 4.22 patch at create time). ROKS control-plane feature sets can be `CustomNoUpgrade`,
which blocks in-place upgrades - so getting a newer version means a new cluster,
not an upgrade.

## Prerequisites

- `ibmcloud` CLI logged in (`ibmcloud login`; suggest `! ibmcloud login` in-session)
- `container-service` plugin (`ibmcloud plugin install container-service`)
- A target resource group (`ibmcloud target -g <group>`)
- An existing VPC + subnet in the target zone, and a Cloud Object Storage instance
  (required for the ROKS internal registry)

## Step 1: Discover parameters (mirror an existing reference cluster)

```bash
ibmcloud target -g Default
ibmcloud ks versions --show-version OpenShift              # pick >= 4.22

# Mirror the reference cluster's shape:
ibmcloud ks cluster get     --cluster <reference> | grep -iE "resource group|vpc|zone"
ibmcloud ks worker-pool get --cluster <reference> --worker-pool default | grep -iE "flavor|vpc"
ibmcloud ks subnets --provider vpc-gen2 --vpc-id <vpc-id> --zone <zone>
ibmcloud resource service-instances --service-name cloud-object-storage   # reuse or note the COS name
ibmcloud resource service-instance <cos-name> --output json | grep '"crn"'   # need the CRN, not the GUID
```

A COS instance is a required `--cos-instance` flag value, but the registry does not
actually end up COS-backed on this account - see "Internal registry storage" below
before assuming you need COS or an IAM authorization for it.

Reference values captured for `hysh-ibm-01` (mirrors `hypershell-cluster`,
2026-08-15), with the version raised to 4.22:

| Parameter | Value |
|-----------|-------|
| Version | `4.22_openshift` |
| Resource group | `Default` |
| Zone | `us-east-1` |
| VPC | `r014-be56e5de-5cd9-493f-8ac2-149791cdc58b` |
| Subnet | `hypershell-subnet-1` / `0757-cacfbdee-1d22-444c-8ce5-5eff35c43faf` |
| Flavor | `bx2.4x16` |
| Workers / zone | `2` |
| COS instance | `hypershell-cos` / guid `e674d660-110e-49a2-94d5-6a8e7ef5fcd1` |

## Step 2: Create the cluster

```bash
ibmcloud ks cluster create vpc-gen2 \
  --name hysh-ibm-01 \
  --version 4.22_openshift \
  --zone us-east-1 \
  --vpc-id r014-be56e5de-5cd9-493f-8ac2-149791cdc58b \
  --subnet-id 0757-cacfbdee-1d22-444c-8ce5-5eff35c43faf \
  --flavor bx2.4x16 \
  --workers 2 \
  --cos-instance "crn:v1:bluemix:public:cloud-object-storage:global:a/dca8e7b41db847da9e58bf43e92a7ccf:e674d660-110e-49a2-94d5-6a8e7ef5fcd1::"
```

`--cos-instance` requires the **CRN** (the bare GUID fails with `E4acb "could not
find the specified cloud object storage instance"`). Provisioning is asynchronous
(~30–60 min) and incurs cost - confirm before running.

A `Ece8a: Could not create a bucket in your cloud object storage instance` warning
is expected here and is **not** blocking - the registry falls back to `emptyDir`.
See "Internal registry storage" below to choose a persistent backend instead.

## Step 3: Watch until `normal`

```bash
ibmcloud ks cluster get --cluster hysh-ibm-01 | grep -iE "state|status|ingress"
ibmcloud ks workers    --cluster hysh-ibm-01
```

Wait for cluster `State: normal`, all workers `Normal`, and `Ingress Status: healthy`
(the ingress subdomain + IBM-managed wildcard cert appear only once ingress is up).

## Step 4: Get kubeconfig and verify Gateway API is built in

```bash
ibmcloud ks cluster config --cluster hysh-ibm-01 --admin   # --admin = cert-based; a plain token context 401s
oc get clusterversion version -o jsonpath='{.status.desired.version}{"\n"}'   # expect 4.22.x
oc get featuregate cluster -o jsonpath='{.spec.customNoUpgrade.enabled}{"\n"}' | tr ',' '\n' | grep -i gateway
                                                                             # expect GatewayAPI + GatewayAPIController enabled
oc get crd | grep gateway.networking.k8s.io                                  # gateways/grpcroutes CRDs present
oc get gatewayclass                                                          # NOTE: empty on a fresh cluster (see below)
```

### The `openshift-default` GatewayClass is NOT auto-created - you create it (triggers Istio)

On OCP >= 4.19 the Gateway API **CRDs** are managed automatically (feature gates
`GatewayAPI` + `GatewayAPIController` are enabled), but the `openshift-default`
GatewayClass does **not** appear on its own. The admin creates it, and that creation
is what tells the (IBM-managed, hidden-control-plane) Cluster Ingress Operator to
install `istiod` into `openshift-ingress`:

```bash
cat <<'EOF' | oc apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: openshift-default
spec:
  controllerName: openshift.io/gateway-controller/v1
EOF
oc get gatewayclass openshift-default -o jsonpath='{.status.conditions[?(@.type=="Accepted")].status}{"\n"}'   # want True
oc -n openshift-ingress get pods -l app=istiod                                # istiod should reach Running
```

### ROKS 4.21 cannot run the built-in Gateway API

Use ROKS 4.22 or later. On 4.22 the Ingress Operator installs its own `istiod`
(the `GatewayAPIWithoutOLM` path, no OSSM subscription). Verified on ROKS 4.22.13
(2026-09-30, `hypc-ibm-00`): the GatewayClass is `Accepted`, `istiod` runs, and a
Gateway gets a public IBM VPC load balancer in seconds.

On ROKS 4.21 the `istiod-openshift-gateway` pod stays in `ImagePullBackOff`:

- IBM's node-level crio mirror redirects the OSSM images to
  `us.icr.io/armada-extensions/...`, which does not stock them (`manifest unknown`).
- The direct pull from `registry.redhat.io` times out when worker egress is closed
  (see the next section).
- You cannot add an `ImageDigestMirrorSet` on ROKS, because IBM owns the
  HostedCluster.

If `oc get clusterversion` shows 4.21, provision a new 4.22 cluster instead.

### Worker egress: the ROKS Security Group

The ROKS worker **Security Group** `kube-<clusterID>` (e.g. `kube-da0a9c9w0br7f5gf3dd0`)
is **default-deny outbound**. It allows only IBM Cloud infrastructure (service
endpoints `161.26.0.0/16` / `166.8.0.0/14`, VPE gateways, the in-cluster
registry, metadata `169.254.169.254`). DNS resolves, but no packets reach the
public internet. So `registry.redhat.io`, `quay.io`, and any **app-level** egress
(e.g. `oauth2.googleapis.com` when a gateway mints a Vertex AI token - see §5.7)
all time out. The Public Gateway is attached and the subnet ACL is wide open; the
SG is the gate. Diagnose from any pod with a shell (gateway pods are distroless;
use a debug pod):
`timeout 8 bash -c 'cat </dev/null >/dev/tcp/1.1.1.1/443' && echo OPEN || echo BLOCKED`.

Adding a rule to this ROKS-managed SG only *grants* egress (it never removes
IBM's rules):

```bash
SG=$(ibmcloud is security-groups --vpc <vpc-id> --output json \
  | python3 -c "import json,sys;print(next(g['id'] for g in json.load(sys.stdin) if g['name'].startswith('kube-') and 'lbaas' not in g['name'] and 'vpegw' not in g['name']))")
ibmcloud is security-group-rule-add "$SG" outbound tcp --remote 0.0.0.0/0 --port-min 443 --port-max 443
```

This enables all outbound HTTPS from workers (egress-only, port 443). Cloud-model
providers need it (§5.7). It also lets nodes pull directly from
`registry.redhat.io`, `quay.io` and `ghcr.io`, because crio shares the node's SG.
With it in place you can skip the image mirroring in §5.2. This is IBM Cloud VPC
state, **not** reproducible from GitOps; re-apply it on any rebuild.

## Step 5: Deploy HyperShell + a tenant gateway (Gateway API)

ROKS uses the same Gateway API ingress as the AWS reference. Every tenant gateway
gets a `GRPCRoute`, and its console gets an `HTTPRoute`, both attached to the
shared `openshift-ingress/openshell-grpc-gateway`.

1. Make the worker nodes able to pull images: open egress (above) or mirror the
   images (§5.0-§5.2).
2. Install cert-manager (§5.1).
3. Run [`cloud-hub-ingress-bootstrap`](../cloud-hub-ingress-bootstrap/SKILL.md).
   It creates the shared Gateway, the wildcard certificate and the wildcard DNS
   record for the IBM VPC load balancer.
4. Deploy the platform with the `deploy/openshift` overlay (§5.3).

### 5.0: Registry push identity (only when mirroring images)

`ibmcloud ks cluster config --admin` gives a **certificate-based** kubeconfig with
no bearer token, so `oc whoami -t` fails and you cannot `podman login` the registry
route as yourself. Mint a ServiceAccount token instead:

```bash
oc -n hypershell create sa pusher
oc adm policy add-cluster-role-to-user system:image-builder -z pusher -n hypershell   # cluster-wide: lets pusher write any namespace's imagestreams
REG=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}')
podman login --tls-verify=false -u pusher -p "$(oc -n hypershell create token pusher --duration=2h)" "$REG"
```

Tokens are short-lived; re-mint (and re-`podman login`) if a later push 401s.

### 5.1: Install cert-manager (hard prerequisite)

cert-manager is **not** optional. It issues the shared Gateway's wildcard
certificate (`cloud-hub-ingress-bootstrap`). It also mints each tenant's per-tenant
CA (`openshell-ca` Issuer + `openshell-ca`/`openshell-server` Certificates) for the
gateway pod's own server TLS, **plus an `openshell-client` Certificate
(`openshell-client-tls`)**. That client cert is NOT external-client mTLS (external
clients authenticate via OIDC through the shared Gateway). It exists so sandbox
runners can verify the gateway's server cert. openshell 0.0.109 mounts
`openshell-client-tls` into every sandbox and sets `OPENSHELL_TLS_CA` from its
`ca.crt` whenever `gateway.toml` sets `client_tls_secret_name`. Without it every
sandbox crashloops on `OPENSHELL_TLS_CA is required` and never reaches Ready (see
§5.5). The control plane fails closed (`cert-manager is required but not
available`) without cert-manager.

When worker egress is open, install cert-manager from its release manifest as
usual. When it is closed, the OperatorHub catalog pods cannot pull either
(`ImagePullBackOff`), so install by mirroring the images:

```bash
V=v1.16.3
for I in controller webhook cainjector; do
  skopeo copy --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
    docker://quay.io/jetstack/cert-manager-$I:$V \
    docker://$REG/cert-manager/cert-manager-$I:$V
done
# download the release manifest, repoint the three images at the mirror, apply:
curl -sL https://github.com/cert-manager/cert-manager/releases/download/$V/cert-manager.yaml \
  | sed -E 's#quay.io/jetstack/(cert-manager-[a-z]+):#image-registry.openshift-image-registry.svc:5000/cert-manager/\1:#g' \
  | oc apply -f -
oc -n cert-manager rollout status deploy/cert-manager
```

The controller caches cert-manager detection at startup, so **restart it** after
cert-manager is up: `oc -n hypershell-system rollout restart deploy/hypershell-controller`.

### 5.2: Mirror the platform + tenant workload images (only when egress is closed)

Skip this when worker egress is open. Otherwise mirror the platform images
(api-server, controller, PostgreSQL) per [`deploy-cluster`](../deploy-cluster/SKILL.md)
Step 3 into `.svc:5000/hypershell/{hypershell-api-server,hypershell-controller,postgresql}`.
Then mirror the **tenant gateway** images into the `openshift` namespace. Every
namespace's default SA can pull from it, so per-tenant image-pull secrets are
unnecessary:

```bash
# postgres/RHEL images carry signatures the internal registry rejects -> --remove-signatures
skopeo copy --remove-signatures --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
  docker://docker.io/library/postgres:18 docker://$REG/openshift/postgres:18
# Source images are normally pinned by digest (resolved from the Quay tag API).
# The v0.1.6-rhaiv.4 bump could not reach quay.io from the agent runtime that
# made this change, so these fall back to a tag-only reference; resolve and pin
# the digest before mirroring with
# `skopeo inspect --format '{{.Digest}}' docker://<ref>`. A bare tag is a
# mutable reference and only a last-resort fallback.
skopeo copy --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
  docker://quay.io/opendatahub/odh-openshell-gateway:v0.1.6-rhaiv.4    docker://$REG/openshift/openshell-gateway:v0.1.6-rhaiv.4
skopeo copy --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
  docker://quay.io/opendatahub/odh-openshell-supervisor:v0.1.6-rhaiv.4 docker://$REG/openshift/openshell-supervisor:v0.1.6-rhaiv.4
skopeo copy --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
  docker://quay.io/opendatahub/odh-openshell-sandbox:v0.1.6-rhaiv.4 docker://$REG/openshift/openshell-sandbox-runtime:v0.1.6-rhaiv.4
oc -n openshift get is    # expect openshell-gateway, openshell-supervisor, openshell-sandbox-runtime, postgres
```

Point the platform at the mirror in a local kustomize overlay on top of
`deploy/openshift`: an `images:` block for api-server, controller and PostgreSQL,
and a controller env patch that sets `GATEWAY_IMAGE`, `GATEWAY_SUPERVISOR_IMAGE`
and `GATEWAY_SANDBOX_RUNTIME_IMAGE` to the
`image-registry.openshift-image-registry.svc:5000/openshift/...` pullspecs.

### 5.3: Deploy with the `deploy/openshift` overlay

```bash
oc kustomize deploy/openshift | oc apply -f -    # or your local overlay from §5.2
oc -n hypershell-system rollout status deploy/hypershell-controller
```

Keep these controller settings aligned with this cluster (patch them in a local
overlay):

- `GATEWAY_API_GATEWAY_NAME=openshell-grpc-gateway` and
  `GATEWAY_API_GATEWAY_NAMESPACE=openshift-ingress`: the shared Gateway from
  `cloud-hub-ingress-bootstrap`.
- `GATEWAY_API_BASE_DOMAIN=<wildcard base domain>`: the domain the shared
  Gateway's wildcard listener and DNS record cover.
- `GATEWAY_API_HTTP_LISTENER_NAME`: the listener name of the shared Gateway, so
  console `HTTPRoute`s attach to it.
- `HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES=provider/ibm,network/public`.
- Leave `GATEWAY_INGRESS_MODE` unset (or set it to `gateway-api`). The controller
  then publishes every gateway through the Gateway API.
- **`deploy/base/platform-resources/controller-rbac.yaml`** - a cluster-wide `ClusterRole` for the
  controller. Reconciling whole tenants needs cluster-wide
  namespaces/secrets/services/deployments/networkpolicies, the Gateway API route
  kinds (`gateways`, `grpcroutes`, `httproutes`, `backendtlspolicies`), and
  `bind` on the `system:openshift:scc:privileged` ClusterRole (so the controller
  can bind the sandbox SA to the privileged SCC without an RBAC escalation error).

### 5.4: Provision a tenant gateway

The control plane reads the gateway's own `image` and `supervisor_image` fields
and requires them to be set explicitly. Required environment variables `GATEWAY_IMAGE` and
`GATEWAY_SUPERVISOR_IMAGE` (set in `deploy/base/platform-resources/controller.yaml`) define the authoritative
image sources and have no fallback defaults. When you mirror images, set both to the
mirrored internal refs. Pass
`namespace` explicitly (the deployed API image still validates it as required despite the
OpenAPI `readOnly` marking):

```bash
API="https://$(oc -n hypershell-system get route hypershell-api -o jsonpath='{.spec.host}')/api/hypershell/v1"
# Create takes a required placement intent, not a cluster_id. The selector
# matches HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES from §5.3. The gateway
# database server is configured through the hypershell-gateway-database-admin
# Secret, not the API.
curl -sk -X POST "$API/gateways" -H 'Content-Type: application/json' -d '{
  "name":"ibm-test-gw","placement":{"selector":["network/public","provider/ibm"]},
  "namespace":"openshell-ibmtest",
  "image":"image-registry.openshift-image-registry.svc:5000/openshift/openshell-gateway:0.0.109",
  "supervisor_image":"image-registry.openshift-image-registry.svc:5000/openshift/openshell-supervisor:0.0.109"
}'
```

The control plane then: mints the per-tenant CA via cert-manager, **auto-injects
`gw-<tenant>.<base-domain>` into the server certificate SANs** (you do not set
`external_dns` by hand), waits for the gateway DB, deploys the gateway, creates a
`GRPCRoute openshell-gateway` (host `gw-<tenant>.<base-domain>`) and a console
`HTTPRoute` on the shared `openshift-ingress/openshell-grpc-gateway`, and
publishes `grpcs://<host>:443`.

### 5.5: Verify end to end

```bash
NS=openshell-ibmtest
oc -n "$NS" get pods            # openshell-gateway + -db Running, -certgen Completed
oc -n "$NS" get grpcroute openshell-gateway \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{"\n"}{end}'   # Accepted=True, ResolvedRefs=True
HOST=$(oc -n "$NS" get grpcroute openshell-gateway -o jsonpath='{.spec.hostnames[0]}')

# The shared Gateway terminates TLS with the publicly trusted wildcard certificate:
openssl s_client -connect "$HOST:443" -servername "$HOST" \
  -verify_hostname "$HOST" </dev/null 2>/dev/null | grep 'Verify return code'   # 0 (ok)

# gRPC transport through the shared Gateway (OIDC auth aside, this proves the path):
openshell status --gateway-endpoint "https://$HOST:443"   # Status: Connected
```

`openshell status --gateway-endpoint …` proves the transport path without auth. For an authenticated registration a **bare** `openshell gateway
add https://<host>` is wrong - it selects edge/"cloud" mode and 404s on these
gRPC-only gateways; use OIDC mode (`--oidc-issuer …`, §5.7).

**Two 0.0.109 gateway-workload details the control plane provisions for you** (both
were regressions found bringing up 0.0.109 on ROKS - noted so a future bump can
re-verify them):

- **Sandbox client TLS.** The gateway runs `topology = "combined"`, so it launches
  sandbox runners that must verify the gateway's own TLS server cert. The control
  plane mints a second cert-manager `Certificate openshell-client` (secret
  `openshell-client-tls`, same per-tenant CA as `openshell-server`) and sets
  `client_tls_secret_name = "openshell-client-tls"` in `gateway.toml`; the gateway
  copies its `ca.crt` into each runner as `OPENSHELL_TLS_CA`. Without it the sandbox
  agent crash-loops on `OPENSHELL_TLS_CA is required`. This is **internal**
  sandbox↔gateway TLS, distinct from external-client mTLS (which we do NOT use -
  external clients authenticate via OIDC through the shared Gateway, so there is no
  `client_ca_path` under `[openshell.gateway.tls]`). Verify:
  `oc -n "$NS" get certificate openshell-client` (READY=True) and
  `oc -n "$NS" get secret openshell-client-tls`.
- **Deployment, never StatefulSet.** The gateway workload is a `Deployment`. An
  earlier manifest set shipped a `statefulset.yaml` in the deploy order too, which
  raced the Deployment and left an orphaned crash-looping `openshell-gateway-0`
  pod. There must be exactly one gateway workload:
  `oc -n "$NS" get deploy,statefulset -l app.kubernetes.io/component=gateway`
  → a Deployment and **no** StatefulSet.

### 5.6: Enable Agent Sandboxes (required for `openshell sandbox ...`)

A running gateway can serve `status`, but `openshell sandbox list/create` return
gRPC `Unimplemented` until the **Agent Sandbox CRD + controller**
(`sandboxes.agents.x-k8s.io`, the upstream `kubernetes-sigs/agent-sandbox`
project) are installed cluster-wide. The gateway's Kubernetes compute driver
watches this CRD; without it the driver loops on `no supported Agent Sandbox API
version is available; tried v1beta1, v1alpha1` (404s). The HyperShell control
plane grants the tenant SA RBAC *against* `agents.x-k8s.io` and mints the
per-tenant sandbox SA + privileged-SCC binding, but it does **not** install the
CRD/controller - that is a cluster prerequisite, like cert-manager. Verified on
`hysh-ibm-01` (2026-08-15) with `agent-sandbox` **v0.5.5** (first line to serve
`v1beta1`, which gateway 0.0.109 prefers).

When worker egress is closed, the controller image
(`registry.k8s.io/agent-sandbox/agent-sandbox-controller:v0.5.5`) and the tenant
sandbox base image (`ghcr.io/nvidia/openshell-community/sandboxes/base:latest`)
are **not node-reachable**. Mirror both into the internal `openshift` namespace
(cluster-wide pullable) and repoint. With open egress, apply the release manifest
unchanged.

```bash
# a) mirror the controller image + the sandbox base image (openshift ns = globally pullable)
skopeo copy --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
  docker://registry.k8s.io/agent-sandbox/agent-sandbox-controller:v0.5.5 \
  docker://$REG/openshift/agent-sandbox-controller:v0.5.5
skopeo copy --remove-signatures --dest-tls-verify=false --dest-creds "pusher:$(oc -n hypershell create token pusher)" \
  docker://ghcr.io/nvidia/openshell-community/sandboxes/base:latest \
  docker://$REG/openshift/openshell-sandbox-base:latest

# b) install the CRD + controller, repointing ONLY the controller image at the mirror
V=v0.5.5
curl -sL https://github.com/kubernetes-sigs/agent-sandbox/releases/download/$V/sandbox.yaml \
  | sed "s#registry.k8s.io/agent-sandbox/agent-sandbox-controller:$V#image-registry.openshift-image-registry.svc:5000/openshift/agent-sandbox-controller:$V#g" \
  | oc apply -f -
oc -n agent-sandbox-system rollout status deploy/agent-sandbox-controller
oc get crd sandboxes.agents.x-k8s.io -o jsonpath='{.status.conditions[?(@.type=="Established")].status}{"\n"}'   # True
```

The upstream controller Deployment declares no `securityContext`, so OpenShift's
`restricted-v2` SCC assigns a UID and it runs unmodified (the `fsGroup`/`runAsUser`
blocks in `sandbox.yaml` are inside the CRD's OpenAPI *schema examples*, not the
controller pod). The CRD's conversion webhook is served by the controller itself
(it self-injects the caBundle via its `customresourcedefinitions` patch grant); no
cert-manager `Certificate` is needed. Because v0.5.x serves a single real version,
conversion is never invoked, so `list` works as soon as the CRD is Established.

**The gateway caches API discovery at startup**, so after the CRD exists you must
restart each already-running tenant gateway once for its compute driver to pick up
`agents.x-k8s.io`:

```bash
oc -n "$NS" rollout restart deploy/openshell-gateway   # then: 0 "no supported Agent Sandbox" logs; "Compute driver connected"
```

**Sandbox workload image must be node-reachable too.** Select the mirrored
workload image explicitly with `openshell sandbox create --from <mirror-image>`.
The control plane does not expose a workload-image override. Omitting `--from`
uses the vendored chart's `sandbox.image` default, which the worker nodes may
not be able to pull. Mirroring an image alone does not change that default.

Verify (headless, no CLI auth needed - the sandbox RPC requires an authenticated
caller since `allow_unauthenticated_users=false`, so an unauthenticated
`openshell sandbox list` returns `Unauthenticated` rather than `Unimplemented` once the
CRD is in; to prove the controller + mirrored-image path directly, apply a Sandbox CR):

```bash
cat <<'EOF' | oc apply -f -
apiVersion: agents.x-k8s.io/v1beta1
kind: Sandbox
metadata: { name: mirror-test, namespace: NS_HERE }
spec:
  podTemplate:
    spec:
      serviceAccountName: openshell-gateway-sandbox
      containers:
        - name: sandbox
          image: image-registry.openshift-image-registry.svc:5000/openshift/openshell-sandbox-base:latest
          command: ["sleep", "3600"]
EOF
oc -n "$NS" get sandbox mirror-test          # READY=True, REASON=DependenciesReady
oc -n "$NS" get pod mirror-test -o jsonpath='{.status.containerStatuses[0].imageID}{"\n"}'  # ...svc:5000/openshift/openshell-sandbox-base@sha256:...
oc -n "$NS" delete sandbox mirror-test
```

Full authenticated `openshell sandbox create` additionally needs
`openshell gateway add` (interactive edge/OIDC) so the CLI carries a bearer token.

### 5.7: OIDC default-secure gateways + cloud-model providers

**Default-secure every gateway via Keycloak (no per-gateway `oidc` field).** When
the controller finds a `hypershell-keycloak-admin` Secret in its namespace it runs
`reconcileKeycloakClient` on every gateway provision: it uses a `client_credentials`
grant to auto-create a per-gateway public PKCE Keycloak client `<name>-<id>`
(loopback redirects `http://127.0.0.1:*`,`http://localhost:*`), its
`openshell-admin`/`openshell-user` client roles + audience/`hypershell.roles`
protocol mappers, then persists the resulting `oidc` block back onto the Gateway
and into `gateway.toml`. Wire it once:

```bash
# a) admin secret in the controller namespace (keys read verbatim by main.go).
#    server-url stays IN-CLUSTER; the external issuer is set separately (below).
oc create secret generic hypershell-keycloak-admin -n hypershell \
  --from-literal=server-url="http://hypershell-keycloak-service.keycloak-system.svc.cluster.local:8080" \
  --from-literal=realm="hypershell" \
  --from-literal=client-id="hypershell-control-plane" \
  --from-literal=client-secret="control-plane-secret"

# b) client-facing issuer = the EXTERNAL Keycloak host (must match KC_HOSTNAME).
#    This overrides only the issuer written to the gateway/toml, not the admin URL.
oc set env deploy/hypershell-controller -n hypershell \
  GATEWAY_OIDC_ISSUER_URL="https://keycloak.<appdomain>/realms/hypershell"
oc rollout restart deploy/hypershell-controller -n hypershell
# log confirms: "keycloak integration enabled ... gateway reconciler ... keycloak=true"
```

The `hypershell-control-plane` service account needs realm-management roles
(`manage-clients`,`manage-users`,`view-users`,`query-clients`,`query-users`) or
the client-credentials grant 403s. Declare them in the realm import as a
`service-account-hypershell-control-plane` user with `clientRoles`
(bootstrap-hyperfleet `bases/hypershell/keycloak/realm-import.yaml`) -
`clientScopeMappings` alone does **not** grant SA roles. Per-user authorization is
separate: assign the `openshell-admin`/`openshell-user` client role on the
`<name>-<id>` client to each end user (sandbox create additionally needs workspace
membership - see §5.9).

### 5.9: Workspace membership - the second authz layer for standard users

openshell 0.0.109 enforces **two independent authorization systems**, and a
standard user must clear both to create a sandbox:

1. **OIDC role** (layer 1): the `openshell-admin`/`openshell-user` client role
   carried in the token's `hypershell.roles` claim (§5.7). This gates *whether the
   principal is an admin or a standard user* - not *which workspaces it can touch*.
2. **Workspace membership** (layer 2): an explicit, per-workspace membership record
   that is **not** claim-derived. An admin has implicit access to the `default`
   workspace; a standard `openshell-user` does **not** - it must be added as a
   member first, or `sandbox create` fails with
   `PermissionDenied: not a member of workspace 'default'` and no pod is created.

A platform admin (who has implicit `default` access) grants membership:

```bash
# subject = the user's OIDC `sub` (get it from `openshell -g <gw> whoami --output json`
# on the user's own registration, or decode the token's `sub` claim).
openshell -g <admin-gateway> workspace member add \
  --workspace default --subject <oidc-sub> --role user
# -> ✓ Added <sub> to workspace default as user
```

**CLI version matters.** The `workspace` subcommand did not exist in older CLIs
(the system `/bin/openshell` on the dev host is **0.0.55** and errors with
`unrecognized subcommand 'workspace'`). Use a CLI **>= 0.0.98** (there is no
downloadable 0.0.109 CLI release - only the gateway container image is 0.0.109; the
0.0.98 CLI reads the same config format and talks to a 0.0.109 gateway fine).

**CLI connect - OIDC mode, not edge/cloud mode.** A bare `openshell gateway add
https://<host>` treats the endpoint as edge-authenticated ("cloud") and 404s on
these gRPC-only gateways. Use OIDC mode. The shared Gateway serves a publicly
trusted wildcard certificate, so the CLI verifies TLS with no extra flags:

```bash
openshell gateway add \
  --oidc-issuer   "https://keycloak.<appdomain>/realms/hypershell" \
  --oidc-client-id "<name>-<id>" --oidc-audience "<name>-<id>" \
  "https://gw-<tenant>.<base-domain>:443"
```

**Cloud-model providers need worker internet egress.** `openshell provider create
--type google-vertex-ai --from-gcloud-adc …` makes the gateway pod call
`oauth2.googleapis.com` (token mint) and later `*.googleapis.com` (inference).
On ROKS that fails with `transport error … token endpoint request failed` until
the worker SG is opened for outbound 443 (see *Worker egress: the ROKS Security
Group* in Step 4):

```bash
openshell provider create \
  --name vertex-claude --type google-vertex-ai --from-gcloud-adc \
  --config VERTEX_AI_PROJECT_ID="$(gcloud config get-value project)" \
  --config VERTEX_AI_REGION=global
# ✓ Created provider ... / Configured GCP credentials from gcloud ADC and minted the initial access token
```

**`openshell sandbox connect`** execs the system `ssh`, whose `ProxyCommand`
re-execs `openshell ssh-proxy`. The child verifies the same publicly trusted
gateway certificate, so no extra TLS setting is needed:

```bash
openshell sandbox connect <name>
# remote shell runs as uid=1000790000(sandbox) on the sandbox pod
```

### 5.8: Run an agent (Claude Code) in a sandbox - credential-free via `inference.local`

An in-sandbox agent reaches the cloud model through the OpenShell **inference
router**, NOT by holding a credential. `inference.local:443` is a virtual host the
supervisor intercepts: it strips the client's key and injects the operator's
provider token server-side, translating the Anthropic `/v1/messages` body into
Vertex's `:rawPredict` contract. So the sandbox never sees a secret. See
[`openshell-inference-routing.spec.md`](../../../specs/platform/openshell-inference-routing.spec.md)
for the model. Do **not** try `CLAUDE_CODE_USE_VERTEX=1` - that makes Claude Code
do its own Google ADC (none in the sandbox) and hang.

```bash
# a) Point inference.local at the Vertex provider from 5.7 (workspace-level, persists).
#    --no-verify because the provider region is `global` (no region endpoint to probe).
openshell inference set -g <name> \
  --provider vertex-claude --model 'claude-sonnet-4-5@20250929' --no-verify
openshell inference get -g <name>   # confirm the user route
```

**CRITICAL - pin a non-effort model with `--model claude-sonnet-4-5`.** Claude Code
2.1.x defaults to an effort-capable model and emits newer request fields that the
Vertex Anthropic partner endpoint (`anthropic_version = vertex-2023-10-16`, strict
validation) rejects - and the router does NOT strip them:

- default model → `400 thinking: 'adaptive' does not match 'disabled'/'enabled'`
- with `MAX_THINKING_TOKENS=0` → `400 output_config.effort: Extra inputs are not permitted`

These fields are gated on the model, not on any env var (there is no Claude Code
flag to strip them). Selecting `claude-sonnet-4-5` (non-effort, non-adaptive)
suppresses both → HTTP 200. The router forces the served model via the URL anyway,
so `--model` here only controls the request *shape*, not which model answers.

```bash
openshell sandbox connect <sandbox>          # from 5.6 / the connect note above

# inside the sandbox - the API key value is discarded by the router:
ANTHROPIC_BASE_URL=https://inference.local ANTHROPIC_API_KEY=unused \
  claude --model claude-sonnet-4-5 -p "Reply with one word: PONG"     # -> PONG
```

**Make bare `claude` "just work"** - persist the model + base URL in the sandbox's
`~/.claude/settings.json` (HOME is `/sandbox`; merge, don't clobber `theme` etc.):

```json
{
  "model": "claude-sonnet-4-5",
  "env": {
    "ANTHROPIC_BASE_URL": "https://inference.local",
    "ANTHROPIC_API_KEY": "unused"
  }
}
```

After that, a bare `claude -p "..."` (no flags, no env) returns a real completion.

## Internal registry storage (COS is NOT required)

`--cos-instance` is a required create flag, but on this account the ROKS registry
does **not** actually end up COS-backed. Verified on the reference cluster
`hypershell-cluster` (2026-08-15):

```bash
oc get configs.imageregistry.operator.openshift.io cluster -o jsonpath='{.spec.storage}{"\n"}'
# -> {"emptyDir":{},"managementState":"Managed"}    # ephemeral, not COS
oc -n openshift-image-registry get secret image-registry-private-configuration -o jsonpath='{.data}'
# -> empty
```

So the `Ece8a: Could not create a bucket …` warning at create time is **not
blocking** - the registry falls back to `emptyDir` and image pushes for
`deploy-cluster` still work (that is how the reference cluster runs today). Pick a
registry storage backend deliberately:

| Option | Persistence | Setup | When |
|--------|-------------|-------|------|
| `emptyDir` (default fallback) | ephemeral - images lost if the registry pod restarts | none | matches reference; fine for demo/dev |
| **PVC (`ibmc-vpc-block-*`)** | persistent | patch registry `spec.storage.pvc` (RWO, single replica) | **recommended** for a persistent Cloud Hub; no COS/IAM dependency |
| COS-backed | persistent | needs a Kubernetes Service → COS IAM authorization | only if you want object-store backing |

PVC backend (recommended, self-contained) - **chosen for `hysh-ibm-01`**:

```bash
oc patch configs.imageregistry.operator.openshift.io cluster --type merge -p \
  '{"spec":{"storage":{"emptyDir":null,"pvc":{"claim":""}},"rolloutStrategy":"Recreate","replicas":1}}'
# null out emptyDir and let the operator auto-create the image-registry-storage PVC
# on the default ibmc-vpc-block storage class (RWO -> Recreate + single replica)
oc -n openshift-image-registry get pvc image-registry-storage        # verify Bound
oc get configs.imageregistry.operator.openshift.io cluster -o jsonpath='{.spec.storage}{"\n"}'
```

Apply this once the cluster is `normal` and before `deploy-cluster` Step 3
(image push), so the registry is persistent from the first push.

### Note on the COS s2s IAM authorization

The IBM-documented fix (`ibmcloud iam authorization-policy-create
containers-kubernetes cloud-object-storage Writer --target-service-instance-name
<cos>`) **failed on this account** with `BXNAC12104 "cloud-object-storage does not
has any supportedRoles for policyType authorization"`, and the CLI identity had no
visible `iam user-policies` - i.e. it lacks authorization-policy rights. If you
genuinely need COS-backed registry, create that authorization in the IBM Cloud
**console** (Manage → Access (IAM) → Authorizations) as an account admin, or use
the PVC backend above and skip COS entirely.

## Retiring the old reference cluster

Only after `hysh-ibm-01` is validated end to end (a tenant gateway reachable over
its subdomain). Then:

```bash
ibmcloud ks cluster rm --cluster <old-cluster> --force-delete-storage
```

Double-check you are naming the OLD cluster. Confirm the new cluster is serving
traffic first.
