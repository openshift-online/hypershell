# Spec ID Registry

Machine-readable registry of every `MODULE-GROUP` prefix in `specs/`, per
`specs/standards/platform/spec-identifiers.spec.md`. One row per prefix, one
owning file (except the three multi-group files noted below). A new module
or group is added here before any requirement uses it.

## Active Prefixes

### ADO

| Prefix | File | Requirements |
|--------|------|--------------|
| `ADO-DASH` | `web-console/adoption-dashboard.spec.md` | 10 |

### AGT

| Prefix | File | Requirements |
|--------|------|--------------|
| `AGT-EXT` | `platform/extension-api-openshell-api-mapping.spec.md` | 5 |
| `AGT-INF` | `platform/openshell-inference-routing.spec.md` | 3 |
| `AGT-SEC` | `platform/agent-secret-late-binding.spec.md` | 13 |
| `AGT-WSP` | `platform/agent-workspace-gateway-api.spec.md` | 10 |

### API

| Prefix | File | Requirements |
|--------|------|--------------|
| `API-OBS` | `platform/api-server-observability.spec.md` | 8 |

### CICD

| Prefix | File | Requirements |
|--------|------|--------------|
| `CICD-BLD` | `platform/openshell-branch-build.spec.md` | 8 |
| `CICD-CRED` | `platform/ephemeral-test-credentials.spec.md` | 12 |
| `CICD-MRG` | `platform/mergify-parallel-merge-queue.spec.md` | 12 |
| `CICD-PREN` | `platform/ephemeral-pr-environments.spec.md` | 12 |
| `CICD-SEC` | `platform/ephemeral-ci-secrets.spec.md` | 5 |

### CLI

| Prefix | File | Requirements |
|--------|------|--------------|
| `CLI-EXT` | `platform/hsctl-extension-commands.spec.md` | 4 |
| `CLI-GWC` | `platform/hsctl-gateway-connection.spec.md` | 2 |
| `CLI-REL` | `platform/hsctl-release.spec.md` | 6 |
| `CLI-TUI` | `platform/hsctl-terminal-ui.spec.md` | 4 |

### CTRL

| Prefix | File | Requirements |
|--------|------|--------------|
| `CTRL-CORE` | `platform/control-plane.spec.md` | 14 |
| `CTRL-TLS` | `platform/hub-grpc-tls.spec.md` | 5 |
| `CTRL-TRC` | `platform/reconcile-trace-correlation.spec.md` | 5 |
| `CTRL-WDEL` | `platform/watch-delete-events.spec.md` | 3 |

### DEV

| Prefix | File | Requirements |
|--------|------|--------------|
| `DEV-LOC` | `platform/local-development.spec.md` | 17 |
| `DEV-OCP` | `platform/openshift-development.spec.md` | 14 |

### E2E

| Prefix | File | Requirements |
|--------|------|--------------|
| `E2E-CORE` | `platform/e2e-testing.spec.md` | 46 |
| `E2E-COV` | `platform/e2e-feature-coverage.spec.md` | 7 |
| `E2E-LOG` | `platform/e2e-run-log-visibility.spec.md` | 18 |

### FLT

| Prefix | File | Requirements |
|--------|------|--------------|
| `FLT-CAP` | `platform/fleet-dashboard-capability-chips.spec.md` | 6 |
| `FLT-MAP` | `platform/fleet-dashboard-map-hub-spine-layout.spec.md` | 7 |
| `FLT-NAV` | `platform/glass-url-addressable-navigation.spec.md` | 21 |
| `FLT-SYNC` | `platform/fleet-dashboard-sync-retry-visibility.spec.md` | 4 |

### GTW

| Prefix | File | Requirements |
|--------|------|--------------|
| `GTW-ACC` | `platform/gateway-access-management.spec.md` | 14 |
| `GTW-ATTR` | `platform/gateway-managed-cluster-attribution.spec.md` | 6 |
| `GTW-CFG` | `platform/generated-gateway-config-validation.spec.md` | 4 |
| `GTW-CONC` | `platform/gateway-reconcile-concurrency.spec.md` | 3 |
| `GTW-DEL` | `platform/gateway-deletion-finalization.spec.md` | 8 |
| `GTW-PHS` | `platform/gateway-phase-vocabulary.spec.md` | 5 |
| `GTW-PROG` | `platform/gateway-provisioning-progress.spec.md` | 5 |

### OBS

