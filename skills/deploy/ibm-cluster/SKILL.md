---
name: ibm-cluster
description: >
  Provision (and later retire) an IBM Cloud ROKS OpenShift cluster on VPC Gen2 to
  serve as a HyperShell Cloud Hub. Ensures a version with the built-in, CIO-managed
  Gateway API (OCP >= 4.19) so tenant-gateway manifests match the AWS reference.
  Use when: "create IBM cluster", "new ROKS cluster", "provision cloud hub on IBM",
  "upgrade IBM OpenShift", "hysh-ibm".
---

# IBM Cloud (ROKS) Cluster Provisioning

Stands up a VPC Gen2 OpenShift cluster on IBM Cloud as a Cloud Hub. After it is
`normal`, deploy platform services with [`deploy-cluster`](../deploy-cluster/SKILL.md)
and tenant ingress with [`cloud-hub-ingress-bootstrap`](../cloud-hub-ingress-bootstrap/SKILL.md).

## Why the OpenShift version matters (critical)

The tenant-gateway ingress uses the Kubernetes Gateway API. The **built-in,
Cluster-Ingress-Operator-managed** Gateway API (`openshift-default` GatewayClass,
matching the AWS reference) is **GA only on OpenShift >= 4.19**. On older ROKS
(e.g. 4.17) you would have to hand-install OSSM 3 / Sail and accept a divergent
`istio` GatewayClass. **Always provision >= 4.19** (IBM default at time of writing:
`4.21.27_openshift`). ROKS control-plane feature sets can be `CustomNoUpgrade`,
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
ibmcloud ks versions --show-version OpenShift              # pick >= 4.19 (default is fine)

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

Reference values captured for `hysh-ibm-01` (mirrors `hypershell-cluster`, 2026-08-15):

| Parameter | Value |
|-----------|-------|
| Version | `4.21.27_openshift` |
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
  --version 4.21.27_openshift \
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
oc get clusterversion version -o jsonpath='{.status.desired.version}{"\n"}'   # expect 4.21.x
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

### BLOCKER on ROKS 4.21 (confirmed 2026-08-15, hysh-ibm-01): OSSM istiod image unpullable

Creating the GatewayClass made the CIO spin up `istiod-openshift-gateway` in
`openshift-ingress`, but it lands in **`ImagePullBackOff`** and the GatewayClass
stays `Accepted=Unknown / reason=Pending (Waiting for controller)`. Root cause is an
IBM Cloud image-availability gap, verified two ways:

- The istiod image `registry.redhat.io/openshift-service-mesh/istio-pilot-rhel9@sha256:2a25…`
  is redirected by the **node-level crio mirror** (IBM sets this in the workers'
  `registries.conf`, *not* an `ImageDigestMirrorSet` - the cluster IDMS only mirrors
  OCP release images) to `us.icr.io/armada-extensions/registry-redhat-io/...`, which
  returns **`manifest unknown`** - IBM's mirror does not stock the OSSM images.
- The direct fallback to `registry.redhat.io` **times out**: worker egress to
  `registry.redhat.io` is blocked (`curl https://registry.redhat.io/v2/` → rc=124),
  while `us.icr.io/v2/` answers `HTTP 401` in ~7ms. The global `pull-secret` *does*
  contain `registry.redhat.io` creds - the problem is reachability + mirror stock,
  not auth.

So the native `openshift-default` path cannot pull `istiod` on ROKS out of the box.
Remediation options (pick with the user - do not silently mirror/patch):
1. **Mirror the OSSM image set into a worker-reachable registry** (IBM Container
   Registry `icr.io`, or the cluster internal registry via its route) from a host
   that *can* reach `registry.redhat.io`, then add an `ImageDigestMirrorSet`
   redirecting `registry.redhat.io/openshift-service-mesh` → that mirror. Keeps the
   native `openshift-default` GatewayClass. Note: an IDMS change rolls worker nodes,
   and the CIO pins more OSSM images (proxy/gateway) as Gateways are created - mirror
   the whole `openshift-service-mesh` repo, not just `istio-pilot-rhel9`.
2. **Open an IBM ticket** to sync OSSM images into `armada-extensions` or allowlist
   worker egress to `registry.redhat.io`. Correct long-term, not same-day.

