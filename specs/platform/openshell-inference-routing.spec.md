# OpenShell Inference Routing Specification

**Date:** 2026-08-15
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-credentials.spec.md` - provider credential storage; `openshell-gateway-oidc.spec.md` - gateway authentication

---

## Purpose

This specification defines how agents running **inside OpenShell sandboxes** reach cloud inference models (Anthropic Claude on Vertex AI, Bedrock, OpenAI-compatible providers, etc.) **without any provider credential ever entering the sandbox**.

The platform requirement is a security guarantee: a sandbox is an untrusted execution environment for user/agent code, so the operator's cloud-provider credentials (GCP Vertex tokens, Anthropic keys) MUST NOT be readable from within it. OpenShell satisfies this with two distinct, upstream-provided mechanisms — a per-binary egress credential rewrite, and a workspace-scoped **inference router** at the virtual host `inference.local`. This spec covers both, defines when each applies, and specifies how HyperShell provisions and operates them.

The inference router itself is upstream OpenShell functionality. HyperShell's desired state is that (a) sandbox agents get credential-free cloud-model access, and (b) the operational configuration that enables it is reproducible per environment. Concrete per-cluster runbook steps live in the [`ibm-cluster`](../../skills/deploy/ibm-cluster/SKILL.md) skill.

---

## Architecture

### Two credential-delivery paths

```
                        ┌─────────────────────────── SANDBOX (untrusted) ───────────────────────────┐
                        │  agent binary (e.g. /usr/local/bin/claude)                                  │
                        └───────────────┬───────────────────────────────┬───────────────────────────┘
                                        │                               │
           PATH A: direct-to-upstream  │                               │  PATH B: inference router
        (allowlisted host, per-binary) │                               │  (ANTHROPIC_BASE_URL=https://inference.local)
                                        ▼                               ▼
                   ┌──────────────────────────────┐   ┌─────────────────────────────────────────────┐
                   │ Supervisor L7 egress proxy    │   │ Supervisor intercepts CONNECT               │
                   │ Rewrites the sentinel bearer  │   │   inference.local:443, TLS-terminates,      │
                   │   Authorization: Bearer       │   │   L7-routes to the inference router          │
                   │   openshell:resolve:env:<KEY> │   │ Router STRIPS client authorization/x-api-key│
                   │   → real stored credential    │   │   INJECTS operator provider token           │
                   │ Agent MUST emit the sentinel  │   │   TRANSLATES /v1/messages → provider shape  │
                   └───────────────┬───────────────┘   └───────────────────┬─────────────────────────┘
                                   ▼                                       ▼
                          upstream provider host                  upstream provider host
                     (api.anthropic.com, *.googleapis.com)    (e.g. *-aiplatform.googleapis.com :rawPredict)
```

- **Path A — direct-to-upstream (per-binary sentinel rewrite).** The sandbox connects straight to an allowlisted provider host. The supervisor's L7 proxy rewrites an `Authorization: Bearer openshell:resolve:env:<KEY>` sentinel into the real stored credential. The agent MUST emit the sentinel as its bearer. This path does not fit clients that manage their own provider auth (e.g. Claude Code in `CLAUDE_CODE_USE_VERTEX` mode uses Google ADC and never emits the sentinel).

- **Path B — inference router (`inference.local`).** `inference.local:443` is a virtual host intercepted by the supervisor proxy — not real DNS. The router receives the request, **discards the caller's credential**, **injects the operator-configured provider token server-side** (resolved by the gateway into the route bundle), and forwards. The client sends a throwaway key. This is the path for standard inference clients pointed at a custom base URL.

### Inference router surfaces

The router exposes standard inference APIs and adapts them to the configured provider:

| Client surface | Path | Adapted for provider |
|---|---|---|
| Anthropic Messages | `POST /v1/messages` | Vertex Anthropic (`:rawPredict`), direct Anthropic |
| OpenAI Chat Completions | `POST /v1/chat/completions` | Vertex OpenAI-compat, OpenAI, DeepInfra, NVIDIA |
| OpenAI Completions / Responses / Embeddings | `POST /v1/{completions,responses,embeddings}` | provider-dependent |
| Model discovery | `GET /v1/models`, `GET /v1/models/*` | provider-dependent |

### Anthropic-on-Vertex translation

For a `google-vertex-ai` provider serving a `claude-*` model, an incoming Anthropic Messages request is rewritten into Vertex's Anthropic partner contract:

```
POST https://{host}/v1/projects/{project}/locations/{location}/publishers/anthropic/models/{model}:rawPredict
  (streaming → :streamRawPredict)
```

The router removes the top-level `"model"` field (it moves into the URL path), injects `"anthropic_version": "vertex-2023-10-16"` into the body, and strips the `anthropic-beta` request header (Vertex rejects it).

---

## Requirements

### Requirement: Credential-free cloud-model access from sandboxes

Sandbox agents SHALL be able to invoke cloud inference models without any provider credential (API key, OAuth token, service-account material) being injected into, stored in, or readable from the sandbox.

#### Scenario: No provider credential in the sandbox environment

- GIVEN a `google-vertex-ai` provider configured on the gateway with a valid token
- AND a workspace inference route pointing `inference.local` at that provider
- WHEN an agent inside a sandbox calls the model via `https://inference.local`
- THEN the request SHALL succeed
- AND no GCP token, service-account key, or Anthropic key SHALL be present in the sandbox's environment, filesystem, or process arguments

#### Scenario: Caller-supplied credential is ignored

- GIVEN an inference route configured on `inference.local`
- WHEN a sandbox client sends any `Authorization` or `x-api-key` header (including a real secret or the sentinel)
- THEN the router SHALL strip that header before forwarding
- AND the router SHALL inject the operator-configured provider credential server-side
- AND the caller-supplied value SHALL NOT reach the upstream provider

---

### Requirement: Workspace inference route configuration

The platform SHALL support a workspace-scoped inference route that binds a provider and a forced model to the `inference.local` virtual host, configured via `openshell inference set --provider <name> --model <id>`.

| Route | Name | Audience |
|---|---|---|
| user-facing | `inference.local` | agent/user code in sandboxes |
| system | `sandbox-system` | platform functions (agent harness); not reachable by user code |

#### Scenario: User route configured for a Vertex Claude model

- GIVEN a `google-vertex-ai` provider `vertex-claude` holding a valid GCP token
- WHEN the operator runs `openshell inference set --provider vertex-claude --model claude-sonnet-4-5@<date>`
- THEN `openshell inference get` SHALL report the user route as configured with that provider and model
- AND requests to `https://inference.local/v1/messages` SHALL be served by that model

#### Scenario: Global-region routes skip client-side endpoint verification

- GIVEN a Vertex provider whose region is `global`
- WHEN the operator configures the route
- THEN `--no-verify` MAY be required so the CLI does not pre-flight-probe a region-specific endpoint that does not exist for `global`

---

### Requirement: Client request shape must match the provider API contract

Because the inference router performs a targeted field-level adaptation (not a full schema transpile), the client's request body fields SHALL be compatible with the upstream provider's API version. Newer client fields that the provider's validation rejects SHALL be avoided at the client, since the router does not strip them.

#### Scenario: Vertex Anthropic partner endpoint rejects newer client fields

- GIVEN Claude Code configured with `ANTHROPIC_BASE_URL=https://inference.local` against a Vertex Anthropic route (`anthropic_version = vertex-2023-10-16`, strict body validation)
- WHEN Claude Code uses an effort-capable model that emits `thinking: {type: "adaptive"}` and `output_config.effort`
- THEN Vertex SHALL reject the request with HTTP 400 (`thinking` tag mismatch; `output_config.effort` not permitted)
- AND selecting a non-effort model (e.g. `claude-sonnet-4-5`) SHALL suppress those fields and the request SHALL succeed

> These fields are gated on the model, not on an environment flag — no Claude Code
> env var strips them. Pinning a non-effort model is the supported lever. See the
> [`ibm-cluster`](../../skills/deploy/ibm-cluster/SKILL.md) skill for the exact recipe.

#### Scenario: Native-provider client mode must be disabled

- GIVEN Claude Code inside a sandbox
- WHEN it is configured for a provider's native auth mode (`CLAUDE_CODE_USE_VERTEX=1`)
- THEN it SHALL attempt Google Application Default Credentials (absent in the sandbox) and fail/hang
- AND the supported configuration SHALL instead use standard Anthropic mode with `ANTHROPIC_BASE_URL=https://inference.local` and a throwaway `ANTHROPIC_API_KEY`

---

## Client Configuration Reference

Point a standard inference client at the router; the key value is discarded:

```bash
# Anthropic Messages surface
ANTHROPIC_BASE_URL="https://inference.local" ANTHROPIC_API_KEY=unused claude --model claude-sonnet-4-5 --dangerously-skip-permissions

# OpenAI-compatible surface
ANTHROPIC_BASE_URL="https://inference.local/v1" ANTHROPIC_API_KEY=unused opencode
```

Persisting this for Claude Code (so bare `claude` works) via `~/.claude/settings.json` inside the sandbox:

```json
{
  "model": "claude-sonnet-4-5",
  "env": {
    "ANTHROPIC_BASE_URL": "https://inference.local",
    "ANTHROPIC_API_KEY": "unused"
  }
}
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| Client hangs, no HTTP response | `CLAUDE_CODE_USE_VERTEX=1` set → client tries Google ADC (none in sandbox) | Unset it; use standard mode + `ANTHROPIC_BASE_URL=https://inference.local` |
| `400 thinking: 'adaptive' does not match 'disabled'/'enabled'` | Effort-capable model emits adaptive thinking; Vertex `vertex-2023-10-16` rejects it | Pin a non-effort model: `--model claude-sonnet-4-5` |
| `400 output_config.effort: Extra inputs are not permitted` | Effort-capable model emits `output_config`; Vertex rejects it | Pin a non-effort model (same fix) |
| `404` model not found | Model ID not published for the GCP project/region | Use a published Vertex model ID (e.g. `claude-sonnet-4-5@<date>`) |
| `403` on inference call | GCP IAM deny on `aiplatform.endpoints.predict`, or provider token expired | Fix GCP IAM / refresh the provider credential (`openshell provider refresh`) |
| Route verification fails at `inference set` for `global` region | CLI pre-flights a region endpoint that does not exist for `global` | Add `--no-verify` |

---

## References

- [`ibm-cluster`](../../skills/deploy/ibm-cluster/SKILL.md) skill — ROKS runbook for provider + inference + sandbox agent wiring
- [OpenShell inference routing](https://docs.nvidia.com/openshell/latest/sandboxes/inference-routing)
- [OpenShell supported agents](https://docs.nvidia.com/openshell/latest/about/supported-agents)
- [Vertex AI Anthropic Claude models](https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude)
