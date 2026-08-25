# syntax=docker/dockerfile:1
FROM registry.access.redhat.com/hi/go:1.26.5@sha256:eb29e565caf76cf408a77a73548a3ed62e00e3516c17d02d5f36838aea69a605 AS builder

WORKDIR /workspace

# Install Helm via Go toolchain (avoids OS package dependencies).
# CGO_ENABLED=0 produces a static binary that runs on the distroless runtime.
RUN CGO_ENABLED=0 GOBIN=/usr/local/bin go install helm.sh/helm/v3/cmd/helm@v3.14.0

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

# Package Helm chart from fork (git is provided by the Go toolchain image)
COPY charts/CHART_REPO charts/CHART_REF /tmp/chart-source/
RUN CHART_REPO=$(cat /tmp/chart-source/CHART_REPO) && \
    CHART_REF=$(cat /tmp/chart-source/CHART_REF) && \
    git clone --depth 1 --branch "${CHART_REF}" "${CHART_REPO}" /tmp/openshell && \
    mkdir -p /workspace/charts && \
    helm package /tmp/openshell/deploy/helm/openshell --destination /workspace/charts/ && \
    mv /workspace/charts/*.tgz /workspace/charts/openshell.tgz && \
    rm -rf /tmp/openshell

FROM registry.access.redhat.com/hi/static:1787099997@sha256:f4d5109b57cf7eab0a7adc566f2d78f80fa0c5ec9ccab698c9fb8eb448db6071

COPY --from=builder --chown=1001:0 /workspace/hypershell-controller /usr/local/bin/
COPY --from=builder --chown=1001:0 /workspace/charts/ /charts/
COPY --from=builder --chown=1001:0 /usr/local/bin/helm /usr/local/bin/helm

ENTRYPOINT ["/usr/local/bin/hypershell-controller"]

LABEL org.opencontainers.image.title="HyperShell Controller" \
      org.opencontainers.image.description="Control plane that reconciles HyperShell resources via gRPC watch streams" \
      org.opencontainers.image.version="0.0.1"