Only once `istiod` is Running and `openshift-default` is `Accepted=True` proceed to
`cloud-hub-ingress-bootstrap`.

#### Tracked: OSSM images that must be mirrored (built-in Gateway API, OCP 4.21.27)

The complete image set the CIO references for the `openshift-default` Gateway API
(discovered from the `istiod` deployment + the `istio-sidecar-injector-openshift-gateway`
configmap on hysh-ibm-01, 2026-08-15):

| Purpose | Source image (pin by digest) |
|---------|------------------------------|
| istiod (control plane) | `registry.redhat.io/openshift-service-mesh/istio-pilot-rhel9@sha256:2a25d47b4bb3bf346563a0ccea986c0ab0466709ca4cb9d2666ba6a02a8a5f31` |
| istio-proxy (gateway data plane) | `registry.redhat.io/openshift-service-mesh/istio-proxyv2-rhel9@sha256:2b5f5aa5ee9974269d8e3666b1bfc58da10c172bf3f1d6defd555fbd1ac9a6ec` |

(`busybox:1.28` appears only in an inert sample template in the injector configmap,
not the live injection - no need to mirror it.) Digests are OCP-version-specific;
re-discover them after any cluster upgrade.

#### Mirror target: node egress is restricted to IBM registries only

Workers can reach `us.icr.io` (HTTP 401 in ~7ms) but **not** `registry.redhat.io`
(egress timeout) - and public registries like `quay.io` are equally out of reach.
So the mirror must live on a **node-reachable** registry: IBM Container Registry
(`icr.io`) or the cluster's own internal registry route.

- **IBM Container Registry (`icr.io`) - preferred, but needs IAM.** Nodes already
  carry the `all-icr-io` pull secret, so no node cred changes are needed. BUT on this
  account the CLI identity **could not create a namespace** (`ibmcloud cr namespace-add`
  → "not authorized" - same IAM-permission class as the COS s2s failure). Have an
  account admin grant **Container Registry Manager** (or pre-create a namespace and
  grant Writer), then:
  ```bash
  ibmcloud plugin install container-registry -f
  ibmcloud cr region-set us-south            # -> us.icr.io (workers reach this)
  ibmcloud cr namespace-add hypershell
  ibmcloud cr login                          # configures podman/docker
  # source creds: extract registry.redhat.io auth from the cluster pull-secret
  oc get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d > /tmp/rh-auth.json
  for D in \
    istio-pilot-rhel9@sha256:2a25d47b4bb3bf346563a0ccea986c0ab0466709ca4cb9d2666ba6a02a8a5f31 \
    istio-proxyv2-rhel9@sha256:2b5f5aa5ee9974269d8e3666b1bfc58da10c172bf3f1d6defd555fbd1ac9a6ec; do
    skopeo copy --authfile /tmp/rh-auth.json \
      docker://registry.redhat.io/openshift-service-mesh/$D \
      docker://us.icr.io/hypershell/${D%@*}@${D#*@}
  done
  ```
  Then the IDMS below with mirror `us.icr.io/hypershell`.

- **Internal registry route - self-service fallback (no IAM), more moving parts.**
  Enable `defaultRoute`, push the two images to `<route>/openshift-ingress/...`, add the
  route host's puller creds to the **global** `openshift-config/pull-secret` (so node
  crio can authenticate - the internal `.svc` address is NOT resolvable by node crio,
  the public route host is), then IDMS mirror to `<route-host>/openshift-ingress`.
  Editing the global pull secret + adding an IDMS both **roll the worker nodes**.

#### ImageDigestMirrorSet (redirect the OSSM repo to the mirror)

```yaml
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: ossm-gateway-mirror
spec:
  imageDigestMirrors:
    - source: registry.redhat.io/openshift-service-mesh
      mirrors:
        - us.icr.io/hypershell            # or <internal-registry-route>/openshift-ingress
```

#### BLOCKER: you cannot create an IDMS on ROKS (HyperShift-hosted)

