# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/nodejs:24.18.1-builder and hi/nodejs:24.18.1.
# TARGETARCH selects both stages so native addons and the node binary match the cluster.
# Bump amd64 and arm64 together when changing tags.
ARG TARGETARCH

FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:7d3b87dbb6bbf8fc85c865f8a00c5355961dae0e0e4ead15d530af3c8d6e3ebe AS build-amd64
FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:5034714f7a4bdd7423231f880b0863b8e69c5aa9b8f5ab69ed1cd1e76760f0e0 AS build-arm64
FROM build-${TARGETARCH} AS build

USER root

COPY scripts/bootstrap_pnpm.sh .
RUN ./bootstrap_pnpm.sh

USER ${CONTAINER_DEFAULT_USER}

WORKDIR /app

COPY --chown=${CONTAINER_DEFAULT_USER} package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY --chown=${CONTAINER_DEFAULT_USER} components/sdk-typescript/package.json components/sdk-typescript/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} packages/gateway-management-ui/package.json packages/gateway-management-ui/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console/package.json components/web-console/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console/bff/package.json components/web-console/bff/package.json
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console/domain-probes/package.json components/web-console/domain-probes/package.json

RUN pnpm install --frozen-lockfile

COPY --chown=${CONTAINER_DEFAULT_USER} components/sdk-typescript components/sdk-typescript
COPY --chown=${CONTAINER_DEFAULT_USER} packages/gateway-management-ui packages/gateway-management-ui
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console components/web-console
COPY --chown=${CONTAINER_DEFAULT_USER} images/brand images/brand

RUN pnpm run build:web \
  && pnpm --filter @openshift-online/hypershell-web-console-bff deploy --prod /tmp/web-console \
  && cp -R components/web-console/build/client /tmp/web-console/public

FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:6725f3730de000b6255b97d31e2ef53916f35397b1fb779bdb8d4a3a1c84ad50 AS runtime-amd64
FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:8b3a067f10336acb6798f4d8251882fed107c90116b69a469e02472ecc1c7648 AS runtime-arm64
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

CMD ["node", "dist/index.js"]
