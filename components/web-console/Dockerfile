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

FROM --platform=linux/amd64 registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:76b9f45b0058783accf4aa383c962d51f7624acb4fb2d8368da1023e9c5ae0ea AS build-amd64
FROM --platform=linux/arm64 registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:1e1eba50cafc6a4ad8fa18ec1d308e93b82af0fe4b519719b5290e30736169e7 AS build-arm64
FROM --platform=linux/${BUILDARCH} build-${BUILDARCH} AS build

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

RUN pnpm run build:web

FROM --platform=linux/${TARGETARCH} build-${TARGETARCH} AS bundle

USER root

COPY scripts/bootstrap_pnpm.sh .
RUN ./bootstrap_pnpm.sh

USER ${CONTAINER_DEFAULT_USER}

WORKDIR /app

COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/package.json /app/package.json
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/pnpm-lock.yaml /app/pnpm-lock.yaml
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/pnpm-workspace.yaml /app/pnpm-workspace.yaml
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/components /app/components
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/packages /app/packages
COPY --from=build --chown=${CONTAINER_DEFAULT_USER} /app/images /app/images

RUN pnpm install --frozen-lockfile --prod --filter @openshift-online/hypershell-web-console-bff \
  && pnpm --filter @openshift-online/hypershell-web-console-bff deploy --prod /tmp/web-console \
  && cp -R components/web-console/build/client /tmp/web-console/public

FROM --platform=linux/amd64 registry.access.redhat.com/hi/nodejs:24.18.1@sha256:ed083f49a5836777494a848735b46e722b4a81731eb3bfd2d74b819a2e7fe097 AS runtime-amd64
FROM --platform=linux/arm64 registry.access.redhat.com/hi/nodejs:24.18.1@sha256:022f595d925e3fe3aa3e896f4d9a55c1cc696758349748c8c51eebd35a20b47a AS runtime-arm64
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
