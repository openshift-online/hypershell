# syntax=docker/dockerfile:1
# HI pins: hi/nodejs builder (UI bundle), hi/go builder (BFF), hi/static runtime.
# BUILDARCH selects native toolchains (avoid qemu for vite/esbuild and the Go
# compiler); TARGETARCH selects the runtime and GOARCH. Bump amd64 and arm64
# tags together.
ARG BUILDARCH
ARG TARGETARCH

# Stamp the source commit into the binary (reported at /healthz and in the UI
# runtime config). Konflux passes the cloned commit; local builds default it.
ARG GIT_VERSION=unknown
ARG BUILD_TIME=unknown

# ---------------------------------------------------------------------------
# Stage 1 - build the React UI bundle (packages/fleet-dashboard-ui -> dist/)
# ---------------------------------------------------------------------------
FROM --platform=linux/amd64 registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:7d3b87dbb6bbf8fc85c865f8a00c5355961dae0e0e4ead15d530af3c8d6e3ebe AS ui-build-amd64
FROM --platform=linux/arm64 registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:5034714f7a4bdd7423231f880b0863b8e69c5aa9b8f5ab69ed1cd1e76760f0e0 AS ui-build-arm64
FROM --platform=linux/${BUILDARCH} ui-build-${BUILDARCH} AS ui-build

USER root
COPY scripts/bootstrap_pnpm.sh .
RUN ./bootstrap_pnpm.sh
USER ${CONTAINER_DEFAULT_USER}

WORKDIR /app

# Workspace manifests first for layer caching, then install, then sources.
COPY --chown=${CONTAINER_DEFAULT_USER} package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY --chown=${CONTAINER_DEFAULT_USER} packages/fleet-dashboard-ui/package.json packages/fleet-dashboard-ui/package.json
RUN pnpm install --frozen-lockfile --filter @openshift-online/hypershell-fleet-dashboard-ui...

COPY --chown=${CONTAINER_DEFAULT_USER} packages/fleet-dashboard-ui packages/fleet-dashboard-ui
RUN pnpm --filter @openshift-online/hypershell-fleet-dashboard-ui run build

# ---------------------------------------------------------------------------
# Stage 2 - build the static Go BFF with the UI bundle embedded
# ---------------------------------------------------------------------------
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:2a2215ce496fdd33fdc19739eaadde902f7719c8be222f22f92fd7da02c889cb AS go-amd64
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:ba850dda34c47052bb8db5561c80c43fb08d0f9a729c90fddd755a2d5b46f74b AS go-arm64
FROM go-${BUILDARCH} AS builder

WORKDIR /workspace

COPY components/fleet-dashboard/go.mod components/fleet-dashboard/go.sum ./
RUN go mod download

COPY components/fleet-dashboard/cmd/ cmd/
COPY components/fleet-dashboard/pkg/ pkg/
COPY components/fleet-dashboard/web/ web/

# Drop the built UI bundle into the embed root before compiling, so the binary
# is self-contained (single artifact, served with an SPA fallback).
COPY --from=ui-build /app/packages/fleet-dashboard-ui/dist/ web/dist/

ARG GIT_VERSION
ARG BUILD_TIME
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -mod=mod -ldflags="-s -w \
  -X github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/version.Version=${GIT_VERSION} \
  -X 'github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/version.BuildTime=${BUILD_TIME}'" \
  -o fleet-dashboard ./cmd/fleet-dashboard

# ---------------------------------------------------------------------------
# Stage 3 - distroless static runtime (no shell, non-root)
# ---------------------------------------------------------------------------
FROM registry.access.redhat.com/hi/static:1787099997@sha256:3d43712a61ce01c7049e983e7f4c4609aaad7582c7bbd930b57372f82c446f98 AS static-amd64
FROM registry.access.redhat.com/hi/static:1787099997@sha256:2ccd532da5e8868a92f479a7268a2fce65279c1f7c8a8dd0880d399c04d6a356 AS static-arm64
FROM static-${TARGETARCH}

COPY --from=builder /workspace/fleet-dashboard /usr/local/bin/

# 8080 = HTTP (fronted by oauth-proxy in the pod); 9090 = /metrics.
EXPOSE 8080
EXPOSE 9090

ENTRYPOINT ["/usr/local/bin/fleet-dashboard"]

LABEL org.opencontainers.image.title="HyperShell Fleet Dashboard" \
      org.opencontainers.image.description="Read-only fleet operational dashboard (BFF + embedded UI)" \
      org.opencontainers.image.version="0.0.1"
