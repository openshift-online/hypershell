# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/nodejs:24.18.1-builder and hi/nodejs:24.18.1.
# TARGETARCH selects both stages so native addons and the node binary match the cluster.
# Bump amd64 and arm64 together when changing tags.
ARG TARGETARCH

FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:d745ec2a0bbd683bb74cde03d43d473f2e5616deebf01e30241ef89f8a036136 AS build-amd64
FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:184209155ace74fcd587b15dfd7e8604f56241444252a0fb0363186e9945f271 AS build-arm64
FROM build-${TARGETARCH} AS build

USER root

COPY scripts/bootstrap_pnpm.sh .
RUN ./bootstrap_pnpm.sh

USER ${CONTAINER_DEFAULT_USER}

WORKDIR /app

COPY --chown=${CONTAINER_DEFAULT_USER} package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY --chown=${CONTAINER_DEFAULT_USER} components/sdk-typescript/package.json components/sdk-typescript/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} packages/gateway-management-ui/package.json packages/gateway-management-ui/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} packages/operational-dashboard-ui/package.json packages/operational-dashboard-ui/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console/package.json components/web-console/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console/bff/package.json components/web-console/bff/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console/domain-probes/package.json components/web-console/domain-probes/package.json

RUN pnpm install --frozen-lockfile

COPY --chown=${CONTAINER_DEFAULT_USER} components/sdk-typescript components/sdk-typescript
COPY --chown=${CONTAINER_DEFAULT_USER} packages/gateway-management-ui packages/gateway-management-ui
COPY --chown=${CONTAINER_DEFAULT_USER} packages/operational-dashboard-ui packages/operational-dashboard-ui
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console components/web-console
COPY --chown=${CONTAINER_DEFAULT_USER} images/brand images/brand

RUN pnpm run build:web \
  && pnpm --filter @openshift-online/hypershell-web-console-bff deploy --prod /tmp/web-console \
  && cp -R components/web-console/build/client /tmp/web-console/public

FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:a0db612946134e94e808c303eaf31b335bd632a46ea5eea74f33b03a05c5f838 AS runtime-amd64
FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:fe0ecc3423492daf5b7ba8c441bfe04baa94773c9d28eb4768fbc337d4997d52 AS runtime-arm64
FROM runtime-${TARGETARCH}

LABEL org.opencontainers.image.title="HyperShell web console" \
      org.opencontainers.image.description="HyperShell web console BFF and static application" \
      org.opencontainers.image.version="0.0.1"

ENV HOST=0.0.0.0 \
  HYPERSHELL_API_ORIGIN=http://127.0.0.1:8000 \
  HYPERSHELL_API_TIMEOUT_MS=30000 \
  NODE_ENV=production \
  PORT=8080 \
  STATIC_ROOT=/app/public

WORKDIR /app
COPY --from=build /tmp/web-console/ ./

EXPOSE 8080

CMD ["node", "dist/bff/src/index.js"]
