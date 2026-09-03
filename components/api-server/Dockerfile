# syntax=docker/dockerfile:1
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:4c7c064cc73698c13384d1445ab93595f3b9309dd89ed45b9992e67284e3ed82 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod edit -dropreplace github.com/openshift-online/rh-trex-ai && go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/
COPY plugins/ plugins/
COPY openapi/ openapi/

ARG GIT_VERSION=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -mod=mod -ldflags="-s -w \
  -X github.com/openshift-online/hypershell/components/api-server/pkg/api.Version=${GIT_VERSION} \
  -X 'github.com/openshift-online/hypershell/components/api-server/pkg/api.BuildTime=${BUILD_TIME}'" \
  -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/static:1787099997@sha256:f4d5109b57cf7eab0a7adc566f2d78f80fa0c5ec9ccab698c9fb8eb448db6071

COPY --from=builder /workspace/hypershell /usr/local/bin/

EXPOSE 8000
EXPOSE 9000

ENTRYPOINT ["/usr/local/bin/hypershell"]
CMD ["serve"]

LABEL org.opencontainers.image.title="HyperShell API Server" \
      org.opencontainers.image.description="REST + gRPC API server for HyperShell fleet management" \
      org.opencontainers.image.version="0.0.1"
