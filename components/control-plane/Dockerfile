FROM registry.access.redhat.com/ubi9/go-toolset:1.26@sha256:46376c6723c3a4961a165c2768461e7fac48a79932cdde0a6e6a57724ef61ba0 AS builder

USER root

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

RUN cd components/control-plane && go build -mod=mod -ldflags="-s -w" -o /workspace/hypershell-controller ./cmd/hypershell-controller

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8@sha256:48fa5d8cda7fc00d270d8747c3eaa54ae196f0820d8540074a9c8c61d5e3056f

WORKDIR /

COPY --from=builder /workspace/hypershell-controller /usr/local/bin/
COPY components/control-plane/manifests/ /manifests/

USER 1001
ENTRYPOINT ["/usr/local/bin/hypershell-controller"]

LABEL name="hypershell-controller" \
      version="0.0.1" \
      summary="HyperShell Controller" \
      description="Control plane that reconciles HyperShell resources via gRPC watch streams"
