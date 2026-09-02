# syntax=docker/dockerfile:1
ARG BUILD_PREFIX=dev
ARG BUILD_SUFFIX=
ARG VCS_REF=unknown

FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:e2a04f7443db3d27751ed6e9d74ee5c3a02217dbf6c8769e468ef3b1b2a3e6fa AS build

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

FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:07b0f6cf5dabef30b1efa030eea020095b1ea227c088dbcbe0ebd35bb24b7ad9

ARG BUILD_PREFIX
ARG BUILD_SUFFIX
ARG VCS_REF

# Remove the final 33 characters from the 40-character VCS_REF.
# This keeps the first seven characters.
LABEL org.opencontainers.image.title="HyperShell web console" \
      org.opencontainers.image.description="HyperShell web console BFF and static application" \
      org.opencontainers.image.version="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}${BUILD_SUFFIX}" \
      org.opencontainers.image.revision="${VCS_REF}"

ENV HOST=0.0.0.0 \
  HYPERSHELL_BUILD_REVISION="${VCS_REF}" \
  HYPERSHELL_BUILD_VERSION="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}${BUILD_SUFFIX}" \
  HYPERSHELL_API_ORIGIN=http://127.0.0.1:8000 \
  HYPERSHELL_API_TIMEOUT_MS=30000 \
  NODE_ENV=production \
  PORT=8080 \
  STATIC_ROOT=/app/public

WORKDIR /app
COPY --from=build /tmp/web-console/ ./

EXPOSE 8080

CMD ["node", "dist/index.js"]
