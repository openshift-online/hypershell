# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/go:1.26.7 and hi/static:1787099997.
# BUILDARCH selects a native Go toolchain; TARGETARCH selects the runtime and GOARCH.
# Bump amd64 and arm64 together when changing tags.
ARG BUILDARCH
ARG TARGETARCH

FROM registry.access.redhat.com/hi/go:1.26.7@sha256:2a2215ce496fdd33fdc19739eaadde902f7719c8be222f22f92fd7da02c889cb AS go-amd64
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:ba850dda34c47052bb8db5561c80c43fb08d0f9a729c90fddd755a2d5b46f74b AS go-arm64
FROM go-${BUILDARCH} AS builder

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -mod=mod -ldflags="-s -w \
  -X github.com/openshift-online/hypershell/components/api-server/pkg/api.Version=${GIT_VERSION} \
  -X 'github.com/openshift-online/hypershell/components/api-server/pkg/api.BuildTime=${BUILD_TIME}'" \
  -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/static:1787099997@sha256:3d43712a61ce01c7049e983e7f4c4609aaad7582c7bbd930b57372f82c446f98 AS static-amd64
FROM registry.access.redhat.com/hi/static:1787099997@sha256:2ccd532da5e8868a92f479a7268a2fce65279c1f7c8a8dd0880d399c04d6a356 AS static-arm64
FROM static-${TARGETARCH}

COPY --from=builder /workspace/hypershell /usr/local/bin/

EXPOSE 8000
EXPOSE 9000

ENTRYPOINT ["/usr/local/bin/hypershell"]
CMD ["serve"]

LABEL org.opencontainers.image.title="HyperShell API Server" \
      org.opencontainers.image.description="REST + gRPC API server for HyperShell fleet management" \
      org.opencontainers.image.version="0.0.1"
