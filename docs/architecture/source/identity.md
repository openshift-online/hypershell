# Identity and access

OIDC is the production authentication mechanism. Keycloak federation carries identity down the tiers, while the gateway validates issuer, audience, signature, expiry, and roles.

Authoritative source: specs/platform/global-architecture.spec.md; specs/platform/openshell-gateway-oidc.spec.md; specs/platform/openshell-gateway-keycloak.spec.md

### Interactive CLI authentication follows the federated Keycloak chain; tokens are issued by the ManagedCluster Keycloak client for that Gateway.

```mermaid
sequenceDiagram
  participant U as openshell CLI user
  participant G as OpenShell Gateway
  participant MC as ManagedCluster Keycloak
  participant CH as Cloud Hub Keycloak
  participant GH as Global Keycloak
  participant RH as Red Hat SSO
  U->>G: Connect to gateway
  G-->>U: OIDC login redirect
  U->>MC: Authenticate
  MC->>CH: Federate authentication
  CH->>GH: Federate authentication
  GH->>RH: Federate authentication
  RH-->>GH: Auth response
  GH-->>CH: Auth response
  CH-->>MC: Auth response
  MC-->>U: Issue gateway-scoped token
  U->>G: Bearer token request
  G->>MC: Validate JWT via issuer/JWKS
  MC-->>G: Valid issuer, audience, roles
  G-->>U: Authorized gateway operation
```

### The control plane provisions per-Gateway Keycloak clients and bridges HyperShell RBAC to gateway roles.

```mermaid
graph LR
  API[Gateway resource<br/>RBAC RoleBindings] --> CP[Control Plane]
  CP -->|internal gRPC| PROV[Service-account<br/>provisioner]
  PROV --> KC[Keycloak<br/>per-gateway client]
  KC --> CLAIM[hypershell.roles<br/>openshell-admin/user]
  CLAIM --> TOML[gateway.toml<br/>OIDC config]
  TOML --> GW[Gateway JWT validation]
  SA[OpenShellGatewayServiceAccount] -->|client credentials| KC
  SA -->|short-lived token| GW
  style API fill:#fff3cd
  style CP fill:#f8d7da
  style KC fill:#cce5ff
  style GW fill:#d4edda
```
