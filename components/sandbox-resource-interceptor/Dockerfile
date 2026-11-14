# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/go:1.26.7 and hi/static:1787099997,
# matching components/control-plane/Dockerfile. BUILDARCH selects a native Go
# toolchain; TARGETARCH selects the runtime and GOARCH. Bump amd64 and arm64
# together when changing tags.
ARG BUILDARCH
ARG TARGETARCH

FROM registry.access.redhat.com/hi/go:1.26.7@sha256:2a2215ce496fdd33fdc19739eaadde902f7719c8be222f22f92fd7da02c889cb AS go-amd64
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:ba850dda34c47052bb8db5561c80c43fb08d0f9a729c90fddd755a2d5b46f74b AS go-arm64
FROM go-${BUILDARCH} AS builder

WORKDIR /workspace

COPY components/sandbox-resource-interceptor/go.mod components/sandbox-resource-interceptor/go.sum components/sandbox-resource-interceptor/
RUN cd components/sandbox-resource-interceptor && go mod download

COPY components/sandbox-resource-interceptor/cmd/ components/sandbox-resource-interceptor/cmd/
COPY components/sandbox-resource-interceptor/internal/ components/sandbox-resource-interceptor/internal/
COPY components/sandbox-resource-interceptor/pkg/ components/sandbox-resource-interceptor/pkg/

ARG TARGETARCH
RUN cd components/sandbox-resource-interceptor && \
    CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -ldflags="-s -w" \
    -o /workspace/sandbox-resource-interceptor ./cmd/sandbox-resource-interceptor

FROM registry.access.redhat.com/hi/static:1787099997@sha256:3d43712a61ce01c7049e983e7f4c4609aaad7582c7bbd930b57372f82c446f98 AS static-amd64
FROM registry.access.redhat.com/hi/static:1787099997@sha256:2ccd532da5e8868a92f479a7268a2fce65279c1f7c8a8dd0880d399c04d6a356 AS static-arm64
FROM static-${TARGETARCH}

COPY --from=builder --chown=1001:0 /workspace/sandbox-resource-interceptor /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/sandbox-resource-interceptor"]

LABEL org.opencontainers.image.title="HyperShell Sandbox Resource Interceptor" \
      org.opencontainers.image.description="Gateway sidecar that defaults and floors sandbox workload resources" \
      org.opencontainers.image.version="0.0.1"
