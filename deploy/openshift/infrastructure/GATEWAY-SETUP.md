# Gateway Setup with cert-manager

The control plane requires a pre-existing Gateway resource. This guide
documents how to set one up on OpenShift with cert-manager for automatic
wildcard TLS certificate management.

## Prerequisites

- OpenShift cluster with the Gateway API controller enabled
- cert-manager installed (or available as a cluster add-on)
- DNS zone access for your domain

## 1. Create the GatewayClass

Apply the GatewayClass if it does not already exist:

```shell
oc apply -f gatewayclass.yaml
```

## 2. Create a ClusterIssuer

Create a cert-manager `ClusterIssuer` for your domain. This example uses
Let's Encrypt with DNS-01 validation via Route53:

```shell
oc apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-dns
spec:
  acme:
    email: admin@example.com
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - dns01:
          route53:
            region: us-east-1
            accessKeyIDSecretRef:
              name: route53-credentials
              key: aws_access_key_id
            secretAccessKeySecretRef:
              name: route53-credentials
              key: aws_secret_access_key
        selector:
          dnsZones:
            - example.com
EOF
```

Verify the issuer is ready:

```shell
oc get clusterissuer letsencrypt-dns
```

## 3. Issue a wildcard certificate

Create a `Certificate` in the Gateway's namespace (`openshift-ingress`) so
cert-manager issues a wildcard cert and stores it in a Secret:

```shell
oc apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: wildcard-openshell
  namespace: openshift-ingress
spec:
  secretName: wildcard-openshell-tls
  issuerRef:
    name: letsencrypt-dns
    kind: ClusterIssuer
  dnsNames:
    - "*.openshell.example.com"
EOF
```

Wait for the certificate to be issued:

```shell
oc get certificate wildcard-openshell -n openshift-ingress -w
```

The `READY` column should show `True` once the ACME DNS challenge completes.

## 4. Create the Gateway

Update `gateway.yaml` with your wildcard hostname and certificate secret
name, then apply:

```shell
oc apply -f gateway.yaml
```

Verify the Gateway is programmed:

```shell
oc get gateway openshell-grpc-gateway -n openshift-ingress
```

The `PROGRAMMED` column should show `True` and an `ADDRESS` should be assigned.

## 5. Set up DNS

Create a DNS wildcard record pointing to the Gateway's external address:

```shell
# Get the Gateway's load balancer address
oc get gateway openshell-grpc-gateway -n openshift-ingress \
  -o jsonpath='{.status.addresses[0].value}'
```

Create a CNAME record:
```
*.openshell.example.com → <gateway-lb-address>
```

Each tenant gets a unique hostname under the wildcard
(e.g. `gw-<namespace>.openshell.example.com`).

## 6. Configure the control plane

Set these environment variables on the controller deployment:

| Variable | Value | Purpose |
|----------|-------|---------|
| `GATEWAY_API_GATEWAY_NAME` | `openshell-grpc-gateway` | Name of the pre-existing Gateway |
| `GATEWAY_API_GATEWAY_NAMESPACE` | `openshift-ingress` | Namespace where the Gateway lives |
| `GATEWAY_API_BASE_DOMAIN` | `openshell.example.com` | Base domain for tenant hostnames |

The control plane derives per-tenant hostnames as
`gw-<namespace>.<base-domain>` and creates GRPCRoutes that attach to the
shared Gateway.

## Certificate renewal

cert-manager handles renewal automatically. The default renewal window is
30 days before expiry. No manual intervention is required.

## Troubleshooting

**Certificate not ready**: Check the cert-manager logs and the Certificate's
status conditions:
```shell
oc describe certificate wildcard-openshell -n openshift-ingress
oc logs -n cert-manager deploy/cert-manager
```

**Gateway not programmed**: Check the Gateway status and events:
```shell
oc describe gateway openshell-grpc-gateway -n openshift-ingress
oc get events -n openshift-ingress --field-selector involvedObject.name=openshell-grpc-gateway-openshift-default
```

**Wrong TLS certificate served**: Verify DNS resolves to the Gateway's LB
address (not the OpenShift default router):
```shell
dig +short my-tenant.openshell.example.com
oc get svc openshell-grpc-gateway-openshift-default -n openshift-ingress \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
```
