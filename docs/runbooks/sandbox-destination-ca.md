# Sandbox destination TLS trust

The sandbox supervisor verifies upstream TLS independently of the workload.
`curl -k` inside a sandbox cannot disable the supervisor's verification. Corporate
HTTPS destinations therefore need corporate CA roots in the supervisor trust store.

## Configure

Create an operator-owned ConfigMap in the controller namespace containing a public
PEM certificate bundle under `ca.crt` (`ca-bundle.crt` is also accepted). Set the
controller environment variable `GATEWAY_ADDITIONAL_CA_CONFIGMAP_NAME` to its name.
No private keys belong in this ConfigMap. Install only roots approved for this
cluster. This setting is separate from OIDC, Vault, and gateway mTLS trust.

The controller validates the bundle, copies a content-addressed ConfigMap into
each gateway namespace, and sets the chart value
`supervisor.network.additionalCaConfigMapName`. Missing or invalid configured
bundles fail reconciliation rather than silently dropping the requested trust.

Controller startup checks existing gateways even when their API generation has
already converged. Changing bundle contents changes the gateway's volume reference
and rolls the gateway. Restart the controller after updating the source ConfigMap
to ensure existing gateways are rechecked; there is no ConfigMap watch for this.
Old CA ConfigMap generations remain available until gateway namespace cleanup.

## Rollout and verification

1. Deploy the controller image containing this support before enabling the setting.
2. Apply the ConfigMap and controller environment setting through GitOps.
3. Check controller reconciliation and gateway Deployment rollout.
4. Stop and start affected sandboxes through OpenShell so their supervisors acquire
   the new trust generation. This interrupts their running processes; coordinate
   with owners. Do not delete sandboxes or durable workspaces.
5. Retry the allowed corporate HTTPS destination from inside the sandbox and check
   supervisor logs for upstream certificate errors.

The bundle does not authorize destinations. A matching network policy must still
allow the hostname, port, and binary. Private corporate destinations also require
VPN routing and DNS. Advisor-proposed rules restrict private addresses separately.

To remove extra trust, remove the controller setting and explicitly reconcile each
affected gateway (an API spec update advances its generation), then stop/start its
sandboxes. With the setting absent, the normal convergence gate remains in place;
removing the setting alone does not roll already-converged gateways.
