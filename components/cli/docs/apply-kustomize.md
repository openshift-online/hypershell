# hsctl apply -k: Kustomize Support

## Overview

The `hsctl apply -k` command builds and applies HyperShell resources from a Kustomize directory, providing a declarative way to manage gateway infrastructure across multiple environments.

## Usage

```bash
hsctl apply -k <directory>
```

The `-k` flag and `-f` flag are mutually exclusive.

## Security Features

The implementation includes the following security constraints:

1. **No Arbitrary Plugin Execution**: The command runs kustomize with `--enable-alpha-plugins=false` to prevent execution of arbitrary plugins.

2. **Load Restrictions**: Uses `--load-restrictor=LoadRestrictionsRootOnly` to restrict file loading to the kustomize root directory.

3. **No Secret Leakage**: The command does not print secret values. Only resource status (created/configured/unchanged) is displayed.

## Supported Kinds

- `Gateway`
- `GatewayNetwork`
- `GatewayRelease`
- `ManagedCluster`
- `Role`
- `RoleBinding`

Resources with unsupported kinds are skipped with a warning.

## Examples

### Basic Base Configuration

Directory structure:
```
.hypershell/
├── base/
│   ├── kustomization.yaml
│   ├── gateway.yaml
│   ├── cluster.yaml
│   └── release.yaml
```

Apply the base:
```bash
hsctl apply -k .hypershell/base/
```

### Overlays for Different Environments

Directory structure:
```
.hypershell/
├── base/
│   ├── kustomization.yaml
│   └── resources...
└── overlays/
    ├── dev/
    │   ├── kustomization.yaml
    │   └── patches...
    ├── staging/
    │   ├── kustomization.yaml
    │   └── patches...
    └── prod/
        ├── kustomization.yaml
        └── patches...
```

Apply production overlay:
```bash
hsctl apply -k .hypershell/overlays/prod/
```

### Complete Example

**base/kustomization.yaml:**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - gateway.yaml
  - cluster.yaml
  - release.yaml
```

**base/gateway.yaml:**
```yaml
apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: api-gateway
  description: Main API Gateway
spec:
  cluster_id: eks-us-east-1
  release_id: v1.0.0
  tls_mode: enabled
  service_type: LoadBalancer
  external_dns: api.example.com
```

**overlays/prod/kustomization.yaml:**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

patches:
  - path: gateway-patch.yaml
    target:
      kind: Gateway
      name: api-gateway
```

**overlays/prod/gateway-patch.yaml:**
```yaml
apiVersion: hypershell.redhat.io/v1
kind: Gateway
metadata:
  name: api-gateway
  description: Production API Gateway
spec:
  external_dns: api.prod.example.com
```

## Error Handling

The command provides clear error messages for common issues:

### Missing Directory
```bash
$ hsctl apply -k /nonexistent
Error: kustomize directory not found: /nonexistent
```

### Missing kustomization.yaml
```bash
$ hsctl apply -k ./empty-dir/
Error: no kustomization.yaml or kustomization.yml found in ./empty-dir/
```

### Invalid Kustomize Content
```bash
$ hsctl apply -k ./bad-kustomize/
Error: kustomize build failed: <detailed error from kustomize>
```

Build and validation errors identify source files and return non-zero exit codes.

## Flags

| Flag | Description |
|------|-------------|
| `-k <dir>` | Kustomize directory to build and apply |
| `--dry-run` | Print what would be applied without making changes |
| `-o json` | Output results as JSON |

## Reconciliation Behavior

The `-k` flag uses the same reconciliation semantics as `-f`:

- If a resource with the same name exists, it is updated (PATCH)
- If a resource does not exist, it is created (POST)
- Resources are applied in the order they appear in the kustomize output

## Output

Default output shows one line per resource:

```
managedcluster/eks-us-east-1 created
gatewayrelease/v1.0.0 created
gateway/api-gateway created
```

JSON output (`-o json`) provides detailed results:

```json
[
  {
    "kind": "ManagedCluster",
    "name": "eks-us-east-1",
    "status": "created"
  },
  {
    "kind": "GatewayRelease",
    "name": "v1.0.0",
    "status": "created"
  },
  {
    "kind": "Gateway",
    "name": "api-gateway",
    "status": "created"
  }
]
```

## Requirements

- `kustomize` binary must be installed and available in PATH
- Install from: https://kustomize.io/

## Testing

The implementation includes comprehensive tests covering:

- Valid overlays with patches
- Missing directories
- Invalid kustomize content
- Mixed resource kinds
- Security (no plugin execution)

Run tests:
```bash
cd components/cli
go test ./cmd/hsctl/apply/... -v
```

## Implementation Details

### Build Process

1. Verify the directory exists and contains `kustomization.yaml` or `kustomization.yml`
2. Execute `kustomize build` with security flags:
   - `--enable-alpha-plugins=false`
   - `--load-restrictor=LoadRestrictionsRootOnly`
3. Parse the YAML stream output
4. Validate each resource kind is supported
5. Apply each resource using the same reconciliation logic as `-f`

### Deterministic Output

The kustomize build produces a deterministic resource stream:
- Resources are ordered consistently
- The same input always produces the same output
- No side effects or state dependencies

### Error Propagation

- Kustomize build errors include stderr output for debugging
- Individual resource application errors are reported but don't stop processing
- The command returns non-zero if any resource fails to apply
- Source file information is preserved in error messages where available
