# Cost dashboard rate card

This optional component turns on the cost dashboard prices (WEB-COST-03 in
`specs/web-console/cost-dashboard.spec.md`). Without it, the console starts
normally and every cost widget shows "cost model not configured".

The component adds three things:

1. The `hypershell-cost-rate-card` ConfigMap, built from `rate-card.json`.
2. A volume and a read-only mount of that ConfigMap at `/etc/hypershell/cost`
   in the web console Deployment.
3. The `COST_RATE_CARD_FILE` environment variable, which points at the file.

`rate-card.json` holds the default prices: IBM Cloud Red Hat OpenShift on VPC
gen2 workers in us-east, November 2026 list prices (vCPU plus the OpenShift
license per vCPU, and memory).

## Use it in a site overlay

Add the component to the instance overlay, with a pinned repository revision:

```yaml
components:
  - https://github.com/openshift-online/hypershell/deploy/gitops-base/components/cost-rate-card?ref=<sha>
```

To use other prices, keep the component and replace the ConfigMap in the site
overlay (for example with a `configMapGenerator` using `behavior: replace`). A
site can also skip the component and provide its own ConfigMap, mount, and
`COST_RATE_CARD_FILE` setting.

The BFF re-reads the file when the ConfigMap changes, so a price change needs no
restart. The rate card is never sent to the browser.

The Kind and OpenShift dev overlays (`deploy/kind`, `deploy/openshift`) include
this component. `deploy/gitops/applications` and `deploy/hub` do not.
