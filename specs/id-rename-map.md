# Spec ID History

This file records identifiers that no longer name a live requirement.
`scripts/check_spec_ids.py` reads it. Spec files do not carry this history,
so agents that read specs see only current requirements
(`specs/standards/platform/spec-identifiers.spec.md`, STD-IDS-04).

- **Renamed IDs**: pre-migration ad-hoc IDs and the `MODULE-GROUP-NN` ID each
  became. An old ID cited anywhere outside this file and the ID standard
  fails `make check`.
- **Retired IDs**: IDs of deleted requirements and scenarios. A retired ID is
  never reused.

## Renamed IDs

The `MODULE-GROUP-NN` migration (HYPERSHELL-419 / issue #636) renamed 411
pre-migration ad-hoc IDs, each exactly once. A rename is not a retirement:
the requirement each ID names did not go away, only its identifier changed.

| Old ID | New ID | File |
|--------|--------|------|
| `SDLC-VIS-12a` | `SDLC-VIS-20` | `platform/agent-sdlc-visibility-dashboard.spec.md` |
| `SDLC-VIS-12b` | `SDLC-VIS-21` | `platform/agent-sdlc-visibility-dashboard.spec.md` |
| `ARM-00` | `OBS-REL-08` | `platform/api-reliability-metrics.spec.md` |
| `ARM-01` | `OBS-REL-01` | `platform/api-reliability-metrics.spec.md` |
| `ARM-02` | `OBS-REL-02` | `platform/api-reliability-metrics.spec.md` |
| `ARM-03` | `OBS-REL-03` | `platform/api-reliability-metrics.spec.md` |
| `ARM-04` | `OBS-REL-04` | `platform/api-reliability-metrics.spec.md` |
| `ARM-05` | `OBS-REL-05` | `platform/api-reliability-metrics.spec.md` |
| `ARM-06` | `OBS-REL-06` | `platform/api-reliability-metrics.spec.md` |
| `ARM-07` | `OBS-REL-07` | `platform/api-reliability-metrics.spec.md` |
| `CC-01` | `OBS-CPU-01` | `platform/cluster-cpu.spec.md` |
| `CC-02` | `OBS-CPU-02` | `platform/cluster-cpu.spec.md` |
| `CC-03` | `OBS-CPU-03` | `platform/cluster-cpu.spec.md` |
| `CC-04` | `OBS-CPU-04` | `platform/cluster-cpu.spec.md` |
| `CC-05` | `OBS-CPU-05` | `platform/cluster-cpu.spec.md` |
| `CC-06` | `OBS-CPU-06` | `platform/cluster-cpu.spec.md` |
| `CC-07` | `OBS-CPU-07` | `platform/cluster-cpu.spec.md` |
| `CC-08` | `OBS-CPU-08` | `platform/cluster-cpu.spec.md` |
| `CM-01` | `OBS-MEM-01` | `platform/cluster-memory.spec.md` |
| `CM-02` | `OBS-MEM-02` | `platform/cluster-memory.spec.md` |
| `CM-03` | `OBS-MEM-03` | `platform/cluster-memory.spec.md` |
| `CM-04` | `OBS-MEM-04` | `platform/cluster-memory.spec.md` |
| `CM-05` | `OBS-MEM-05` | `platform/cluster-memory.spec.md` |
| `CM-06` | `OBS-MEM-06` | `platform/cluster-memory.spec.md` |
| `CM-07` | `OBS-MEM-07` | `platform/cluster-memory.spec.md` |
| `CM-08` | `OBS-MEM-08` | `platform/cluster-memory.spec.md` |
| `CLN-01` | `OBS-NODE-01` | `platform/cluster-nodes.spec.md` |
| `CLN-02` | `OBS-NODE-02` | `platform/cluster-nodes.spec.md` |
| `CLN-03` | `OBS-NODE-03` | `platform/cluster-nodes.spec.md` |
| `CLN-04` | `OBS-NODE-04` | `platform/cluster-nodes.spec.md` |
| `CLN-05` | `OBS-NODE-05` | `platform/cluster-nodes.spec.md` |
| `CLN-06` | `OBS-NODE-06` | `platform/cluster-nodes.spec.md` |
| `CLN-07` | `OBS-NODE-07` | `platform/cluster-nodes.spec.md` |
| `CLN-08` | `OBS-NODE-08` | `platform/cluster-nodes.spec.md` |
| `CLP-01` | `OBS-POD-01` | `platform/cluster-pods.spec.md` |
| `CLP-02` | `OBS-POD-02` | `platform/cluster-pods.spec.md` |
| `CLP-03` | `OBS-POD-03` | `platform/cluster-pods.spec.md` |
| `CLP-04` | `OBS-POD-04` | `platform/cluster-pods.spec.md` |
| `CLP-05` | `OBS-POD-05` | `platform/cluster-pods.spec.md` |
| `CLP-06` | `OBS-POD-06` | `platform/cluster-pods.spec.md` |
| `CLP-07` | `OBS-POD-07` | `platform/cluster-pods.spec.md` |
| `CLP-08` | `OBS-POD-08` | `platform/cluster-pods.spec.md` |
| `CP-OBS-01` | `OBS-CPL-01` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-02` | `OBS-CPL-02` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-03` | `OBS-CPL-03` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-04` | `OBS-CPL-04` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-05` | `OBS-CPL-05` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-06` | `OBS-CPL-06` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-07` | `OBS-CPL-07` | `platform/control-plane-observability.spec.md` |
| `CP-OBS-08` | `OBS-CPL-08` | `platform/control-plane-observability.spec.md` |
| `CRM-001` | `OBS-CRM-01` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-002` | `OBS-CRM-02` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-003` | `OBS-CRM-03` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-004` | `OBS-CRM-04` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-005` | `OBS-CRM-05` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-006` | `OBS-CRM-06` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-007` | `OBS-CRM-07` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-008` | `OBS-CRM-08` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-009` | `OBS-CRM-09` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-010` | `OBS-CRM-10` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-011` | `OBS-CRM-11` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-012` | `OBS-CRM-12` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `CRM-013` | `OBS-CRM-13` | `platform/control-plane-reconciliation-metrics.spec.md` |
| `GAM-01` | `GTW-ACC-01` | `platform/gateway-access-management.spec.md` |
| `GAM-02` | `GTW-ACC-02` | `platform/gateway-access-management.spec.md` |
| `GAM-03` | `GTW-ACC-03` | `platform/gateway-access-management.spec.md` |
| `GAM-04` | `GTW-ACC-04` | `platform/gateway-access-management.spec.md` |
| `GAM-05` | `GTW-ACC-05` | `platform/gateway-access-management.spec.md` |
| `GAM-06` | `GTW-ACC-06` | `platform/gateway-access-management.spec.md` |
| `GAM-07` | `GTW-ACC-07` | `platform/gateway-access-management.spec.md` |
| `GAM-08` | `GTW-ACC-08` | `platform/gateway-access-management.spec.md` |
| `GAM-09` | `GTW-ACC-09` | `platform/gateway-access-management.spec.md` |
| `GAM-10` | `GTW-ACC-10` | `platform/gateway-access-management.spec.md` |
| `GAM-11` | `GTW-ACC-11` | `platform/gateway-access-management.spec.md` |
| `GAM-12` | `GTW-ACC-12` | `platform/gateway-access-management.spec.md` |
| `GAM-13` | `GTW-ACC-13` | `platform/gateway-access-management.spec.md` |
| `GAM-14` | `GTW-ACC-14` | `platform/gateway-access-management.spec.md` |
| `GFT-01` | `OBS-GFT-01` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-02` | `OBS-GFT-02` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-03` | `OBS-GFT-03` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-04` | `OBS-GFT-04` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-05` | `OBS-GFT-05` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-06` | `OBS-GFT-06` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-07` | `OBS-GFT-07` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-08` | `OBS-GFT-08` | `platform/gateway-fleet-total-trend.spec.md` |
| `GFT-09` | `OBS-GFT-09` | `platform/gateway-fleet-total-trend.spec.md` |
| `GMCA-01` | `GTW-ATTR-01` | `platform/gateway-managed-cluster-attribution.spec.md` |
| `GMCA-02` | `GTW-ATTR-02` | `platform/gateway-managed-cluster-attribution.spec.md` |
| `GMCA-03` | `GTW-ATTR-03` | `platform/gateway-managed-cluster-attribution.spec.md` |
| `GMCA-04` | `GTW-ATTR-04` | `platform/gateway-managed-cluster-attribution.spec.md` |
| `GMCA-05` | `GTW-ATTR-05` | `platform/gateway-managed-cluster-attribution.spec.md` |
| `GMCA-06` | `GTW-ATTR-06` | `platform/gateway-managed-cluster-attribution.spec.md` |
| `DASH-01` | `OBS-GMD-01` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-02` | `OBS-GMD-02` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-03` | `OBS-GMD-03` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-04` | `OBS-GMD-04` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-05` | `OBS-GMD-05` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-06` | `OBS-GMD-06` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-07` | `OBS-GMD-07` | `platform/gateway-metrics-dashboard.spec.md` |
| `DASH-08` | `OBS-GMD-08` | `platform/gateway-metrics-dashboard.spec.md` |
| `GPO-00` | `OBS-GPO-09` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-01` | `OBS-GPO-01` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-02` | `OBS-GPO-02` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-03` | `OBS-GPO-03` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-04` | `OBS-GPO-04` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-05` | `OBS-GPO-05` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-06` | `OBS-GPO-06` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-07` | `OBS-GPO-07` | `platform/gateway-provision-outcomes.spec.md` |
| `GPO-08` | `OBS-GPO-08` | `platform/gateway-provision-outcomes.spec.md` |
| `GPT-00` | `OBS-GPT-09` | `platform/gateway-provision-time.spec.md` |
| `GPT-01` | `OBS-GPT-01` | `platform/gateway-provision-time.spec.md` |
| `GPT-02` | `OBS-GPT-02` | `platform/gateway-provision-time.spec.md` |
| `GPT-03` | `OBS-GPT-03` | `platform/gateway-provision-time.spec.md` |
| `GPT-04` | `OBS-GPT-04` | `platform/gateway-provision-time.spec.md` |
| `GPT-05` | `OBS-GPT-05` | `platform/gateway-provision-time.spec.md` |
| `GPT-06` | `OBS-GPT-06` | `platform/gateway-provision-time.spec.md` |
| `GPT-07` | `OBS-GPT-07` | `platform/gateway-provision-time.spec.md` |
| `GPT-08` | `OBS-GPT-08` | `platform/gateway-provision-time.spec.md` |
| `GPP-01` | `GTW-PROG-01` | `platform/gateway-provisioning-progress.spec.md` |
| `GPP-02` | `GTW-PROG-02` | `platform/gateway-provisioning-progress.spec.md` |
| `GPP-03` | `GTW-PROG-03` | `platform/gateway-provisioning-progress.spec.md` |
| `GPP-04` | `GTW-PROG-04` | `platform/gateway-provisioning-progress.spec.md` |
| `GPP-05` | `GTW-PROG-05` | `platform/gateway-provisioning-progress.spec.md` |
| `CP-CONC-01` | `GTW-CONC-01` | `platform/gateway-reconcile-concurrency.spec.md` |
| `CP-CONC-02` | `GTW-CONC-02` | `platform/gateway-reconcile-concurrency.spec.md` |
| `CP-CONC-03` | `GTW-CONC-03` | `platform/gateway-reconcile-concurrency.spec.md` |
| `GRD-01` | `OBS-RLSD-01` | `platform/gateway-release-distribution.spec.md` |
| `GRD-02` | `OBS-RLSD-02` | `platform/gateway-release-distribution.spec.md` |
| `GRD-03` | `OBS-RLSD-03` | `platform/gateway-release-distribution.spec.md` |
| `GRD-04` | `OBS-RLSD-04` | `platform/gateway-release-distribution.spec.md` |
| `GRD-05` | `OBS-RLSD-05` | `platform/gateway-release-distribution.spec.md` |
| `GRD-06` | `OBS-RLSD-06` | `platform/gateway-release-distribution.spec.md` |
| `GRD-07` | `OBS-RLSD-07` | `platform/gateway-release-distribution.spec.md` |
| `GSAT-01` | `OBS-SBXT-01` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-02` | `OBS-SBXT-02` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-03` | `OBS-SBXT-03` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-04` | `OBS-SBXT-04` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-05` | `OBS-SBXT-05` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-06` | `OBS-SBXT-06` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-07` | `OBS-SBXT-07` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-08` | `OBS-SBXT-08` | `platform/gateway-sandbox-active-trends.spec.md` |
| `GSAT-09` | `OBS-SBXT-09` | `platform/gateway-sandbox-active-trends.spec.md` |
| `SSA-01` | `OBS-SBXA-01` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-02` | `OBS-SBXA-02` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-03` | `OBS-SBXA-03` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-04` | `OBS-SBXA-04` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-05` | `OBS-SBXA-05` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-06` | `OBS-SBXA-06` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-07` | `OBS-SBXA-07` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-08` | `OBS-SBXA-08` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-09` | `OBS-SBXA-09` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-10` | `OBS-SBXA-10` | `platform/gateway-sandbox-attention-status.spec.md` |
| `SSA-11` | `OBS-SBXA-11` | `platform/gateway-sandbox-attention-status.spec.md` |
| `GLASS-NAV-01` | `FLT-NAV-01` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-02` | `FLT-NAV-02` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-03` | `FLT-NAV-03` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-04` | `FLT-NAV-04` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-05` | `FLT-NAV-05` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-06` | `FLT-NAV-06` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-07` | `FLT-NAV-07` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-08` | `FLT-NAV-08` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-09` | `FLT-NAV-09` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-10` | `FLT-NAV-10` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-11` | `FLT-NAV-11` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-12` | `FLT-NAV-12` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-13` | `FLT-NAV-13` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-14` | `FLT-NAV-14` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-15` | `FLT-NAV-15` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-16` | `FLT-NAV-16` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-17` | `FLT-NAV-17` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-18` | `FLT-NAV-18` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-19` | `FLT-NAV-19` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-20` | `FLT-NAV-20` | `platform/glass-url-addressable-navigation.spec.md` |
| `GLASS-NAV-21` | `FLT-NAV-21` | `platform/glass-url-addressable-navigation.spec.md` |
| `HEC-01` | `CLI-EXT-01` | `platform/hsctl-extension-commands.spec.md` |
| `HEC-02` | `CLI-EXT-02` | `platform/hsctl-extension-commands.spec.md` |
| `HEC-03` | `CLI-EXT-03` | `platform/hsctl-extension-commands.spec.md` |
| `HEC-04` | `CLI-EXT-04` | `platform/hsctl-extension-commands.spec.md` |
| `HSR-01` | `CLI-REL-01` | `platform/hsctl-release.spec.md` |
| `HSR-02` | `CLI-REL-02` | `platform/hsctl-release.spec.md` |
| `HSR-03` | `CLI-REL-03` | `platform/hsctl-release.spec.md` |
| `HSR-04` | `CLI-REL-04` | `platform/hsctl-release.spec.md` |
| `HSR-05` | `CLI-REL-05` | `platform/hsctl-release.spec.md` |
| `HSR-06` | `CLI-REL-06` | `platform/hsctl-release.spec.md` |
| `TUI-01` | `CLI-TUI-01` | `platform/hsctl-terminal-ui.spec.md` |
| `TUI-02` | `CLI-TUI-02` | `platform/hsctl-terminal-ui.spec.md` |
| `TUI-03` | `CLI-TUI-03` | `platform/hsctl-terminal-ui.spec.md` |
| `TUI-04` | `CLI-TUI-04` | `platform/hsctl-terminal-ui.spec.md` |
| `HCUT-01` | `OBS-HCUT-01` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-02` | `OBS-HCUT-02` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-03` | `OBS-HCUT-03` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-04` | `OBS-HCUT-04` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-05` | `OBS-HCUT-05` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-06` | `OBS-HCUT-06` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-07` | `OBS-HCUT-07` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-08` | `OBS-HCUT-08` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-09` | `OBS-HCUT-09` | `platform/hub-cluster-utilization-trends.spec.md` |
| `HCUT-10` | `OBS-HCUT-10` | `platform/hub-cluster-utilization-trends.spec.md` |
| `PI-01` | `PLAT-INV-01` | `platform/platform-inventory.spec.md` |
| `PI-02` | `PLAT-INV-02` | `platform/platform-inventory.spec.md` |
| `PI-03` | `PLAT-INV-03` | `platform/platform-inventory.spec.md` |
| `PI-04` | `PLAT-INV-04` | `platform/platform-inventory.spec.md` |
| `PI-05` | `PLAT-INV-05` | `platform/platform-inventory.spec.md` |
| `PI-06` | `PLAT-INV-06` | `platform/platform-inventory.spec.md` |
| `PI-07` | `PLAT-INV-07` | `platform/platform-inventory.spec.md` |
| `PI-08` | `PLAT-INV-08` | `platform/platform-inventory.spec.md` |
| `PI-09` | `PLAT-INV-09` | `platform/platform-inventory.spec.md` |
| `PI-10` | `PLAT-INV-10` | `platform/platform-inventory.spec.md` |
| `RTC-01` | `CTRL-TRC-01` | `platform/reconcile-trace-correlation.spec.md` |
| `RTC-02` | `CTRL-TRC-02` | `platform/reconcile-trace-correlation.spec.md` |
| `RTC-03` | `CTRL-TRC-03` | `platform/reconcile-trace-correlation.spec.md` |
| `RTC-04` | `CTRL-TRC-04` | `platform/reconcile-trace-correlation.spec.md` |
| `RTC-05` | `CTRL-TRC-05` | `platform/reconcile-trace-correlation.spec.md` |
| `RU-01` | `PLAT-USR-01` | `platform/registered-users.spec.md` |
| `RU-02` | `PLAT-USR-02` | `platform/registered-users.spec.md` |
| `RU-03` | `PLAT-USR-03` | `platform/registered-users.spec.md` |
| `RU-04` | `PLAT-USR-04` | `platform/registered-users.spec.md` |
| `RU-05` | `PLAT-USR-05` | `platform/registered-users.spec.md` |
| `RU-06` | `PLAT-USR-06` | `platform/registered-users.spec.md` |
| `RU-07` | `PLAT-USR-07` | `platform/registered-users.spec.md` |
| `RU-08` | `PLAT-USR-08` | `platform/registered-users.spec.md` |
| `RU-09` | `PLAT-USR-09` | `platform/registered-users.spec.md` |
| `RU-10` | `PLAT-USR-10` | `platform/registered-users.spec.md` |
| `RU-11` | `PLAT-USR-11` | `platform/registered-users.spec.md` |
| `RU-12` | `PLAT-USR-12` | `platform/registered-users.spec.md` |
| `RU-13` | `PLAT-USR-13` | `platform/registered-users.spec.md` |
| `RU-14` | `PLAT-USR-14` | `platform/registered-users.spec.md` |
| `RU-15` | `PLAT-USR-15` | `platform/registered-users.spec.md` |
| `UI-A11Y-01` | `UIS-A11Y-01` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-02` | `UIS-A11Y-02` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-03` | `UIS-A11Y-03` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-04` | `UIS-A11Y-04` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-05` | `UIS-A11Y-05` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-06` | `UIS-A11Y-06` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-07` | `UIS-A11Y-07` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-08` | `UIS-A11Y-08` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-09` | `UIS-A11Y-09` | `standards/ui/accessibility.spec.md` |
| `UI-A11Y-10` | `UIS-A11Y-10` | `standards/ui/accessibility.spec.md` |
| `UI-BRAND-01` | `UIS-BRND-01` | `standards/ui/brand-color.spec.md` |
| `UI-BRAND-02` | `UIS-BRND-02` | `standards/ui/brand-color.spec.md` |
| `UI-BRAND-03` | `UIS-BRND-03` | `standards/ui/brand-color.spec.md` |
| `UI-BRAND-04` | `UIS-BRND-04` | `standards/ui/brand-color.spec.md` |
| `UI-BRAND-05` | `UIS-BRND-05` | `standards/ui/brand-color.spec.md` |
| `UI-BRAND-06` | `UIS-BRND-06` | `standards/ui/brand-color.spec.md` |
| `UI-CONTENT-01` | `UIS-CONT-01` | `standards/ui/content-localization.spec.md` |
| `UI-CONTENT-02` | `UIS-CONT-02` | `standards/ui/content-localization.spec.md` |
| `UI-CONTENT-03` | `UIS-CONT-03` | `standards/ui/content-localization.spec.md` |
| `UI-CONTENT-04` | `UIS-CONT-04` | `standards/ui/content-localization.spec.md` |
| `UI-CONTENT-05` | `UIS-CONT-05` | `standards/ui/content-localization.spec.md` |
| `UI-I18N-01` | `UIS-I18N-01` | `standards/ui/content-localization.spec.md` |
| `UI-I18N-02` | `UIS-I18N-02` | `standards/ui/content-localization.spec.md` |
| `UI-I18N-03` | `UIS-I18N-03` | `standards/ui/content-localization.spec.md` |
| `UI-I18N-04` | `UIS-I18N-04` | `standards/ui/content-localization.spec.md` |
| `UI-I18N-05` | `UIS-I18N-05` | `standards/ui/content-localization.spec.md` |
| `UI-OBS-01` | `UIS-OBS-01` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-02` | `UIS-OBS-02` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-03` | `UIS-OBS-03` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-04` | `UIS-OBS-04` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-05` | `UIS-OBS-05` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-06` | `UIS-OBS-06` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-07` | `UIS-OBS-07` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-08` | `UIS-OBS-08` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-09` | `UIS-OBS-09` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-10` | `UIS-OBS-10` | `standards/ui/domain-observability.spec.md` |
| `UI-OBS-11` | `UIS-OBS-11` | `standards/ui/domain-observability.spec.md` |
| `UI-FND-01` | `UIS-FND-01` | `standards/ui/foundations.spec.md` |
| `UI-FND-02` | `UIS-FND-02` | `standards/ui/foundations.spec.md` |
| `UI-FND-03` | `UIS-FND-03` | `standards/ui/foundations.spec.md` |
| `UI-FND-04` | `UIS-FND-04` | `standards/ui/foundations.spec.md` |
| `UI-FND-05` | `UIS-FND-05` | `standards/ui/foundations.spec.md` |
| `UI-FND-06` | `UIS-FND-06` | `standards/ui/foundations.spec.md` |
| `UI-FND-07` | `UIS-FND-07` | `standards/ui/foundations.spec.md` |
| `UI-FND-08` | `UIS-FND-08` | `standards/ui/foundations.spec.md` |
| `UI-FND-09` | `UIS-FND-09` | `standards/ui/foundations.spec.md` |
| `UI-FND-10` | `UIS-FND-10` | `standards/ui/foundations.spec.md` |
| `UI-HEX-01` | `UIS-HEX-01` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-02` | `UIS-HEX-02` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-03` | `UIS-HEX-03` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-04` | `UIS-HEX-04` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-05` | `UIS-HEX-05` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-06` | `UIS-HEX-06` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-07` | `UIS-HEX-07` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-08` | `UIS-HEX-08` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-HEX-09` | `UIS-HEX-09` | `standards/ui/hexagonal-architecture.spec.md` |
| `UI-INT-01` | `UIS-INT-01` | `standards/ui/interaction.spec.md` |
| `UI-INT-02` | `UIS-INT-02` | `standards/ui/interaction.spec.md` |
| `UI-INT-03` | `UIS-INT-03` | `standards/ui/interaction.spec.md` |
| `UI-INT-04` | `UIS-INT-04` | `standards/ui/interaction.spec.md` |
| `UI-INT-05` | `UIS-INT-05` | `standards/ui/interaction.spec.md` |
| `UI-INT-06` | `UIS-INT-06` | `standards/ui/interaction.spec.md` |
| `UI-INT-07` | `UIS-INT-07` | `standards/ui/interaction.spec.md` |
| `UI-INT-08` | `UIS-INT-08` | `standards/ui/interaction.spec.md` |
| `UI-INT-09` | `UIS-INT-09` | `standards/ui/interaction.spec.md` |
| `UI-INT-10` | `UIS-INT-10` | `standards/ui/interaction.spec.md` |
| `UI-PF-01` | `UIS-PFLY-01` | `standards/ui/patternfly.spec.md` |
| `UI-PF-02` | `UIS-PFLY-02` | `standards/ui/patternfly.spec.md` |
| `UI-PF-03` | `UIS-PFLY-03` | `standards/ui/patternfly.spec.md` |
| `UI-PF-04` | `UIS-PFLY-04` | `standards/ui/patternfly.spec.md` |
| `UI-PF-05` | `UIS-PFLY-05` | `standards/ui/patternfly.spec.md` |
| `UI-PF-06` | `UIS-PFLY-06` | `standards/ui/patternfly.spec.md` |
| `UI-PF-07` | `UIS-PFLY-07` | `standards/ui/patternfly.spec.md` |
| `UI-PF-08` | `UIS-PFLY-08` | `standards/ui/patternfly.spec.md` |
| `UI-PF-09` | `UIS-PFLY-09` | `standards/ui/patternfly.spec.md` |
| `UI-PF-10` | `UIS-PFLY-10` | `standards/ui/patternfly.spec.md` |
| `UI-PERF-01` | `UIS-PERF-01` | `standards/ui/trust-performance.spec.md` |
| `UI-PERF-02` | `UIS-PERF-02` | `standards/ui/trust-performance.spec.md` |
| `UI-PERF-03` | `UIS-PERF-03` | `standards/ui/trust-performance.spec.md` |
| `UI-PERF-04` | `UIS-PERF-04` | `standards/ui/trust-performance.spec.md` |
| `UI-PERF-05` | `UIS-PERF-05` | `standards/ui/trust-performance.spec.md` |
| `UI-TRUST-01` | `UIS-TRST-01` | `standards/ui/trust-performance.spec.md` |
| `UI-TRUST-02` | `UIS-TRST-02` | `standards/ui/trust-performance.spec.md` |
| `UI-TRUST-03` | `UIS-TRST-03` | `standards/ui/trust-performance.spec.md` |
| `UI-TRUST-04` | `UIS-TRST-04` | `standards/ui/trust-performance.spec.md` |
| `UI-TRUST-05` | `UIS-TRST-05` | `standards/ui/trust-performance.spec.md` |
| `UI-VER-01` | `UIS-VER-01` | `standards/ui/verification.spec.md` |
| `UI-VER-02` | `UIS-VER-02` | `standards/ui/verification.spec.md` |
| `UI-VER-03` | `UIS-VER-03` | `standards/ui/verification.spec.md` |
| `UI-VER-04` | `UIS-VER-04` | `standards/ui/verification.spec.md` |
| `UI-VER-05` | `UIS-VER-05` | `standards/ui/verification.spec.md` |
| `UI-VER-06` | `UIS-VER-06` | `standards/ui/verification.spec.md` |
| `UI-VER-07` | `UIS-VER-07` | `standards/ui/verification.spec.md` |
| `UI-VER-08` | `UIS-VER-08` | `standards/ui/verification.spec.md` |
| `UI-VER-09` | `UIS-VER-09` | `standards/ui/verification.spec.md` |
| `UI-VER-10` | `UIS-VER-10` | `standards/ui/verification.spec.md` |
| `WEB-AUTH-00` | `WEB-AUTH-04` | `web-console/architecture.spec.md` |
| `WEB-DEPLOY-01` | `WEB-DPLY-01` | `web-console/architecture.spec.md` |
| `WEB-DEPLOY-02` | `WEB-DPLY-02` | `web-console/architecture.spec.md` |
| `WEB-UI-01` | `WEB-UIEX-01` | `web-console/architecture.spec.md` |
| `WEB-UI-02` | `WEB-UIEX-02` | `web-console/architecture.spec.md` |
| `WEB-UI-03` | `WEB-UIEX-03` | `web-console/architecture.spec.md` |
| `WEB-UI-03A` | `WEB-UIEX-06` | `web-console/architecture.spec.md` |
| `WEB-UI-04` | `WEB-UIEX-04` | `web-console/architecture.spec.md` |
| `WEB-UI-05` | `WEB-UIEX-05` | `web-console/architecture.spec.md` |
| `GAM-UI-01` | `WEB-ACC-01` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-02` | `WEB-ACC-02` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-03` | `WEB-ACC-03` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-04` | `WEB-ACC-04` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-05` | `WEB-ACC-05` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-06` | `WEB-ACC-06` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-07` | `WEB-ACC-07` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-08` | `WEB-ACC-08` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-09` | `WEB-ACC-09` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-10` | `WEB-ACC-10` | `web-console/gateway-access-management.spec.md` |
| `GAM-UI-11` | `WEB-ACC-11` | `web-console/gateway-access-management.spec.md` |
| `GP-01` | `WEB-PLC-01` | `web-console/gateway-provision-placement.spec.md` |
| `GP-02` | `WEB-PLC-02` | `web-console/gateway-provision-placement.spec.md` |
| `GP-03` | `WEB-PLC-03` | `web-console/gateway-provision-placement.spec.md` |
| `GP-04` | `WEB-PLC-04` | `web-console/gateway-provision-placement.spec.md` |
| `GP-05` | `WEB-PLC-05` | `web-console/gateway-provision-placement.spec.md` |
| `GP-06` | `WEB-PLC-06` | `web-console/gateway-provision-placement.spec.md` |
| `GP-07` | `WEB-PLC-07` | `web-console/gateway-provision-placement.spec.md` |
| `GP-08` | `WEB-PLC-08` | `web-console/gateway-provision-placement.spec.md` |
| `GP-09` | `WEB-PLC-09` | `web-console/gateway-provision-placement.spec.md` |
| `GP-10` | `WEB-PLC-10` | `web-console/gateway-provision-placement.spec.md` |
| `GP-11` | `WEB-PLC-11` | `web-console/gateway-provision-placement.spec.md` |
| `GVV-01` | `WEB-VER-01` | `web-console/gateway-version-visibility.spec.md` |
| `GVV-02` | `WEB-VER-02` | `web-console/gateway-version-visibility.spec.md` |
| `GVV-03` | `WEB-VER-03` | `web-console/gateway-version-visibility.spec.md` |
| `GVV-04` | `WEB-VER-04` | `web-console/gateway-version-visibility.spec.md` |
| `GVV-05` | `WEB-VER-05` | `web-console/gateway-version-visibility.spec.md` |
| `OP-DASH-01` | `WEB-OPS-01` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-02` | `WEB-OPS-02` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-03` | `WEB-OPS-03` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-04` | `WEB-OPS-04` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-05` | `WEB-OPS-05` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-06` | `WEB-OPS-06` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-07` | `WEB-OPS-07` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-08` | `WEB-OPS-08` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-09` | `WEB-OPS-09` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-10` | `WEB-OPS-10` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-11` | `WEB-OPS-11` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-12` | `WEB-OPS-12` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-13` | `WEB-OPS-13` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-14` | `WEB-OPS-14` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-15` | `WEB-OPS-15` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-16` | `WEB-OPS-16` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-17` | `WEB-OPS-17` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-18` | `WEB-OPS-18` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-19` | `WEB-OPS-19` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-21` | `WEB-OPS-21` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-22` | `WEB-OPS-22` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-23` | `WEB-OPS-23` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-25` | `WEB-OPS-25` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-26` | `WEB-OPS-26` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-28` | `WEB-OPS-28` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-29` | `WEB-OPS-29` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-30` | `WEB-OPS-30` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-31` | `WEB-OPS-31` | `web-console/operational-dashboard.spec.md` |
| `OP-DASH-32` | `WEB-OPS-32` | `web-console/operational-dashboard.spec.md` |
| `WEB-TRACE-01` | `WEB-TRC-01` | `web-console/tracing.spec.md` |
| `WEB-TRACE-02` | `WEB-TRC-02` | `web-console/tracing.spec.md` |
| `WEB-TRACE-03` | `WEB-TRC-03` | `web-console/tracing.spec.md` |
| `WEB-TRACE-04` | `WEB-TRC-04` | `web-console/tracing.spec.md` |
| `WEB-TRACE-05` | `WEB-TRC-05` | `web-console/tracing.spec.md` |
| `WEB-TRACE-06` | `WEB-TRC-06` | `web-console/tracing.spec.md` |
| `WEB-TRACE-07` | `WEB-TRC-07` | `web-console/tracing.spec.md` |
| `WEB-TRACE-08` | `WEB-TRC-08` | `web-console/tracing.spec.md` |
| `WEB-TRACE-09` | `WEB-TRC-09` | `web-console/tracing.spec.md` |
| `WEB-TRACE-10` | `WEB-TRC-10` | `web-console/tracing.spec.md` |
| `WEB-TRACE-11` | `WEB-TRC-11` | `web-console/tracing.spec.md` |
| `USAGE-DASH-01` | `WEB-USE-01` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-02` | `WEB-USE-02` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-03` | `WEB-USE-03` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-04` | `WEB-USE-04` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-05` | `WEB-USE-05` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-06` | `WEB-USE-06` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-07` | `WEB-USE-07` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-08` | `WEB-USE-08` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-09` | `WEB-USE-09` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-10` | `WEB-USE-10` | `web-console/usage-dashboard.spec.md` |
| `USAGE-DASH-11` | `WEB-USE-11` | `web-console/usage-dashboard.spec.md` |

## Later Renames

IDs that were added after the migration in a pre-migration style and
renamed when the ID check was introduced (`STD-IDS-06`).

| Old ID | New ID | File |
|--------|--------|------|
| `SDLC-VIS-11a` | `SDLC-VIS-22` | `platform/agent-sdlc-visibility-dashboard.spec.md` |
| `GSE-01` | `OSG-SEVT-01` | `platform/gateway-sandbox-events.spec.md` |
| `GSE-02` | `OSG-SEVT-02` | `platform/gateway-sandbox-events.spec.md` |
| `GSE-03` | `OSG-SEVT-03` | `platform/gateway-sandbox-events.spec.md` |
| `GSE-04` | `OSG-SEVT-04` | `platform/gateway-sandbox-events.spec.md` |
| `GSE-05` | `OSG-SEVT-05` | `platform/gateway-sandbox-events.spec.md` |
| `GSE-06` | `OSG-SEVT-06` | `platform/gateway-sandbox-events.spec.md` |

## Retired IDs

Deleting a requirement or scenario retires its ID. Add one row per retired
ID. Retiring a requirement also retires its scenarios. "Replaced by" names
the live ID that took over, or `-`; `make check` fails if it names an ID that
does not exist. "Former file" is the spec the ID lived in, relative to
`specs/`.

| ID | Former file | Replaced by | Reason |
|----|-------------|-------------|--------|
| `PLAT-ARCH-01` | `platform/global-architecture.spec.md` | - | Never named a requirement: the ID migration stamped it on a plain `### Requirements` sub-heading by mistake. |
| `PLAT-ARCH-02` | `platform/global-architecture.spec.md` | - | Never named a requirement: the ID migration stamped it on a plain `### Requirements` sub-heading by mistake. |
| `PLAT-ARCH-03` | `platform/global-architecture.spec.md` | - | Never named a requirement: the ID migration stamped it on a plain `### Requirements` sub-heading by mistake. |
| `PLAT-ARCH-04` | `platform/global-architecture.spec.md` | - | Never named a requirement: the ID migration stamped it on a plain `### Requirements` sub-heading by mistake. |
| `CICD-MRG-01.01` | `platform/mergify-parallel-merge-queue.spec.md` | - | Independent-lane behavior superseded by serial speculation. |
| `CICD-MRG-01.03` | `platform/mergify-parallel-merge-queue.spec.md` | - | Independent-lane behavior superseded by serial speculation. |
| `CICD-MRG-04.01` | `platform/mergify-parallel-merge-queue.spec.md` | - | Independent-lane behavior superseded by serial speculation. |
| `CICD-MRG-04.02` | `platform/mergify-parallel-merge-queue.spec.md` | - | Independent-lane behavior superseded by serial speculation. |
| `E2E-CORE-29.01` | `platform/e2e-testing.spec.md` | - | Recorded that `make kind-up` was unchanged by the deploy restructuring; that migration is done. |
| `E2E-COV-01` | `platform/e2e-feature-coverage.spec.md` | - | Recorded a dropped per-gateway credential-driver phase and had no scenarios. The Vault fault path is covered by the P2.7 `vault-fail-closed` phase. |
| `CICD-PREN-12.01` | `platform/ephemeral-pr-environments.spec.md` | - | Recorded the removal of `components/pr-test/e2e-openshell.sh` (HYPERSHELL-250). |
| `CICD-PREN-12.03` | `platform/ephemeral-pr-environments.spec.md` | - | Required documentation to stop citing the removed script; done. |
| `CICD-PREN-12.04` | `platform/ephemeral-pr-environments.spec.md` | `CICD-PREN-12` | Forbade recreating the removed script; the `pr_test` scope rule replaces it. |
| `DEV-OCP-09` | `platform/openshift-development.spec.md` | `CICD-PREN-12` | Recorded the removal of `components/pr-test/e2e-openshell.sh`. |
| `DEV-OCP-09.01` | `platform/openshift-development.spec.md` | `CICD-PREN-12` | Recorded the removal of `components/pr-test/e2e-openshell.sh`. |
| `DEV-OCP-09.02` | `platform/openshift-development.spec.md` | `CICD-PREN-12` | Covered a future ROKS retirement; the `pr_test` scope rule replaces it. |
| `WEB-PLC-11` | `web-console/gateway-provision-placement.spec.md` | `WEB-PLC-04` | Replaced by the preview-card rule. |
| `WEB-VER-03` | `web-console/gateway-version-visibility.spec.md` | - | Newer-version indicator removed with the gateway-version capability (#823). |
| `WEB-OPS-17.02` | `web-console/operational-dashboard.spec.md` | `WEB-OPS-17.03` | The pods widget no longer renders a trend sparkline; it appears only on the Provisioning dashboard, which shows current pod state. |
| `WEB-OPS-21` | `web-console/operational-dashboard.spec.md` | - | Optional platform inventory widgets folded into the widget catalog (#788). Not recorded as retired at the time. |
| `WEB-OPS-22` | `web-console/operational-dashboard.spec.md` | - | Platform inventory summary legacy layout removed (#788). Not recorded as retired at the time. |
| `WEB-OPS-22.01` | `web-console/operational-dashboard.spec.md` | - | Removed with WEB-OPS-22 (#788). |
| `OSG-CORE-04` | `platform/openshell-gateway.spec.md` | - | Described loading static gateway manifests from `/manifests/gateway/` and substituting placeholders. The control plane installs the gateway through the upstream Helm chart. |
| `OSG-CORE-04.01` | `platform/openshell-gateway.spec.md` | - | Retired with OSG-CORE-04. |
| `OSG-CORE-04.02` | `platform/openshell-gateway.spec.md` | - | Retired with OSG-CORE-04. |
| `OSG-CORE-17` | `platform/openshell-gateway.spec.md` | - | Described SSH-over-gRPC payload delivery into sandboxes, which was never built. OSG-CORE-16 places agent configuration out of scope. |
| `OSG-CORE-18.05` | `platform/openshell-gateway.spec.md` | `OSG-CORE-18.09` | The null `workspace_mode` path was removed: migration `2026110212000001` backfilled pre-field gateways to `shared` and made the column required. |
| `OSG-ROUT-01` | `platform/openshell-gateway-routing.spec.md` | - | Superseded placeholder: the control plane creates no NetworkPolicies in gateway namespaces (#339). |
| `OSG-CON-06` | `platform/openshell-gateway-console.spec.md` | - | Required the console NetworkPolicies `openshell-console-allow-router` and `openshell-gateway-allow-console`, removed with all gateway-namespace NetworkPolicies (#339). |
| `OSG-CON-06.01` | `platform/openshell-gateway-console.spec.md` | - | Retired with OSG-CON-06. |
| `OSG-HELM-04` | `platform/openshell-gateway-helm-adoption.spec.md` | - | Recorded the one-time move from static manifests to the Helm chart; the move is complete and the Helm release is the only deployment path. |
| `WEB-PKG-03` | `web-console/architecture.spec.md` | `WEB-PKG-02` | Tracked the one-time npm-to-pnpm migration, which is complete. Its lasting rule (the dependency-age check inspects every pnpm package and workspace importer) moved to WEB-PKG-02. |
| `OSG-NSGC-02.03` | `platform/openshell-gateway-namespace-gc.spec.md` | - | Covered namespaces left unlabeled from before the instance label existed. No longer needed: the fleet has no such namespaces that a live Gateway still uses (checked 2026-11-01). |
| `OSG-NSGC-02.04` | `platform/openshell-gateway-namespace-gc.spec.md` | - | Covered namespaces left unlabeled from before the instance label existed; reconcile labeling is OSG-NSGC-03.02. |
| `OSG-NSGC-04` | `platform/openshell-gateway-namespace-gc.spec.md` | - | Startup backfill of the instance label onto namespaces created before the label existed. No longer needed: the fleet has no such namespaces that a live Gateway still uses (checked 2026-11-01). |
| `OSG-NSGC-04.01` | `platform/openshell-gateway-namespace-gc.spec.md` | - | Retired with OSG-NSGC-04. |
| `OSG-NSGC-04.02` | `platform/openshell-gateway-namespace-gc.spec.md` | - | Retired with OSG-NSGC-04. |
| `OSG-NSGC-04.03` | `platform/openshell-gateway-namespace-gc.spec.md` | - | Retired with OSG-NSGC-04. |
| `OSG-ROT-01` | `platform/openshell-gateway-secret-rotation.spec.md` | - | Required the `applyConfigHashAnnotation` config-hash to cover the database credentials Secret. That function was removed in the Helm cutover (#194), and nothing restarts the gateway when the Secret changes. Credential rotation will be designed in a later spec. |
| `OSG-ROT-01.01` | `platform/openshell-gateway-secret-rotation.spec.md` | - | Retired with OSG-ROT-01. |
| `OSG-ROT-02` | `platform/openshell-gateway-secret-rotation.spec.md` | - | Reserved a future KEK rotation design that was never built. The KEK is not rotated; credential rotation will be designed in a later spec. |
| `PLAT-CLUS-07.01` | `platform/managed-cluster-registration.spec.md` | `PLAT-CLUS-07.03` | The capability-less heartbeat shim for control planes older than the capability model was removed; an empty capability set is now rejected. |
| `PLAT-CLUS-14` | `platform/managed-cluster-registration.spec.md` | - | The server-assigned `managed-cluster/<name>` identity capability was removed. The e2e matrix validates placement per distinct fleet capability (round-robin, `platform/e2e-testing.spec.md` E2E-CORE-30); deterministic single-cluster targeting moves to a separate `cluster_name` placement selector tracked in issue #857. |
| `PLAT-CLUS-14.01` | `platform/managed-cluster-registration.spec.md` | - | Scenario of the removed `managed-cluster/<name>` identity capability requirement. |
| `PLAT-CLUS-14.02` | `platform/managed-cluster-registration.spec.md` | - | Scenario of the removed `managed-cluster/<name>` identity capability requirement. |
| `PLAT-CLUS-14.03` | `platform/managed-cluster-registration.spec.md` | - | Scenario of the removed `managed-cluster/<name>` identity capability requirement. |
| `PLAT-CLUS-14.04` | `platform/managed-cluster-registration.spec.md` | - | Scenario of the removed `managed-cluster/<name>` identity capability requirement. |
| `PLAT-CLUS-14.05` | `platform/managed-cluster-registration.spec.md` | - | Scenario of the removed `managed-cluster/<name>` identity capability requirement. |
| `OSG-OIDC-03.02` | `platform/openshell-gateway-oidc.spec.md` | - | Described a custom raw-TOML `config` field on the Gateway that bypassed OIDC injection. The Gateway resource has no such field. |
| `WEB-PLC-08` | `web-console/gateway-provision-placement.spec.md` | `WEB-PLC-07` | The `mode: local-kind` placement intent was removed; the local Kind cluster is placed with the `provider/kind` capability selector and resolved by the ordinary selector contract (HYPERSHELL-476 / issue #924). |
| `WEB-PLC-08.01` | `web-console/gateway-provision-placement.spec.md` | `WEB-PLC-07` | Scenario of the removed local-kind placement resolution requirement. |
| `CLI-PLC-01.03` | `platform/hsctl-placement.spec.md` | `CLI-PLC-01.10` | The `--local-kind` flag was removed; the same local-development create is expressed with `--placement-selector provider/kind` (HYPERSHELL-476 / issue #924). |
| `CLI-PLC-03.04` | `platform/hsctl-placement.spec.md` | `CLI-PLC-03` | The local-kind dry-run pre-check special case was removed; the ordinary selector pre-check applies. |
| `CTRL-CORE-14.03` | `platform/control-plane.spec.md` | `CTRL-CORE-14.02` | The import writer no longer maps a `mode: local-kind` placement; the capability-selector mapping covers it (HYPERSHELL-476 / issue #924). |
| `PLAT-ARCH-20` | `platform/global-architecture.spec.md` | - | Required installation through a Tekton installer pipeline. No such pipeline exists; hub bootstrap is done by the GitOps repository's `bin/bootstrap`. |
| `PLAT-ARCH-20.01` | `platform/global-architecture.spec.md` | - | Retired with PLAT-ARCH-20. |
| `PLAT-ARCH-21` | `platform/global-architecture.spec.md` | - | Required clusters to be rebuildable from the Tekton installer pipeline, which does not exist. |
