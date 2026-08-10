FROM registry.access.redhat.com/hi/go:1.26.5-builder@sha256:fe0672d0b4bf43b502fe7ab4f251c266b558346cd3b8892c7c79b3b5f2c3a92f AS builder

WORKDIR /workspace

COPY components/api-server/go.mod components/api-server/go.sum components/api-server/
COPY components/control-plane/go.mod components/control-plane/go.sum components/control-plane/

RUN cd components/control-plane && GOPRIVATE=github.com/openshift-online/rh-trex-ai go mod download

COPY components/api-server/cmd/ components/api-server/cmd/
COPY components/api-server/pkg/ components/api-server/pkg/
COPY components/api-server/plugins/ components/api-server/plugins/
COPY components/api-server/openapi/ components/api-server/openapi/
COPY components/api-server/proto/ components/api-server/proto/
COPY components/control-plane/cmd/ components/control-plane/cmd/
COPY components/control-plane/internal/ components/control-plane/internal/

RUN cd components/control-plane && CGO_ENABLED=0 go build -mod=mod -ldflags="-s -w" -o /workspace/hypershell-controller ./cmd/hypershell-controller

FROM registry.access.redhat.com/hi/go:1.26.5@sha256:55d95d0bd29598f821931dec7155efd331068bdd84162d7d4b2e7ad66ec6c124

COPY --from=builder --chown=1001:0 /workspace/hypershell-controller /usr/local/bin/
COPY --chown=1001:0 components/control-plane/manifests/ /manifests/

USER 1001
ENTRYPOINT ["/usr/local/bin/hypershell-controller"]

LABEL name="hypershell-controller" \
      version="0.0.1" \
      summary="HyperShell Controller" \
      description="Control plane that reconciles HyperShell resources via gRPC watch streams"