ROKS runs a **HyperShift-hosted** control plane (`oc get nodes` shows only workers;
the control plane lives in IBM's management cluster). A `ValidatingAdmissionPolicy`
named `mirror` **denies** creating `imagedigestmirrorsets`/`imagetagmirrorsets` in the
guest:

```
ValidatingAdmissionPolicy 'mirror' ... denied request: This resource cannot be
created, updated, or deleted. Please ask your administrator to modify the resource
in the HostedCluster object.
```

So node-level image mirroring (`registries.conf`) is IBM-owned and can only be changed
on the HostedCluster (that is also why IBM's own `registry.redhat.io → armada` mirror
exists but you can't add one). **The internal-registry + IDMS plan dead-ends here** on
ROKS. Confirmed 2026-08-15 on hysh-ibm-01. The mirrored images (above) are still valid
and reusable for whichever path wins.

#### Revised options once IDMS is off the table (ROKS)

1. **IBM ticket (only *supported* fix).** Ask IBM to either sync the OSSM images into
   `armada-extensions`, or add an `imageContentSource`/IDMS to the **HostedCluster**
   pointing `registry.redhat.io/openshift-service-mesh` at a reachable mirror, or
   allowlist worker egress to `registry.redhat.io`. Then the native `openshift-default`
   path "just works." Not same-day.
2. **Guest-side Sail/OSSM 3 with image overrides (works today, diverges from CIO).**
   Install the Sail (`sailoperator`) or OSSM 3 (`servicemeshoperator3`) operator from
   OperatorHub, then in the `Istio` CR set `spec.values.pilot.image` and
   `spec.values.global.proxy.image` to the **mirrored** pullspecs on the internal
   registry route (that host is already in the global pull-secret and is node-resolvable - no IDMS needed). Create a GatewayClass named `openshift-default` whose
   `controllerName` matches the Sail controller so the shared-Gateway manifests stay
   byte-identical to AWS. Trade-off: Sail-managed istiod instead of CIO-managed.
3. **Route-based tenant ingress on IBM (IMPLEMENTED - the chosen path).** Skip
   Gateway API on ROKS; the control plane emits passthrough `Route`s on IBM's free
   `*.containers.appdomain.cloud` wildcard. This is now a first-class,
   config-selected ingress mode (`GATEWAY_INGRESS_MODE=route`), delivered by the
   `deploy/ibm` kustomize overlay - no shared Gateway, wildcard cert, cert-manager
   ClusterIssuer, or Route53 required. **Use this for ROKS.** See "Step 5" and
   [`global-architecture.spec.md`](../../../specs/platform/global-architecture.spec.md)
   (§ "IBM Cloud Cloud Hub - Route ingress mode").

## Step 5: Hand off (Route ingress mode)

ROKS uses the **Route ingress mode**, not Gateway API. Do **not** run
`cloud-hub-ingress-bootstrap` (that bootstraps the shared Gateway for
`gateway-api` mode on AWS/functional clusters).

1. `deploy-cluster` with the **`deploy/ibm` overlay** - platform services
   (API/controller/PostgreSQL) plus `GATEWAY_INGRESS_MODE=route`. Use its
   Cloud-Hub Parameter Overrides (registry host, storage class `ibmc-vpc-block-*`),
   and set `GATEWAY_API_BASE_DOMAIN` to this cluster's ingress subdomain:

   ```bash
   oc get ingresscontroller default -n openshift-ingress-operator \
     -o jsonpath='{.status.domain}{"\n"}'   # -> *.<subdomain>.containers.appdomain.cloud
   ```

2. **Provision a test tenant gateway** with `route.enabled=true`. The control
   plane creates a passthrough `Route` `openshell-gateway` in the tenant namespace
   (host `gw-<tenant>.<base-domain>`) and publishes `grpcs://<host>:443`.

3. **Verify** the Route is admitted and the CLI connects:

   ```bash
   NS=$(oc get ns -l hypershell.redhat.io/managed=true -o name | head -1 | cut -d/ -f2)
   oc -n "$NS" get route openshell-gateway \
     -o jsonpath='{.status.ingress[0].conditions[0].type}={.status.ingress[0].conditions[0].status}{"\n"}'  # Admitted=True
   openshell login "gw-${NS}.<base-domain>:443"
   ```

   The gateway pod's server cert SANs (`ServerDnsNames`/`ExternalDns`) must
   include `gw-<tenant>.<base-domain>` for passthrough TLS to validate.

To later switch to Gateway API (if IBM fixes HostedCluster mirroring), unset
`GATEWAY_INGRESS_MODE` and run `cloud-hub-ingress-bootstrap`; the control plane
then emits `GRPCRoute`s and removes the Routes.

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
