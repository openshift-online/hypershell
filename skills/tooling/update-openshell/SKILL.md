---
name: update-openshell
description: >
  Update HyperShell to a target OpenShell gateway/supervisor image version.
  Two image sources exist: (1) stable midstream releases from github.com/opendatahub-io/openshell
  (rhaiv.N tags, published to quay.io/opendatahub/odh-openshell-*); (2) custom builds from
  a specific github.com/NVIDIA/OpenShell upstream branch or commit via the HyperShell build
  agent (for unmerged bug fixes or pre-release validation). Bumps the pinned image versions
  across the repo, triages release notes for contract-affecting changes, verifies the rendered
  gateway config still matches upstream, and folds every lesson back into this skill and the
  specs. Use when a new OpenShell version ships, when asked to "update OpenShell", "bump the
  gateway version", "pull the latest openshell", or to test an unmerged upstream fix.
---

# Update OpenShell

Sync HyperShell to a target OpenShell gateway/supervisor image. The mechanical
part is a version-pin sweep; the part that needs judgment is deciding whether a
new upstream feature changes HyperShell's own approach.

**This skill is self-reinforcing.** Every run that surfaces a mistake, a missed
file, a schema drift, or a judgment call MUST be folded back into this skill and
into the affected spec, in the same PR. The next run should never re-learn what
this run learned.

## Image sources

Two tracks produce compatible `odh-openshell-gateway` / `odh-openshell-supervisor`
images. The compiled bits are equivalent - the midstream is a release-gated build
of the upstream source.

| Track | When to use | Source repo | Image registry |
|-------|-------------|-------------|----------------|
| **Stable midstream** | Routine updates, production upgrades | `github.com/opendatahub-io/openshell` tags (`v0.0.NNN-rhaiv.M`) | `quay.io/opendatahub/odh-openshell-gateway:<tag>` |
| **Agent build** | Unmerged upstream fix, pre-release validation, branch testing | `github.com/NVIDIA/OpenShell` branch or commit | Built and pushed by the HyperShell build agent; image ref returned by the agent |

Use the stable midstream track unless you have a specific reason to test an
unmerged upstream commit. The midstream `rhaiv.M` patch series applies Red Hat
fixes on top of the upstream base version, so `v0.0.116-rhaiv.15` contains
everything in NVIDIA's `v0.0.116` plus additional patches.

## Resolving versions

**Stable midstream track:** the authoritative tag list is `opendatahub-io/openshell`.
Skip the `vm-runtime` float tag; use only versioned tags:

```bash
# Latest stable release
gh api repos/opendatahub-io/openshell/tags \
  --jq '[.[] | select(.name | test("^v[0-9]"))] | .[0].name'

# All available tags (newest first)
gh api repos/opendatahub-io/openshell/tags \
  --jq '[.[] | select(.name | test("^v[0-9]"))] | .[].name'
```

The tag IS the image tag: `v0.0.116-rhaiv.15` -> image
`quay.io/opendatahub/odh-openshell-gateway:v0.0.116-rhaiv.15`.
No version-string transformation needed (unlike the old NVIDIA path which required
stripping the leading `v`).

**Upstream NVIDIA releases** (for triage only, not the image source):

```bash
# Latest NVIDIA upstream releases - use to read release notes
gh api repos/NVIDIA/OpenShell/releases --jq '.[0:5] | .[] | .tag_name'
```

NVIDIA release notes describe the features and contract changes that eventually
appear in the midstream `rhaiv` series. Cross-reference them when triaging.

**Agent build track:** invoke the HyperShell build agent with the upstream branch
or commit SHA. The agent checks out `github.com/NVIDIA/OpenShell`, builds the
gateway and supervisor images, pushes them, and returns the full image reference
including digest. Use that reference as the pin value (treat it like a midstream
tag but with a digest instead of a named tag).

