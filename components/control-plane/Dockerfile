# syntax=docker/dockerfile:1
ARG BUILD_PREFIX=dev
ARG VCS_REF=unknown

FROM registry.access.redhat.com/hi/go:1.26.7@sha256:4c7c064cc73698c13384d1445ab93595f3b9309dd89ed45b9992e67284e3ed82 AS builder

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

FROM registry.access.redhat.com/hi/static:1787099997@sha256:f4d5109b57cf7eab0a7adc566f2d78f80fa0c5ec9ccab698c9fb8eb448db6071

ARG BUILD_PREFIX
ARG VCS_REF

COPY --from=builder /workspace/hypershell-controller /usr/local/bin/
COPY components/control-plane/manifests/ /manifests/

ENV HYPERSHELL_BUILD_VERSION="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}" \
    HYPERSHELL_BUILD_REVISION="${VCS_REF}"

ENTRYPOINT ["/usr/local/bin/hypershell-controller"]

LABEL org.opencontainers.image.title="HyperShell Controller" \
      org.opencontainers.image.description="Control plane that reconciles HyperShell resources via gRPC watch streams" \
      org.opencontainers.image.version="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}" \
      org.opencontainers.image.revision="${VCS_REF}"
