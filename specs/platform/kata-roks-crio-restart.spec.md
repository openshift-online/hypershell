# Kata ROKS CRI-O Rolling Restart Specification

**Date:** 2026-11-04
**Status:** Draft
**Ticket:** HYPERSHELL-477
**ID prefix:** KATA-ROKS
**Related:** `platform/openshell-gateway.spec.md` (OSG-CORE, "Sandbox Runtime Class"
section, `GATEWAY_SANDBOX_RUNTIME_CLASS=kata`), `platform/managed-cluster-registration.spec.md`
(PLAT-CLUS, the self-declared `runtime/kata` capability label),
`platform/global-architecture.spec.md` (PLAT-ARCH, "GitOps Repository Structure",
sync-wave ordering)

## Purpose

A ManagedCluster opts a gateway's sandboxes into Kata Containers (hardware VM
isolation) by installing the Sandboxed Containers Operator and setting
`GATEWAY_SANDBOX_RUNTIME_CLASS=kata` (`platform/openshell-gateway.spec.md`,
"Sandbox Runtime Class"). On a standard OpenShift cluster the Machine Config
Operator restarts CRI-O on every worker node whenever the Kata operator writes
its drop-in configuration under `/etc/crio/crio.conf.d/50-kata*`, so CRI-O
picks up the new `kata` runtime handler automatically. IBM Cloud ROKS clusters
are HyperShift-hosted and have no Machine Config Operator
(`platform/global-architecture.spec.md`, ROKS notes), so nothing restarts
CRI-O: the drop-in file lands on disk, but the already-running CRI-O process
never rereads it. Every sandbox pod that requests the `kata` `RuntimeClass`
then stays in `PodInitializing` with `failed to find runtime handler kata
from runtime list map[crun runc]`, even though the operator reports Kata as
installed.

