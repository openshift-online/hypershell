# syntax=docker/dockerfile:1
# Chart packaging stage -- the -builder variant includes git, tar, gzip
FROM registry.access.redhat.com/hi/go:1.26.5-builder@sha256:fe0672d0b4bf43b502fe7ab4f251c266b558346cd3b8892c7c79b3b5f2c3a92f AS chart-packager
WORKDIR /workspace
RUN curl -fsSL https://get.helm.sh/helm-v3.14.0-linux-amd64.tar.gz | tar -xz && \
    mv linux-amd64/helm /usr/local/bin/helm && \
    rm -rf linux-amd64
COPY charts/CHART_REPO charts/CHART_REF /tmp/chart-source/
RUN CHART_REPO=$(cat /tmp/chart-source/CHART_REPO) && \
    CHART_REF=$(cat /tmp/chart-source/CHART_REF) && \
    git clone --depth 1 --branch "${CHART_REF}" "${CHART_REPO}" /tmp/openshell && \
    mkdir -p /workspace/charts && \
    helm package /tmp/openshell/deploy/helm/openshell --destination /workspace/charts/ && \
    mv /workspace/charts/*.tgz /workspace/charts/openshell.tgz && \
    rm -rf /tmp/openshell

# HI pins are the per-arch manifests from hi/go:1.26.7 and hi/static:1787099997.
# BUILDARCH selects a native Go toolchain; TARGETARCH selects the runtime and GOARCH.
# Bump amd64 and arm64 together when changing tags.
ARG BUILDARCH
ARG TARGETARCH

FROM registry.access.redhat.com/hi/go:1.26.7@sha256:2a2215ce496fdd33fdc19739eaadde902f7719c8be222f22f92fd7da02c889cb AS go-amd64
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:ba850dda34c47052bb8db5561c80c43fb08d0f9a729c90fddd755a2d5b46f74b AS go-arm64
FROM go-${BUILDARCH} AS builder

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

ARG TARGETARCH
RUN cd components/control-plane && CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -mod=mod -ldflags="-s -w" -o /workspace/hypershell-controller ./cmd/hypershell-controller

FROM registry.access.redhat.com/hi/static:1787099997@sha256:3d43712a61ce01c7049e983e7f4c4609aaad7582c7bbd930b57372f82c446f98 AS static-amd64
FROM registry.access.redhat.com/hi/static:1787099997@sha256:2ccd532da5e8868a92f479a7268a2fce65279c1f7c8a8dd0880d399c04d6a356 AS static-arm64
FROM static-${TARGETARCH}

COPY --from=builder --chown=1001:0 /workspace/hypershell-controller /usr/local/bin/
COPY --from=chart-packager --chown=1001:0 /workspace/charts/ /charts/
COPY --from=chart-packager --chown=1001:0 /usr/local/bin/helm /usr/local/bin/helm

ENTRYPOINT ["/usr/local/bin/hypershell-controller"]

LABEL org.opencontainers.image.title="HyperShell Controller" \
      org.opencontainers.image.description="Control plane that reconciles HyperShell resources via gRPC watch streams" \
      org.opencontainers.image.version="0.0.1"
