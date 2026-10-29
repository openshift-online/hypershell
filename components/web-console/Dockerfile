# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/nodejs:24.18.1-builder and hi/nodejs:24.18.1.
# BUILDARCH selects a native Node toolchain so vite/esbuild are not qemu-emulated.
# TARGETARCH selects production native addons (sodium-native) and the runtime.
# Bump amd64 and arm64 together when changing tags.
ARG BUILDARCH
ARG TARGETARCH

# Stamp the source commit into the image so the BFF can report the console
# build version in the browser runtime config. Defaults to unknown for local
# builds; Konflux pipelines pass the cloned commit.
ARG GIT_VERSION=unknown

FROM --platform=linux/amd64 registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:7d3b87dbb6bbf8fc85c865f8a00c5355961dae0e0e4ead15d530af3c8d6e3ebe AS build-amd64
FROM --platform=linux/arm64 registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:5034714f7a4bdd7423231f880b0863b8e69c5aa9b8f5ab69ed1cd1e76760f0e0 AS build-arm64
FROM --platform=linux/${BUILDARCH} build-${BUILDARCH} AS build

USER root

# Install the pinned pnpm CLI from the Hermeto-prefetched npm bootstrap
# (prefetch/pnpm). `npm ci --prefer-offline` resolves pnpm from the prefetched
# cache so the build is hermetic (no network); outside Hermeto it falls back to
# the registry. Replaces the networked scripts/bootstrap_pnpm.sh for the image
# build while CI keeps using that script.
COPY prefetch/pnpm/package.json prefetch/pnpm/package-lock.json /opt/pnpm-bootstrap/
RUN npm ci --prefix /opt/pnpm-bootstrap --prefer-offline --ignore-scripts --no-audit --no-fund --no-progress \
  && ln -sf /opt/pnpm-bootstrap/node_modules/.bin/pnpm /usr/local/bin/pnpm \
  && test "$(pnpm --version)" = "11.15.1"

USER ${CONTAINER_DEFAULT_USER}

WORKDIR /app

# .npmrc carries Hermeto's injected registry redirect to the prefetched pnpm
# cache during hermetic builds; a benign committed copy keeps local builds working.
COPY --chown=${CONTAINER_DEFAULT_USER} package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
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

RUN pnpm run build:web

# BFF_SOURCE_MAPS=true recompiles the BFF with embedded source maps so
# NODE_V8_COVERAGE data maps back to TypeScript in local Kind coverage runs
# (KIND_COVERAGE=true, specs/platform/local-development.spec.md DEV-LOC-18).
# Release builds leave it off.
ARG BFF_SOURCE_MAPS=false
RUN if [ "${BFF_SOURCE_MAPS}" = "true" ]; then \
    pnpm --filter @openshift-online/hypershell-web-console-bff exec \
      tsc --project tsconfig.json --sourceMap --inlineSources; \
  fi

FROM --platform=linux/${TARGETARCH} build-${TARGETARCH} AS bundle

USER root

# Install the pinned pnpm CLI from the Hermeto-prefetched npm bootstrap
# (prefetch/pnpm). `npm ci --prefer-offline` resolves pnpm from the prefetched
# cache so the build is hermetic (no network); outside Hermeto it falls back to
# the registry. Replaces the networked scripts/bootstrap_pnpm.sh for the image
# build while CI keeps using that script.
COPY prefetch/pnpm/package.json prefetch/pnpm/package-lock.json /opt/pnpm-bootstrap/
RUN npm ci --prefix /opt/pnpm-bootstrap --prefer-offline --ignore-scripts --no-audit --no-fund --no-progress \
  && ln -sf /opt/pnpm-bootstrap/node_modules/.bin/pnpm /usr/local/bin/pnpm \
  && test "$(pnpm --version)" = "11.15.1"

USER ${CONTAINER_DEFAULT_USER}

WORKDIR /app

COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/package.json /app/package.json
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/pnpm-lock.yaml /app/pnpm-lock.yaml
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/pnpm-workspace.yaml /app/pnpm-workspace.yaml
# Carry the Hermeto-injected .npmrc so the prod install stage also resolves
# dependencies from the prefetched cache under network isolation.
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/.npmrc /app/.npmrc
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/components /app/components
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/packages /app/packages
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/images /app/images

RUN pnpm install --frozen-lockfile --prod --filter @openshift-online/hypershell-web-console-bff \
  && pnpm --filter @openshift-online/hypershell-web-console-bff deploy --prod /tmp/web-console \
  && cp -R components/web-console/build/client /tmp/web-console/public

FROM --platform=linux/amd64 registry.access.redhat.com/hi/nodejs:24.18.1@sha256:6725f3730de000b6255b97d31e2ef53916f35397b1fb779bdb8d4a3a1c84ad50 AS runtime-amd64
FROM --platform=linux/arm64 registry.access.redhat.com/hi/nodejs:24.18.1@sha256:8b3a067f10336acb6798f4d8251882fed107c90116b69a469e02472ecc1c7648 AS runtime-arm64
FROM --platform=linux/${TARGETARCH} runtime-${TARGETARCH}

ARG GIT_VERSION

LABEL org.opencontainers.image.title="HyperShell web console" \
      org.opencontainers.image.description="HyperShell web console BFF and static application" \
      org.opencontainers.image.version="0.0.1"

ENV HOST=0.0.0.0 \
  HYPERSHELL_API_ORIGIN=http://127.0.0.1:8000 \
  HYPERSHELL_API_TIMEOUT_MS=30000 \
  HYPERSHELL_WEB_VERSION=${GIT_VERSION} \
  NODE_ENV=production \
  PORT=8080 \
  STATIC_ROOT=/app/public

WORKDIR /app
COPY --from=bundle /tmp/web-console/ ./

EXPOSE 8080

CMD ["node", "dist/bff/src/index.js"]