This spec defines the desired end state: a ROKS Kata spoke's GitOps
automation (`bases/operators/sandboxed-containers` in the central
[`hypershell-gitops`](https://github.com/openshift-online/hypershell-gitops)
repository) converges CRI-O on every worker node onto the `kata` runtime
handler with no human running the manual
`oc debug node/<node> -- chroot /host sh -c 'systemctl restart crio'`
workaround, and does so one node at a time so running workloads are not
disrupted. HyperShell (this repository) does not host the GitOps automation
itself; it specifies the behavior the `hypershell-gitops` repository SHALL
implement for any ROKS (or other Machine-Config-Operator-less) cluster that
enables the Kata GitOps component.

## Requirements

### Requirement: KATA-ROKS-01 -- Automatic CRI-O Convergence After Kata Install

On a ManagedCluster that has no Machine Config Operator (for example a ROKS
spoke) and has the Sandboxed Containers Operator / Kata GitOps component
enabled, the GitOps automation SHALL ensure that, after the operator installs
its CRI-O drop-in configuration, CRI-O on every worker node lists the `kata`
runtime handler, without a human running a manual CRI-O restart.

#### Scenario: KATA-ROKS-01.01 -- Fresh Kata spoke converges unattended

- GIVEN a ROKS cluster with the Kata GitOps component newly enabled
- AND the Sandboxed Containers Operator has just written its CRI-O drop-in
  configuration to every worker node
- WHEN the GitOps automation next reconciles
- THEN CRI-O on every worker node SHALL list the `kata` runtime handler
- AND no person SHALL have run a manual CRI-O restart on any node

#### Scenario: KATA-ROKS-01.02 -- Sandbox scheduling recovers without manual intervention

- GIVEN a gateway's sandboxes request `runtimeClassName: kata`
- AND sandbox pods were previously stuck in `PodInitializing` with
  `failed to find runtime handler kata from runtime list map[crun runc]`
- WHEN the GitOps automation converges CRI-O on the node a pending sandbox
  pod is scheduled to
- THEN that sandbox pod SHALL progress past `PodInitializing`

---

### Requirement: KATA-ROKS-02 -- Rolling, One Node at a Time

The GitOps automation SHALL restart CRI-O on at most one worker node at a
time on a given ManagedCluster. It SHALL NOT restart CRI-O on the next node
until the current node's CRI-O process is active again.

#### Scenario: KATA-ROKS-02.01 -- Other nodes stay available during a restart

- GIVEN a ROKS Kata spoke with multiple worker nodes
- AND the automation is restarting CRI-O on one worker node
- WHEN that restart is in progress
- THEN CRI-O on every other worker node SHALL remain untouched and available

#### Scenario: KATA-ROKS-02.02 -- Running sandboxes survive their node's restart

- GIVEN a worker node is running one or more sandbox pods
- WHEN the automation restarts CRI-O on that node
- THEN those already-running pods SHALL remain running after the restart
  completes, consistent with the documented manual workaround (`crictl info`
  lists the kata runtime and running pods survive the restart)

---

### Requirement: KATA-ROKS-03 -- New and Replaced Nodes Converge Automatically

A worker node that joins a Kata-enabled ManagedCluster after the Sandboxed
Containers Operator is already installed (an autoscaled node, a replaced
node, or a node added by a new MachineSet) SHALL also end up with CRI-O
listing the `kata` runtime handler, with no manual step, not only the nodes
present at the time the operator was first installed.

#### Scenario: KATA-ROKS-03.01 -- A newly joined node is not left behind

- GIVEN a ROKS Kata spoke where every existing worker node already lists the
  `kata` runtime handler
- WHEN a new worker node joins the cluster
- THEN the GitOps automation SHALL converge that new node's CRI-O onto the
  `kata` runtime handler without a human running a manual step

---

### Requirement: KATA-ROKS-04 -- Idempotent Reconciliation

The GitOps automation SHALL NOT restart CRI-O on a worker node whose CRI-O
already lists the `kata` runtime handler. Reconciling repeatedly (for
example on every GitOps sync) SHALL be a no-op on a node that has already
converged.

#### Scenario: KATA-ROKS-04.01 -- Repeated sync does not flap a converged node

- GIVEN a worker node's CRI-O already lists the `kata` runtime handler
- WHEN the GitOps automation reconciles again
- THEN it SHALL NOT restart CRI-O on that node

---

### Requirement: KATA-ROKS-05 -- Scoped to Kata-Enabled Clusters

The GitOps automation described by this spec SHALL run only on
ManagedClusters that have the Kata GitOps component
(`bases/operators/sandboxed-containers`) enabled. It SHALL NOT restart CRI-O
on a cluster that does not enable that component, and SHALL NOT restart
CRI-O for any reason other than converging the `kata` runtime handler.

#### Scenario: KATA-ROKS-05.01 -- Non-Kata clusters are unaffected

- GIVEN a ManagedCluster that does not enable the Kata GitOps component
- WHEN the GitOps automation in this spec reconciles the fleet
- THEN CRI-O on that cluster's worker nodes SHALL NOT be restarted

---

### Requirement: KATA-ROKS-06 -- Drift Is a Validation Failure, Not a Silent Gap

Cluster validation for a Kata-enabled ManagedCluster (tracked separately as
HYPERSHELL-367 / `openshift-online/hypershell#426`) SHALL treat a worker node
that is expected to carry the `kata` runtime handler (the cluster declares
the `runtime/kata` capability, `platform/managed-cluster-registration.spec.md`
PLAT-CLUS) but whose CRI-O does not list it as a validation failure, so the
gap between the self-declared capability and actual node state is surfaced
instead of leaving sandboxes stuck in `PodInitializing` until someone
happens to notice.

#### Scenario: KATA-ROKS-06.01 -- Validation catches an unconverged node

- GIVEN a ManagedCluster has registered the `runtime/kata` capability
- AND one worker node's CRI-O does not list the `kata` runtime handler
- WHEN cluster validation runs against that cluster
- THEN validation SHALL fail for that node

## Non-Goals

- Installing the Sandboxed Containers Operator itself (OLM Subscription,
  `KataConfig` CR) - covered by the operator's own GitOps manifests, not this
  spec.
- CRI-O behavior on standard OpenShift clusters with a Machine Config
  Operator, which already restart CRI-O on drop-in changes without this
  automation.
- Draining or cordoning worker nodes before a CRI-O restart. The documented
  manual workaround shows running pods survive a CRI-O restart without a
  drain; this spec does not add one.
- The full design of cluster validation (HYPERSHELL-367 /
  `openshift-online/hypershell#426`); KATA-ROKS-06 states the required
  end-state behavior, not its implementation.
- Worker node hardware virtualization (KVM) prerequisites, already covered by
  `platform/openshell-gateway.spec.md` "Sandbox Runtime Class".