| Prefix | File | Requirements |
|--------|------|--------------|
| `OBS-CPL` | `platform/control-plane-observability.spec.md` | 8 |
| `OBS-CPU` | `platform/cluster-cpu.spec.md` | 8 |
| `OBS-CRM` | `platform/control-plane-reconciliation-metrics.spec.md` | 13 |
| `OBS-GFT` | `platform/gateway-fleet-total-trend.spec.md` | 9 |
| `OBS-GMD` | `platform/gateway-metrics-dashboard.spec.md` | 8 |
| `OBS-GPO` | `platform/gateway-provision-outcomes.spec.md` | 9 |
| `OBS-GPT` | `platform/gateway-provision-time.spec.md` | 9 |
| `OBS-HCUT` | `platform/hub-cluster-utilization-trends.spec.md` | 10 |
| `OBS-MEM` | `platform/cluster-memory.spec.md` | 8 |
| `OBS-NODE` | `platform/cluster-nodes.spec.md` | 8 |
| `OBS-POD` | `platform/cluster-pods.spec.md` | 8 |
| `OBS-REL` | `platform/api-reliability-metrics.spec.md` | 8 |
| `OBS-RLSD` | `platform/gateway-release-distribution.spec.md` | 7 |
| `OBS-SBXA` | `platform/gateway-sandbox-attention-status.spec.md` | 11 |
| `OBS-SBXT` | `platform/gateway-sandbox-active-trends.spec.md` | 9 |

### OSG

| Prefix | File | Requirements |
|--------|------|--------------|
| `OSG-CON` | `platform/openshell-gateway-console.spec.md` | 9 |
| `OSG-CORE` | `platform/openshell-gateway.spec.md` | 18 |
| `OSG-CRED` | `platform/openshell-gateway-credentials.spec.md` | 12 |
| `OSG-DBS` | `platform/openshell-gateway-database.spec.md` | 12 |
| `OSG-HELM` | `platform/openshell-gateway-helm-adoption.spec.md` | 4 |
| `OSG-HLTH` | `platform/openshell-gateway-health.spec.md` | 6 |
| `OSG-KCLK` | `platform/openshell-gateway-keycloak.spec.md` | 11 |
| `OSG-NSGC` | `platform/openshell-gateway-namespace-gc.spec.md` | 8 |
| `OSG-OIDC` | `platform/openshell-gateway-oidc.spec.md` | 4 |
| `OSG-ROT` | `platform/openshell-gateway-secret-rotation.spec.md` | 4 |
| `OSG-ROUT` | `platform/openshell-gateway-routing.spec.md` | 11 |
| `OSG-SACC` | `platform/openshell-gateway-service-accounts.spec.md` | 17 |
| `OSG-SBX` | `platform/openshell-gateway-sandbox-count.spec.md` | 7 |
| `OSG-SEVT` | `platform/gateway-sandbox-events.spec.md` | 6 |
| `OSG-TLS` | `platform/openshell-gateway-tls.spec.md` | 5 |

### PLAT

| Prefix | File | Requirements |
|--------|------|--------------|
| `PLAT-ARCH` | `platform/global-architecture.spec.md` | 19 |
| `PLAT-CLUS` | `platform/managed-cluster-registration.spec.md` | 13 |
| `PLAT-DATA` | `platform/data-model.spec.md` | 22 |
| `PLAT-INV` | `platform/platform-inventory.spec.md` | 10 |
| `PLAT-USR` | `platform/registered-users.spec.md` | 15 |

### REL

| Prefix | File | Requirements |
|--------|------|--------------|
| `REL-DASH` | `web-console/reliability-dashboard.spec.md` | 14 |

### SDLC

| Prefix | File | Requirements |
|--------|------|--------------|
| `SDLC-JIRA` | `standards/sdlc/jira-github-sync.spec.md` | 13 |
| `SDLC-VIS` | `platform/agent-sdlc-visibility-dashboard.spec.md` | 23 |

### SEC

| Prefix | File | Requirements |
|--------|------|--------------|
| `SEC-OIDC` | `platform/oidc-integration.spec.md` | 11 |
| `SEC-RBAC` | `security/rbac-enforcement.spec.md` | 17 |
| `SEC-STD` | `standards/security/security.spec.md` | 4 |

### STD

| Prefix | File | Requirements |
|--------|------|--------------|
| `STD-IDS` | `standards/platform/spec-identifiers.spec.md` | 8 |

### UIS

| Prefix | File | Requirements |
|--------|------|--------------|
| `UIS-A11Y` | `standards/ui/accessibility.spec.md` | 10 |
| `UIS-BRND` | `standards/ui/brand-color.spec.md` | 6 |
| `UIS-CONT` | `standards/ui/content-localization.spec.md` | 5 |
| `UIS-FND` | `standards/ui/foundations.spec.md` | 10 |
| `UIS-HEX` | `standards/ui/hexagonal-architecture.spec.md` | 9 |
| `UIS-I18N` | `standards/ui/content-localization.spec.md` | 5 |
| `UIS-INT` | `standards/ui/interaction.spec.md` | 10 |
| `UIS-OBS` | `standards/ui/domain-observability.spec.md` | 11 |
| `UIS-PERF` | `standards/ui/trust-performance.spec.md` | 5 |
| `UIS-PFLY` | `standards/ui/patternfly.spec.md` | 10 |
| `UIS-TRST` | `standards/ui/trust-performance.spec.md` | 5 |
| `UIS-VER` | `standards/ui/verification.spec.md` | 10 |

