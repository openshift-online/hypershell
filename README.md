<h1 align="center"><sup><img src="images/brand/logo.png" alt="HyperShell logo" width="80" align="middle"></sup>&nbsp;HyperShell</h1>

HyperShell provisions and manages [OpenShell](https://github.com/NVIDIA/OpenShell) gateways at scale and across clouds.

## Kubernetes Prerequisites

The control plane requires the following resources to be present on the target cluster before it can fully reconcile gateways.

### Agent Sandbox controller

The [Agent Sandbox controller](https://github.com/kubernetes-sigs/agent-sandbox) (`agents.x-k8s.io`) must be installed on the cluster. Gateway pods manage sandboxes via the `Sandbox` custom resource, and the provisioned RBAC grants permissions on `agents.x-k8s.io/sandboxes`. NetworkPolicies also reference sandbox labels (`agents.x-k8s.io/sandbox-name-hash`) for pod-level traffic control.

### cert-manager

[cert-manager](https://cert-manager.io/) must be installed for automatic TLS certificate provisioning. The control plane auto-detects cert-manager at startup. If absent, TLS certificates must be provisioned manually via the certgen job.

```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```

### Gateway API CRDs

The [Gateway API](https://gateway-api.sigs.k8s.io/) CRDs must be installed if external routing via GRPCRoute is desired. The control plane auto-detects Gateway API availability at startup and skips route provisioning if the CRDs are not present.

```shell
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/latest/download/standard-install.yaml
```

### GatewayClass

A GatewayClass must exist on the cluster for per-tenant Gateway resources to reference. On OpenShift, the `openshift-default` GatewayClass is provided automatically. On other clusters (e.g., Kind with [cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind)), install the appropriate GatewayClass for your ingress controller.

The GatewayClass name is configurable via the `GATEWAY_API_GATEWAY_CLASS` environment variable (default: `openshift-default`).

### TLS certificate Secret (`grpc-gateway-certs`)

A wildcard TLS Secret named `grpc-gateway-certs` must be present in the control plane namespace. The control plane copies this Secret into each tenant namespace when provisioning a gateway. The per-tenant Gateway API Gateway resource references it for HTTPS termination.

The Secret must contain `tls.crt` and `tls.key` entries covering all tenant hostnames under the base domain (e.g., `*.hsgw.<base-domain>`).

```shell
kubectl -n hypershell create secret tls grpc-gateway-certs \
  --cert=/path/to/wildcard.crt \
  --key=/path/to/wildcard.key
```

### Trusted CA bundle (optional)

If the cluster uses a private CA, create a ConfigMap named `gateway-trusted-ca` in the control plane namespace (default: `hypershell`). The control plane copies this ConfigMap into each tenant namespace and mounts it into gateway pods so they trust the cluster's internal certificates.

```shell
kubectl -n hypershell create configmap gateway-trusted-ca --from-file=ca-bundle.crt=/path/to/ca.crt
```

### Control plane environment variables

| Variable | Default | Description |
|---|---|---|
| `HYPERSHELL_GRPC_SERVER_ADDR` | `localhost:9000` | gRPC address of the API server |
| `HYPERSHELL_API_SERVER_URL` | `http://localhost:8000` | HTTP address of the API server |
| `HYPERSHELL_NAMESPACE` | `hypershell` | Namespace the control plane runs in (used for trusted CA bundle source) |
| `OPENSHELL_GATEWAY_IMAGE` | `ghcr.io/nvidia/openshell/gateway:0.0.92` | Default gateway container image |
| `GATEWAY_API_GATEWAY_CLASS` | `openshift-default` | GatewayClass name for per-tenant Gateway resources |
| `GATEWAY_API_BASE_DOMAIN` | *(none)* | Base domain for auto-derived hostnames (e.g., `apps.cluster.example.com`) |
| `GATEWAY_MANIFESTS_DIR` | `/manifests/gateway` | Path to gateway manifest templates |
