# Vendored SDK generator

The core generator in this directory is vendored from
[`openshift-online/rh-trex-ai`](https://github.com/openshift-online/rh-trex-ai/tree/ee905fbd54a8653e16cfc2115ca684adf5d29bb4/scripts/sdk-generator)
at commit `ee905fbd54a8653e16cfc2115ca684adf5d29bb4`. The TypeScript `base` and
`main_client` templates are synchronized through commit
[`27d5882f534095fabd4cff65a1f01cc2f2a8b3f2`](https://github.com/openshift-online/rh-trex-ai/commit/27d5882f534095fabd4cff65a1f01cc2f2a8b3f2).

The vendored templates are unchanged. `executeTemplate` is locally adjusted
to emit exactly one trailing newline. HyperShell-specific paths, project
naming, and reproducible header normalization are implemented by
`components/api-server/Makefile`.

The upstream Apache License 2.0 is preserved in `LICENSE`.
