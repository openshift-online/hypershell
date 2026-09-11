# OpenShift dashboard metrics

This optional component owns the console deployment settings for OpenShift
metrics. It provides a dedicated service account, a service CA ConfigMap, a
projected token volume, and the cluster metrics environment variables. The pod
namespace supplies the gateway metrics filter through the Downward API.

Include this component in the instance overlay, with a pinned repository
revision. It can patch the existing `deploy/hub` base or the GitOps wrapper from
PR #251. It does not require the wrapper and does not change other installations.

The site overlay must:

- Set its instance namespace.
- Set `PROMETHEUS_URL` to its application metrics endpoint.
- Create a RoleBinding in `openshift-monitoring` to the existing
  `cluster-monitoring-metrics-api` Role, with subject ServiceAccount
  `hypershell-web-console-metrics` in the instance namespace.
- Use a console image that supports `CLUSTER_PROMETHEUS_*`.

Keep that RoleBinding outside the instance namespace transformer. Each site
owns the binding name, subject namespace, and application metrics endpoint.
Sites can override the cluster endpoint if needed. Keep the credential mounts
and deployment patch here; do not copy them into the site renderer.

The component uses the OpenShift service CA injection controller and the existing
Thanos Querier. It installs no metrics collectors. Cluster charts show the whole
cluster; gateway charts use the instance namespace. Existing ingress policies
from PR #251 do not prevent these outbound queries. Sites that restrict egress
must allow DNS and access to both configured metrics endpoints.

For hypbox, GitOps PR #119 consumes this component. Merge this source change and
publish its console image before merging the site configuration. Update the
component revision and console image digest together. PR #251 can merge in
either order; its wrapper does not enable this OpenShift component by default.
