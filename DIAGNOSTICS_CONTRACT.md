# E2E Failure Diagnostics Wire Contract

## Summary

Failure diagnostics are **lazy** and are **no longer inline** on `GET /api/e2e/runs/{id}`.

1. **`GET /api/e2e/runs/{id}` (`E2ERunDetail`)**:
   - Does **not** query Loki for diagnostic bodies and does **not** return a `diagnostics` array on the run-detail payload.
   - Keeps the diagnostics step (`stepId: "diagnostics"`, `name: "failure diagnostics"`) in `data.steps` with its real step state (`"passed"` | `"failed"` | `"skipped"` | `"not-run"` | `"running"`). The UI uses this step state to decide whether to enable or grey-out/disable the "Show Diagnostics" dropdown without fetching diagnostic bodies.

2. **`GET /api/e2e/diagnostics?run=<run-id>` (`E2EDiagnosticsPayload`)**:
   - The **only** endpoint that queries Loki for failure-diagnostics records (`event:"diagnostic"` chunks and `stepId:"diagnostics"` / `stepId:"failure-diagnostics"` / `DIAG <title>\n<body>` log lines).
   - Uses the same single `run` query parameter scheme (`<tenant>/<namespace>/<runId>`) as `GET /api/e2e/logs?run=<run-id>`.

## Wire Shape (`GET /api/e2e/diagnostics?run=<tenant>/<namespace>/<runId>`)

The response is wrapped in the standard `cache.Snapshot` (`PlaneEnvelope<E2EDiagnosticsPayload>`) JSON envelope:

```ts
export type E2EDiagnosticSeverity = "info" | "warn" | "error";

export interface E2EDiagnosticSection {
  title: string;
  body: string;
  severity?: E2EDiagnosticSeverity;
}

export interface E2EDiagnosticsPayload {
  diagnostics: E2EDiagnosticSection[];
}
```

### Example HTTP 200 Response (With Diagnostics)

```json
{
  "data": {
    "diagnostics": [
      {
        "title": "All pods",
        "body": "hypershell/hypershell-api-server-xxx phase=Running ready=1/1 restarts=0",
        "severity": "info"
      },
      {
        "title": "Platform events",
        "body": "2026-11-04T12:00:05Z Warning BackOff Pod/openshell-gateway-0 Back-off restarting failed container",
        "severity": "warn"
      }
    ]
  },
  "generatedAt": "2026-11-04T21:45:00Z"
}
```

### Example HTTP 200 Response (No Diagnostics)

When a run has no diagnostic sections, `data.diagnostics` is always an empty array `[]` (never `null` or omitted):

```json
{
  "data": {
    "diagnostics": []
  },
  "generatedAt": "2026-11-04T21:45:00Z"
}
```

### Error & Feature-Off Responses

- **Feature disabled (`FD_E2E_LOKI_URL` unconfigured):** HTTP 200 with `{ "data": { "enabled": false }, "generatedAt": "..." }`.
- **Invalid `run` address or unauthorized tenant:** HTTP 404 with `{ "error": "run not found" }`.
- **Loki query failure:** HTTP 200 degraded snapshot with `{ "stale": true, "error": "e2e log backend unavailable", "generatedAt": "..." }`.