### WEB

`web-console/architecture.spec.md` is one of the three files that host more
than one group (see "Multi-group files" below); its groups are listed with
the rest of `WEB` here for lookup convenience.

| Prefix | File | Requirements |
|--------|------|--------------|
| `WEB-ACC` | `web-console/gateway-access-management.spec.md` | 11 |
| `WEB-API` | `web-console/architecture.spec.md` | 1 |
| `WEB-ARCH` | `web-console/architecture.spec.md` | 3 |
| `WEB-AUTH` | `web-console/architecture.spec.md` | 4 |
| `WEB-BFF` | `web-console/architecture.spec.md` | 1 |
| `WEB-DATA` | `web-console/architecture.spec.md` | 4 |
| `WEB-DOCS` | `platform/openshell-install-docs-link.spec.md` | 2 |
| `WEB-DPLY` | `web-console/architecture.spec.md` | 2 |
| `WEB-HOME` | `web-console/homepage.spec.md` | 16 |
| `WEB-I18N` | `web-console/architecture.spec.md` | 1 |
| `WEB-OBS` | `web-console/architecture.spec.md` | 2 |
| `WEB-OPS` | `web-console/operational-dashboard.spec.md` | 30 |
| `WEB-PKG` | `web-console/architecture.spec.md` | 4 |
| `WEB-PLC` | `web-console/gateway-provision-placement.spec.md` | 11 |
| `WEB-QUAL` | `web-console/architecture.spec.md` | 3 |
| `WEB-SDK` | `web-console/architecture.spec.md` | 1 |
| `WEB-SEC` | `web-console/architecture.spec.md` | 1 |
| `WEB-TRC` | `web-console/tracing.spec.md` | 11 |
| `WEB-UIEX` | `web-console/architecture.spec.md` | 6 |
| `WEB-USE` | `web-console/usage-dashboard.spec.md` | 11 |
| `WEB-VER` | `web-console/gateway-version-visibility.spec.md` | 5 |

## Multi-Group Files

These three files host more than one group under one module, by precedent
predating this registry (`specs/standards/platform/spec-identifiers.spec.md`,
Decision 1). Each still owns its groups exclusively; no other file may add
to `UIS-CONT`, `UIS-PERF`, or any `WEB-*` group owned by `architecture.spec.md`.

| File | Groups |
|------|--------|
| `standards/ui/content-localization.spec.md` | `UIS-CONT`, `UIS-I18N` |
| `standards/ui/trust-performance.spec.md` | `UIS-TRST`, `UIS-PERF` |
| `web-console/architecture.spec.md` | `WEB-ARCH`, `WEB-PKG`, `WEB-SDK`, `WEB-AUTH`, `WEB-BFF`, `WEB-DATA`, `WEB-UIEX`, `WEB-I18N`, `WEB-QUAL`, `WEB-DPLY`, `WEB-SEC`, `WEB-OBS`, `WEB-API` |

## Exempt (No Numbered Requirements)

These files are narrative or reference documents with no
`### Requirement` heading: no testable Given/When/Then contract to number.
Each still reserves a prefix so a future requirement in that file starts
from a registered, collision-free code instead of inventing one ad hoc.

| Prefix | File | Reason |
|--------|------|--------|
| (none) | `index.spec.md` | The spec tree's own index; hosts this registry pointer, not requirements |
| `AGT-IMG` | `platform/agent-runtime-images.spec.md` | Image/pipeline description, no Given/When/Then contracts yet |
| `E2E-CON` | `platform/e2e-console-browser-testing.spec.md` | Driver/config description, no Given/When/Then contracts yet |
| `CICD-IMG` | `platform/openshell-image-auto-update.spec.md` | Renovate pipeline description, no Given/When/Then contracts yet |
| `STD-CTRL` | `standards/control-plane/conventions.spec.md` | Coding conventions reference, not behavior contracts |
| `STD-XCUT` | `standards/platform/cross-cutting.spec.md` | Cross-cutting constraints reference, not behavior contracts |
| `STD-NAME` | `standards/platform/naming-multitenancy.spec.md` | Naming-convention reference, not behavior contracts |
| `WEB-PROV` | `web-console/provisioning-dashboard.spec.md` | User-story/contract outline, no Given/When/Then scenarios yet |

## Retired IDs

None yet. See `specs/standards/platform/spec-identifiers.spec.md` for why
the pre-migration ad-hoc IDs were renamed rather than retired, and
`specs/id-rename-map.md` for the old-to-new mapping.
