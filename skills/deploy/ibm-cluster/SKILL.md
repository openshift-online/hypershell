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
which blocks in-place upgrades — so getting a newer version means a new cluster,
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
actually end up COS-backed on this account — see "Internal registry storage" below
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
(~30–60 min) and incurs cost — confirm before running.

A `Ece8a: Could not create a bucket in your cloud object storage instance` warning
is expected here and is **not** blocking — the registry falls back to `emptyDir`.
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
ibmcloud ks cluster config --cluster hysh-ibm-01
oc get clusterversion version -o jsonpath='{.status.desired.version}{"\n"}'   # expect 4.21.x
oc get gatewayclass                                                          # expect openshift-default
oc get crd | grep gateway.networking.k8s.io                                  # gateways/grpcroutes present
```

If `openshift-default` GatewayClass is present, no OSSM 3 / Sail install is needed —
proceed straight to `cloud-hub-ingress-bootstrap`.

## Step 5: Hand off

1. `deploy-cluster` — platform services (API/controller/PostgreSQL). Use its
   Cloud-Hub Parameter Overrides (registry host, storage class `ibmc-vpc-block-*`).
2. `cloud-hub-ingress-bootstrap` — shared Gateway + wildcard DNS/TLS.

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
blocking** — the registry falls back to `emptyDir` and image pushes for
`deploy-cluster` still work (that is how the reference cluster runs today). Pick a
registry storage backend deliberately:

| Option | Persistence | Setup | When |
|--------|-------------|-------|------|
| `emptyDir` (default fallback) | ephemeral — images lost if the registry pod restarts | none | matches reference; fine for demo/dev |
| **PVC (`ibmc-vpc-block-*`)** | persistent | patch registry `spec.storage.pvc` (RWO, single replica) | **recommended** for a persistent Cloud Hub; no COS/IAM dependency |
| COS-backed | persistent | needs a Kubernetes Service → COS IAM authorization | only if you want object-store backing |

PVC backend (recommended, self-contained) — **chosen for `hysh-ibm-01`**:

```bash
oc patch configs.imageregistry.operator.openshift.io cluster --type merge -p \
  '{"spec":{"storage":{"pvc":{"claim":""}},"rolloutStrategy":"Recreate","replicas":1}}'
# creates image-registry-storage PVC on the default ibmc-vpc-block storage class
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
visible `iam user-policies` — i.e. it lacks authorization-policy rights. If you
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
