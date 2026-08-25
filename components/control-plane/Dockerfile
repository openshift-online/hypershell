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

# Go build stage
FROM registry.access.redhat.com/hi/go:1.26.5@sha256:eb29e565caf76cf408a77a73548a3ed62e00e3516c17d02d5f36838aea69a605 AS builder

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

COPY --from=builder --chown=1001:0 /workspace/hypershell-controller /usr/local/bin/
COPY --from=chart-packager --chown=1001:0 /workspace/charts/ /charts/
COPY --from=chart-packager --chown=1001:0 /usr/local/bin/helm /usr/local/bin/helm

ENTRYPOINT ["/usr/local/bin/hypershell-controller"]

LABEL org.opencontainers.image.title="HyperShell Controller" \
      org.opencontainers.image.description="Control plane that reconciles HyperShell resources via gRPC watch streams" \
      org.opencontainers.image.version="0.0.1"
