# Control-Plane Reconciliation Metrics

**Status:** Draft
**Version:** 0.1.0
**Created:** 2026-09-23
**Tracks:** [HYPERSHELL-286](https://redhat.atlassian.net/browse/HYPERSHELL-286)

## Purpose

Expose control-plane reconciliation health to the reliability dashboard through
Prometheus and the web-console BFF without introducing a dashboard database.

## Requirements

### Requirement: CRM-001 -- Prometheus Metric Types

The control plane SHALL expose the following fleet-wide Prometheus metrics:

| Metric | Type | Meaning | Unit |
| --- | --- | --- | --- |
| `hypershell_reconciliation_failures_total` | Counter | Failed reconciliation attempts | count |
| `hypershell_reconciliation_retries_total` | Counter | Reconciliation retries | count |
| `hypershell_reconciliation_lag_seconds` | Histogram | Time from reconciliation eligibility to completion | seconds |
| `hypershell_stale_resource_status_count` | Gauge | Resources whose observed status is stale | count |

The control plane SHALL expose the metrics through its Prometheus scrape
endpoint. The dashboard SHALL NOT read reconciliation data from a database.

### Requirement: CRM-002 -- Prometheus Labels

The counters and histogram SHALL use only bounded labels approved by the
control-plane observability conventions. The metrics SHALL NOT include resource
IDs, customer IDs, names, or other unbounded-cardinality labels.

The failure and retry counters MAY include a bounded reconciliation-kind label.
The lag histogram MAY include a bounded reconciliation-kind label. The stale
resource gauge MAY include a bounded resource-kind label.

### Requirement: CRM-003 -- Historical Query Contract

The BFF SHALL query the metrics through Prometheus `query_range` over the most
recent **24 hours** with a **3600-second** step.

The BFF SHALL calculate:

- failure counts from `increase(hypershell_reconciliation_failures_total[24h])` for current values and `increase(hypershell_reconciliation_failures_total[1h])` for hourly samples;
- retry counts from `increase(hypershell_reconciliation_retries_total[24h])` for current values and `increase(hypershell_reconciliation_retries_total[1h])` for hourly samples;
- median lag from `histogram_quantile(0.50, ...)` over the histogram buckets when
  a valid sample exists; and
- stale resource status count from the gauge value.

The BFF SHALL use the same aggregation dimensions for instant and range
queries. The BFF SHALL omit a historical series when Prometheus returns an
invalid or unavailable sample instead of converting the sample to zero.

### Requirement: CRM-004 -- BFF Response

The BFF SHALL expose `GET /api/metrics/control-plane-reconciliation` and SHALL
return current values plus optional historical arrays with UTC hourly labels.
The failure, retry, and stale-resource current queries are required and SHALL
return HTTP `502` when they fail. The current lag query is optional: when it
returns no valid sample, the response SHALL omit the lag field while preserving
the other current values. Failure of one historical query SHALL omit only that
historical array while preserving successful current values and sibling
historical arrays.

### Requirement: CRM-005 -- Verification

Tests SHALL verify metric registration, histogram bucket export, bounded label
sets, PromQL aggregation, 24-hour range parameters, response mapping, and
failure behavior.

## Assumptions

- Prometheus retention is at least 24 hours.
- HYPERSHELL-285 API reliability metrics are available to the dashboard. This
  specification intentionally reuses and standardizes their shared dashboard
  presentation as `requests/sec` with three fractional digits; these are
  compatibility changes to the existing reliability-dashboard contract.