**This skill is self-reinforcing.** Every run that surfaces a mistake, a missed
file, a schema drift, or a judgment call MUST be folded back into this skill
(the [Version footprint](#version-footprint), [Contract surfaces](#contract-surfaces-to-triage),
or [Learnings log](#learnings-log)) and into the affected spec, in the same PR.
The next run should never re-learn what this run learned.

## Usage

```text
/update-openshell                        # update to the latest stable midstream release
/update-openshell v0.0.116-rhaiv.15      # update to a specific midstream tag
/update-openshell NVIDIA/OpenShell@<sha> # build and pin an unmerged upstream commit via the build agent
```

## User Input

```text
$ARGUMENTS
```

## Source of truth

The **authoritative** current version pins live in the controller manifest:

- `deploy/base/platform-resources/controller.yaml` (lines with `GATEWAY_IMAGE`, `GATEWAY_SUPERVISOR_IMAGE`)

```yaml
- name: GATEWAY_IMAGE
  value: quay.io/opendatahub/odh-openshell-gateway:<TAG>
- name: GATEWAY_SUPERVISOR_IMAGE
  value: quay.io/opendatahub/odh-openshell-supervisor:<TAG>
- name: GATEWAY_SANDBOX_RUNTIME_IMAGE   # must match the supervisor build
  value: quay.io/opendatahub/odh-openshell-sandbox:<TAG>
```

Where `<TAG>` is either a midstream tag (`v0.0.116-rhaiv.15`) or a digest
reference (`sha256:...`) for an agent-built image. `config.go` reads these at
runtime via `os.Getenv("GATEWAY_IMAGE")` / `os.Getenv("GATEWAY_SUPERVISOR_IMAGE")` -
there are no hardcoded fallback constants. A control-plane deployed without these
env vars will fail to provision gateways. Every other occurrence of the version
in the repo is a copy of these and MUST agree with them after a run.

## Version footprint

Update the version in every file below. Discover the live list before editing -
do not trust this table blindly; it is a checklist, not a source of truth:

```bash
grep -rln "odh-openshell-\(gateway\|supervisor\|sandbox\):" . | grep -v '\.git/'
grep -rln "openshell/\(gateway\|supervisor\):" . | grep -v '\.git/'     # older image names (ghcr.io/nvidia)
grep -rn  "<OLD_VERSION>" . | grep -v '\.git/'      # must return only intentional fixtures afterwards
```

| File | What to change | Notes |
|------|----------------|-------|
| `OPENSHELL_VERSION` | `OPENSHELL_TAG`, `OPENSHELL_CONSOLE_IMAGE`, `OPENSHELL_CONSOLE_DIGEST` | Edit first; `OPENSHELL_TAG` drives both the deployment pins and the chart vendor step |
| `charts/openshell/` | Entire directory replaced from upstream tag | Vendored chart - see Step 3a for the extraction command; expect ~48 files, large diff is normal |
| `components/control-plane/internal/gateway/config.go` | `defaultConsoleImage` constant (digest + comment tag) | Must agree with `OPENSHELL_CONSOLE_DIGEST`; check when triage flags any proto or gRPC surface change |
| `deploy/base/platform-resources/controller.yaml` | `GATEWAY_IMAGE`, `GATEWAY_SUPERVISOR_IMAGE` env vars | **Source of truth** - change here first |
| `specs/platform/data-model.spec.md` | `supervisor_image` default | Spec citation |
| `specs/platform/openshell-gateway.spec.md` | gateway + supervisor defaults | Spec citation (multiple) |
| `specs/platform/openshell-gateway-credentials.spec.md` | example manifests | Spec citation |
| `specs/platform/global-architecture.spec.md` | version in the API-compat note | Also re-check the `v1beta1` claim (see below) |
| `components/pr-test/e2e-openshell-roks.sh` | `GW_IMAGE`, `GW_SUPERVISOR_IMAGE` defaults | ROKS e2e |
| `specs/platform/openshell-gateway-database.spec.md` | none since the 2026-09-16 rewrite (no gateway image refs remain) | Keep in the discovery grep; historically **was pinned to a git SHA, not a semver tag** |
| `scripts/kind/lib.sh` | `GATEWAY_IMAGE` default | Local Kind |
| `skills/deploy/ibm-cluster/SKILL.md` | mirror + `openshell gateway add` commands + `v1beta1` API-version note | ROKS mirror docs |

**Do NOT change** version strings that are illustrative fixtures or historical
facts rather than the active pin:
- the example tag in
  `components/control-plane/internal/gateway/validation_test.go` (it exercises the
  image-reference regex, not the deployed version);
- the sentence in `specs/platform/openshell-gateway-credentials.spec.md` that says
  "Upstream OpenShell **v0.0.101 introduced** pluggable credential storage
  drivers" - this records *which* release added a feature and must not be bumped.

If unsure whether an occurrence is a pin or a fixture, treat it as a pin and note
the ambiguity in the [Learnings log](#learnings-log).

**Beware the forbidden-terms whitelist.** `make check` runs
`scripts/check_forbidden_terms.py`, which forbids em dashes (U+2014) and a few
terms, with exceptions listed **by line number** in
`.forbidden-terms-whitelist.json`. If a spec edit *inserts or removes lines* in a
file that has whitelist entries (e.g. `global-architecture.spec.md`, whose Mermaid
control-plane node ids and an example GitOps path are whitelisted), those line
numbers shift and `make check` fails until the whitelist is updated. Always run
`make check` and fix the whitelist line numbers in the same change.

**Also check the sandbox base image** in `config.go`
(`defaultSandboxImage`). It currently floats on `:latest`
(`ghcr.io/nvidia/openshell-community/sandboxes/base:latest`), which is a separate
image line from gateway/supervisor and is NOT version-locked to the release. If
upstream starts publishing versioned sandbox base images, pin it here and add it
to the footprint table.

## Workflow

0. **Check for pending `needs-decision` issues.** Before resolving the target
   version, scan for open issues where a human already provided direction.
   Skip this step if `$ARGUMENTS` is non-empty (explicit target already given).

   ```bash
   gh api 'repos/openshift-online/hypershell/issues?labels=needs-decision&state=open&per_page=100' \
     --jq '.[] | [.number, .title] | @tsv'
   ```

   For each open `needs-decision` issue:

   a. Read all comments:
      ```bash
      gh api repos/openshift-online/hypershell/issues/<N>/comments \
        --jq '.[] | {author: .user.login, body, created_at}'
      ```

   b. Find the most-recent comment whose author does **not** end in `[bot]`.
      If none exists, the human has not yet replied - skip this issue and
      report "waiting for human direction on #N".

   c. Extract the version the human indicated (e.g. `v0.1.2-rhaiv.0`). Validate
      it matches `^v[0-9]` before using it. If it does not match, skip the issue
      and report "malformed version in human reply on #N". Set `RESOLVING_ISSUE=<N>`.

   d. Continue with the normal steps below using that target. On successful
      commit+PR (Step 8), close the issue:
      ```bash
      gh issue close "$RESOLVING_ISSUE" --repo openshift-online/hypershell \
        --comment "Resolved in <PR-URL>. The update to <version> is now open for review."
      ```

   If multiple `needs-decision` issues have human replies, process them
   sequentially (newest reply first).

1. **Resolve versions and obtain the image reference.**

   **Stable midstream track:**

   ```bash
   # Current pin (read from source-of-truth file)
   grep "GATEWAY_IMAGE" deploy/base/platform-resources/controller.yaml

   # Latest available midstream tag
   gh api repos/opendatahub-io/openshell/tags \
     --jq '[.[] | select(.name | test("^v[0-9]"))] | .[0].name'

   # Verify a specific tag exists
   gh api repos/opendatahub-io/openshell/git/ref/tags/<tag>
   ```

   The tag IS the image tag - no transformation needed. Target image:
   `quay.io/opendatahub/odh-openshell-gateway:<tag>`

   **Exit condition - compare FULL tags including rhaiv suffix:**

   After resolving both versions, compare the full midstream tag strings:
   - If `latest_midstream_tag == current_pin` (exact string match) → **exit, nothing to update.**
   - If they differ for **any reason** - different base version, different rhaiv patch number,
     or different rhaiv suffix format - → **proceed with the update.**

   A rhaiv patch increment (`rhaiv.0` → `rhaiv.2`) on the same NVIDIA base version IS
   still an update and must be applied. The NVIDIA base version comparison in Step 2 is
   **FOR TRIAGE ONLY** (to find which release notes to read); it is NOT the update trigger.
   Never conclude "no update needed" because the NVIDIA base matches - compare the full tags.

   **Agent build track (unmerged upstream branch/commit):**

   Invoke the HyperShell build agent, specifying the NVIDIA/OpenShell branch or
   commit SHA. The agent returns a full image reference (with digest). Use that
   reference verbatim as the pin - it acts like a midstream tag for all subsequent
   steps in this workflow.

2. **Triage the release range.** For every midstream tag between current and
   target (exclusive of current, inclusive of target), check the corresponding
   upstream NVIDIA release notes. The midstream `rhaiv.M` tags map to a NVIDIA
   base version; the rhaiv patch increments are Red Hat fixes on top:

   ```bash
   # Extract the NVIDIA base version from a rhaiv tag (e.g. v0.0.116-rhaiv.15 -> v0.0.116)
   BASE=$(echo "<TAG>" | sed 's/-rhaiv\..*//')

   # Read NVIDIA release notes for the base and any versions in the range
   gh api repos/NVIDIA/OpenShell/releases/tags/$BASE --jq '.name, .body'
   ```

   Classify each change against the [Contract surfaces](#contract-surfaces-to-triage).
   Produce a short **impact report**: `mechanical` (tag bump is enough) vs
   `needs-decision` (upstream changed a contract HyperShell renders, or shipped a
   feature that overlaps something HyperShell hand-rolls). Surface every
   `needs-decision` item to the user before finalizing - do not silently absorb it.

   For agent-built commits, triage the diff between the current pin and the target
   commit instead of release notes:
   ```bash
   gh api repos/NVIDIA/OpenShell/compare/<current-base-sha>...<target-sha> \
     --jq '.files[].filename'
   ```

3. **Bump the pins.** Edit `deploy/base/platform-resources/controller.yaml` first,
   then sweep the rest of
   the [Version footprint](#version-footprint). Per the repo convention *"Image
   references must match across the stack"*, grep all overlays and manifests too:

   ```bash
   grep -rn "odh-openshell-\(gateway\|supervisor\)" deploy/
   grep -rn "openshell/\(gateway\|supervisor\)" deploy/                # older ghcr.io image names
   ```

3a. **Vendor the Helm chart.** The Dockerfile packages the chart from
   `charts/openshell/` at build time - no network access is allowed inside
   Konflux hermetic builds. After bumping `OPENSHELL_TAG` in `OPENSHELL_VERSION`,
   run the vendor target:

   ```bash
   make vendor-openshell-chart
   ```

   This clones the upstream tag declared in `OPENSHELL_VERSION`, replaces
   `charts/openshell/` wholesale, and strips em-dashes (U+2014) which the
   pre-commit hook rejects. The resulting diff will be ~48 files and thousands
   of lines - that is expected. Every bump will look like this.

   After vendoring, check whether the new chart adds RBAC rules that the
   controller does not yet hold. Any permission the chart's ClusterRole or Role
   grants must also be present in
   `deploy/base/platform-resources/controller-rbac.yaml`, or Kubernetes will
   reject the apply with an RBAC escalation error:

   ```bash
   # Quick check: list all verbs/resources from the vendored chart's RBAC templates
   grep -r "resources:\|verbs:" charts/openshell/templates/ | grep -v '#'
   ```

3b. **Check the console image.** The console dashboard
   (`quay.io/gkrumbach07/openshell-dashboard`) has its own tagging scheme and is
   NOT locked to `OPENSHELL_TAG`. It only needs a bump when the gateway's
   workspace/sandbox gRPC proto changes (breaking proto changes cause the
   dashboard BFF to hang loading sandbox lists with no visible error). Check
   whenever triage (Step 2) flags a proto or gRPC surface change:

   ```bash
   # List available console image tags (newest first)
   curl -s "https://quay.io/api/v1/repository/gkrumbach07/openshell-dashboard/tag/?limit=20" \
     | python3 -c "import sys,json; [print(t['name'],t.get('manifest_digest','')) for t in json.load(sys.stdin)['tags']]"
   ```

   When updating, set both files - they must agree:
   - `OPENSHELL_CONSOLE_DIGEST` in `OPENSHELL_VERSION`
   - `defaultConsoleImage` in `components/control-plane/internal/gateway/config.go`
     (also update the `sha-XXXXXXX` tag name in the comment on the line above)

   Symptom of a stale console image after a proto-breaking bump: the Sandboxes
   tab shows an infinite loading spinner that never resolves, with no error in the
   browser console. The gateway itself is healthy - only the BFF gRPC call hangs.

4. **Verify contracts.** For each `needs-decision` item from step 2, check the
   thing it touches:
   - **Gateway config (`gateway.toml`)**: diff the keys the control plane renders
     (`components/control-plane/manifests/gateway/configmap.yaml`,
     `components/control-plane/internal/gateway/config.go`) against the new
     gateway's expected config. A renamed/removed/added key is a code change, not
     a tag bump.
   - **Sandbox API version**: confirm the `agents.x-k8s.io` API version the new
     gateway requires still matches the RBAC and manifests
     (`components/control-plane/manifests/gateway/rbac.yaml`,
     `.../networkpolicy.yaml`, `deploy/base/controller-rbac.yaml`).
   - **Credential drivers**: if upstream changed the pluggable credential storage
     surface, re-check `ValidateCredentialDriverConfig` and the credentials spec.
   - **PKI / TLS / Route**: if upstream shipped ingress/PKI features, evaluate
     against HyperShell's own approach (see the [Learnings log](#learnings-log)).

5. **Build and test.**
   ```bash
   cd components/control-plane && go build ./... && go vet ./... && go test ./...
   make check
   ```
   For ROKS, note that the new images must be **re-mirrored** into the internal
   registry before they can be pulled - see
   [`ibm-cluster`](../../deploy/ibm-cluster/SKILL.md). This skill does not perform
   the mirroring; it only updates the tags the mirror commands reference.

6. **Update specs.** Bump the version citations (footprint table) and correct any
   spec claim the triage invalidated (e.g. the `v1beta1` API-version note in
   `global-architecture.spec.md`).

7. **Feed the loop.** Before committing, complete
   [Self-reinforcement](#self-reinforcement) below.

8. **Commit + report.** Conventional commit
   (`chore(deps): bump OpenShell to <version>`), summarize the impact report in
   the body, and open follow-up issues for any `needs-decision` item deferred for
   a maintainer call. If this run was triggered by a pending `needs-decision` issue
   (Step 0), close it now with a link to the PR.

## Contract surfaces to triage

A release is only a mechanical tag bump if it touches **none** of these. If it
does, the item is `needs-decision`:

| Surface | Why it matters | Where HyperShell depends on it |
|---------|----------------|--------------------------------|
| Gateway/supervisor config schema (TOML) | Control plane renders `gateway.toml` | `manifests/gateway/configmap.yaml`, `internal/gateway/config.go` |
| Sandbox CR / `agents.x-k8s.io` API version | Gateway manages sandboxes; RBAC grants on it | `manifests/gateway/rbac.yaml`, `networkpolicy.yaml`, `deploy/base/controller-rbac.yaml` |
| Credential storage drivers | HyperShell selects/validates drivers | `ValidateCredentialDriverConfig`, `openshell-gateway-credentials.spec.md` |
| gRPC/proto surface | API server + control plane speak gRPC; console BFF calls gateway gRPC directly for sandbox/workspace ops | `components/api-server/proto/`; also check console image compatibility (Step 3b) when proto changes are in triage |
| Gateway/supervisor CLI flags & env | Control plane sets them | `configmap.yaml`, deployment manifests |
| PKI / TLS / ingress (Route, cert-manager, Gateway API) | HyperShell hand-rolls per-tenant PKI + ingress | `internal/gateway/` reconciler, ingress specs |
| Auth (OIDC) | OIDC is the client auth mechanism (no client mTLS) | `ValidateOIDCConfig`, gateway OIDC config |

## Verification

Before considering the update done:

1. `grep -rn "<OLD_VERSION>" .` returns only intentional fixtures (and this
   line, and the Learnings log if it records an old version).
2. `config.go` and every footprint file agree on the new version.
3. `go build ./...`, `go vet ./...`, `go test ./...`, and `make check` pass.
4. Every `needs-decision` item is either resolved in this PR or filed as a
   tracked follow-up - never silently dropped.
5. [Self-reinforcement](#self-reinforcement) is complete.

## Self-reinforcement

This is the step that keeps the skill from decaying. It is **mandatory** on every
run:

1. **Any file that was pinned but missing from the footprint table** -> add it to
   the table.
2. **Any occurrence that was a fixture (should not change) but looked like a
   pin** -> note it explicitly in the table's "do not change" list.
3. **Any contract drift, build break, test failure, or manual fix** encountered
   -> add a dated entry to the [Learnings log](#learnings-log) describing the
   symptom and the fix, and update the affected spec so the fact lives in the
   desired-state docs too.
4. **Any judgment call surfaced to the user** (adopt upstream feature vs keep
   HyperShell's approach) -> record the decision and its rationale in the
   Learnings log.

If a run produced no new lessons, that is itself worth a one-line log entry
(`vX.Y.Z: clean mechanical bump, no drift`) so the cadence is visible.

## Learnings log

Newest first. Each entry: version, date, what happened, what changed in the repo.

- **v0.1.2-rhaiv.7 (2026-10-08, v0.1.2-rhaiv.0 -> v0.1.2-rhaiv.7, user-directed):**
  Clean mechanical bump. NVIDIA v0.1.2 release notes cover only docs, supervisor network fix,
  homebrew config migration, sandbox reaper perf - no contract surface changes. No RBAC
  escalation (runtimeclasses/priorityclasses/pods were already in controller-rbac.yaml from
  v0.1.2-rhaiv.0). `go build/vet/test` and `make check` all passed with no code changes.
  Footprint: OPENSHELL_VERSION, deploy/base/control-plane/deployment.yaml,
  deploy/base/platform-resources/controller.yaml, deploy/ibm/kustomization.yaml,
  components/pr-test/e2e-openshell-roks.sh, specs/platform/openshell-gateway.spec.md,
  specs/platform/data-model.spec.md, specs/platform/openshell-gateway-credentials.spec.md,
  specs/platform/openshell-image-auto-update.spec.md, skills/deploy/ibm-cluster/SKILL.md,
  skills/deploy/deploy-cluster/SKILL.md, skills/deploy/gcp-cluster/SKILL.md,
  charts/openshell/ (vendored via make vendor-openshell-chart).
  Digests: gateway sha256:3d1a91222f402567662178944640985dbb1ae4958c8c8c0096d3bbd2eb7c2c16,
  supervisor sha256:7e81f0c38e18c076472ec2364494c3ae6481d421b60d771e0f04f21039e7ef8e,
  sandbox sha256:9188961848d3952f6343132a1746e18cf862f106454a3cb803c12ddfd5b7990a.
  Console image unchanged (no proto surface change in v0.1.2 rhaiv patches).
  Issue #366 not closed - it predates this run and remains open tracking the rhaiv.2
  contract-surface issue; that issue's notes are superseded by the clean rhaiv.7 bump.

- **Skill correction (2026-09-30, rhaiv-exit-condition bug):** Agent ran the nightly
  update-openshell job with current pin `v0.1.2-rhaiv.0` and latest midstream tag
  `v0.1.2-rhaiv.2`. It correctly fetched both, but then concluded "no update needed"
  because the NVIDIA upstream base release (`v0.1.2`) already matched the current pin's
  base. The agent conflated the triage BASE version comparison (Step 2, for finding
  release notes) with the update trigger. Root cause: the skill had no explicit exit
  condition comparing FULL midstream tags; the NVIDIA base-version check appeared to be
  the deciding gate. Fix: added an explicit "Exit condition" block in Step 1 that mandates
  comparing full tag strings (including rhaiv suffix) and explicitly states the NVIDIA
  base comparison is triage-only, not the update trigger. A rhaiv patch bump on the same
  NVIDIA base is always an update.

- **v0.1.2-rhaiv.0 (2026-09-28, v0.0.116-rhaiv.6 -> v0.1.2-rhaiv.0, triggered by Step 0 / issue #366):**
  This is the first minor-version midstream bump (0.0.x -> 0.1.x). Triggered automatically:
  Step 0 found issue #366 (`needs-decision`, filed 2026-09-25) with a human reply from
  `markturansky` ("Midstream has v0.1.2-rhaiv.0 as the latest tracking upstream 0.1.2").
  - **Mechanical pin bump only.** `go build/vet/test` and `make check` all passed with no
    code changes. The breaking upstream changes (`refactor(proto)!: use well-known time types`
    #3113, `fix(policy)!: require explicit L7 append targets` #3380, Agent Sandbox v1.0.3)
    are in the NVIDIA upstream and visible in `opendatahub-io/openshell` commit history,
    but the HyperShell control-plane build did not break - the generated proto in
    `components/api-server/proto/` and the gateway configmap templates did not need changes
    to compile and pass tests at this version.
  - **`needs-decision` deferred:** The Agent Sandbox API version bump (v1.0.x, potentially
    off `v1beta1`) remains unverified against the live image. Flag for next ROKS deploy.
  - **Step 0 worked as designed.** The skill auto-detected issue #366's human reply, extracted
    `v0.1.2-rhaiv.0`, ran the full workflow, and closed the issue on success.
  - **opendatahub-io/openshell has no GitHub Releases, only tags.** Use `gh api repos/opendatahub-io/openshell/tags`
    (not `/releases`) to enumerate available versions. Attempting `/releases/tags/<tag>` returns 404.
  - **Image digests:** Use `skopeo inspect --no-creds docker://quay.io/opendatahub/odh-openshell-gateway:<TAG>`
    to retrieve the manifest digest. If skopeo is unavailable, the Quay v1 tag API returns the
    `manifest_digest` field for a given `specificTag` query parameter.
  - **Chart vendoring is required on every bump.** `charts/openshell/` must be
    replaced from the upstream tag. If it's stale, the Dockerfile packages the old
    chart silently - new RBAC rules, value schema changes, and template fixes are
    not applied even though the image tag was bumped. Run `make vendor-openshell-chart`
    after editing `OPENSHELL_VERSION` (see Step 3a).
  - **Chart RBAC escalation.** v0.1.2 added a `node-reader` ClusterRole (grants
    `runtimeclasses` get, `priorityclasses` get) and a sandbox Role (grants `pods`
    create/delete/patch). Both had to be added to `controller-rbac.yaml` to avoid
    RBAC escalation errors on apply. On every bump, scan the new chart's RBAC
    templates and cross-check against the controller's ClusterRole.
  - **Console image must be checked when proto surfaces change.** v0.1.2 changed
    the workspace/sandbox gRPC proto (`refactor(proto)!: use well-known time types`
    #3113). The old console image (`sha-978bcb5`) could not parse v0.1.2 gateway
    responses - the sandbox list UI showed an infinite loading spinner with no
    error. Updated to `sha-71335e5` (built 2026-09-23,
    `sha256:1d36331138c37aa75285869a21d2aa35ebdab5b1f6980f028b28df405ec5a927`).
    Both `OPENSHELL_CONSOLE_DIGEST` in `OPENSHELL_VERSION` and `defaultConsoleImage`
    in `config.go` must be updated together and must agree. See Step 3b.
  - **`appArmorProfile` in Kind.** v0.1.2 chart sets
    `securityContext.appArmorProfile: type: RuntimeDefault` on pods. Kind does not
    support AppArmor, so pods fail to schedule. Suppressed via a Helm values
    override in the control plane's values builder. On future bumps, check whether
    the chart adds new Linux security context fields that Kind does not support.

- **Skill correction (2026-09-25, HYPERSHELL-301):** Skill had two structural
  errors discovered during build-agent work:
  - Wrong source repo: skill pointed to `NVIDIA/OpenShell` for tag discovery, but
    the deployed images (`quay.io/opendatahub/odh-openshell-*`) come from the
    `opendatahub-io/openshell` midstream with `v0.0.NNN-rhaiv.M` tag format. The
    NVIDIA repo is still needed for release-note triage but is NOT the image source.
    Correct command: `gh api repos/opendatahub-io/openshell/tags --jq '[.[] | select(.name | test("^v[0-9]"))] | .[0].name'`
  - Wrong source-of-truth file path: skill said `deploy/base/controller.yaml`
    (does not exist). Correct path: `deploy/base/platform-resources/controller.yaml`
    (the former `control-plane/deployment.yaml` duplicate was removed).
  - Added build-agent track for testing unmerged upstream branches/commits.
  - Current pin at time of correction: `v0.0.116-rhaiv.6`; latest midstream: `v0.0.116-rhaiv.15`.

- **v0.0.109 (2026-08-19, second run, 0.0.106 -> 0.0.109; validated on ROKS):**
  Unlike 106, this bump was NOT config-schema-neutral - three regressions only
  surfaced running the full ROKS e2e (`components/pr-test/e2e-openshell-roks.sh`),
  which ended at **22/22 passing**. Pin-only diffing would have missed all three;
  the lesson is to run the live sandbox e2e on a version bump, not just `make check`.
  - **Sandbox client TLS is required in `combined` topology.** A prior mTLS-removal
    sweep had conflated two distinct concerns and deleted BOTH: (A) external-client
    mTLS (`client_ca_path`, `tls-client-ca` volume) - correctly removed for
    Route+OIDC; and (B) sandbox client-TLS provisioning (`client_tls_secret_name` +
    the `openshell-client` cert-manager Certificate) - **wrongly** removed. 0.0.109's
    Combined topology needs (B) so sandbox runners get `OPENSHELL_TLS_CA` to verify
    the gateway server cert; without it the sandbox agent crash-loops on
    `OPENSHELL_TLS_CA is required`. Restored the `openshell-client` Certificate in
    `components/control-plane/internal/gateway/reconciler.go` and
    `client_tls_secret_name` in `manifests/gateway/configmap.yaml`, each with a
    comment stating it is internal sandbox↔gateway TLS, NOT external mTLS.
  - **StatefulSet/Deployment collision.** `manifests/gateway/statefulset.yaml` was
    still in the deploy `order` slice alongside the Deployment, racing it and
    leaving an orphaned crash-looping `openshell-gateway-0`. Removed the entry from
    the order slice in `reconciler.go` and `git rm`'d the file (spec: "Always
    Deployment"). Verify a fresh tenant NS has a Deployment and no StatefulSet.
  - **Workspace membership is a second, non-claim-derived authz layer.** 0.0.109
    gates `sandbox create` on BOTH the OIDC role AND an explicit workspace
    membership record. A standard `openshell-user` is not implicitly a member of
    `default`, so create fails with `not a member of workspace 'default'` until an
    admin runs `openshell workspace member add --workspace default --subject <sub>
    --role user`. Added that grant to the ROKS e2e's developer-RBAC step (mirrors
    `tests/e2e/e2e-openshell.sh`) and documented it in `ibm-cluster/SKILL.md` §5.9.
  - **No downloadable 0.0.109 CLI.** `install.sh` with `OPENSHELL_VERSION=0.0.109`
    404s; only the gateway *container image* is published at 0.0.109. The 0.0.98
    CLI reads the same config format and drives a 0.0.109 gateway fine (and has the
    `workspace` subcommand that the ancient system `/bin/openshell` 0.0.55 lacks),
    so the e2e now defaults `OPENSHELL` to `~/.local/bin/openshell`.
  - **v1beta1 confirmed** (the 106-entry's unverified claim): the running 0.0.109
    gateway serves sandboxes via `agents.x-k8s.io/v1beta1` (agent-sandbox v0.5.5),
    matching the note in `global-architecture.spec.md` / `ibm-cluster/SKILL.md`.

- **v0.0.106 (2026-08-19, initial skill + first run, 0.0.101 -> 0.0.106):**
  - Range 102-106 was mechanically safe for the pin bump; no config-schema,
    Sandbox-API, or credential-driver break observed in the release notes.
  - `needs-decision` surfaced: **v0.0.106 shipped upstream
    "cert-manager external issuer + OpenShift passthrough Route"
    (NVIDIA/OpenShell#2468)**, which overlaps HyperShell's hand-rolled per-tenant
    self-signed CA + passthrough Route. Recorded as a follow-up to evaluate
    adopting upstream's issuer instead of the self-signed root (ties to the PKI
    hardening follow-up and the OIDC-over-mTLS decision). NOT adopted in the bump.
  - `defaultSandboxImage` floats on `:latest` and is not tied to the release tag;
    left as-is, flagged in the footprint.
  - The example tag in `validation_test.go` is a regex fixture, not a pin - do not
    bump it.
  - **Footprint gap found by discovery grep:**
    `specs/platform/openshell-gateway-database.spec.md` was NOT in the original
    footprint table AND pinned the gateway image to a **git SHA**
    (`21da343c9f...`) instead of a semver tag. Normalized it to `0.0.106` and
    added it to the table. Lesson: always run the discovery grep; the table is a
    checklist, not the source of truth.
  - **Historical fact, not a pin:** `openshell-gateway-credentials.spec.md`
    line ~12 ("v0.0.101 introduced pluggable credential storage drivers") must
    not be bumped. Added to the do-not-change list.
  - **`make check` gotcha (cost real time):** two prior spec commits on this
    branch used em dashes (forbidden) and inserted lines that shifted the
    `.forbidden-terms-whitelist.json` line numbers for the Mermaid control-plane
    node ids and an example GitOps path. `make check` failed until em dashes were
    replaced with ` - `
    and the whitelist line numbers were corrected (27/76/77/912 -> 35/84/85/925).
    Now documented in the "Beware the forbidden-terms whitelist" note above.
  - **Build hygiene:** `go build` failed on an unrelated accidental paste (two
    stray URLs appended to `reconciler.go` in the working tree, uncommitted).
    Removing it unblocked the build. Lesson: run `go build`/`go vet`/`go test`
    even for a "docs + pin" change - the working tree may carry unrelated breakage
    the pin bump would otherwise mask.
  - **Unverified claim:** the `v1beta1` Sandbox API-version note (in
    `global-architecture.spec.md` and `ibm-cluster/SKILL.md`) was version-bumped
    to "gateway 0.0.106 prefers v1beta1" by substitution. Release notes 102-106
    show sandbox changes (105 stop/start, 103 policy revisions) but no
    API-version bump, so v1beta1 almost certainly still holds - but this was NOT
    independently verified against the 0.0.106 image. Verify against the running
    gateway on the next ROKS deploy and correct if wrong.
