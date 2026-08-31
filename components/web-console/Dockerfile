# syntax=docker/dockerfile:1
FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:09e16ab3b22a1a1a9f7efae92e03e5ad53c948bc4a18a0196893052d59446c66 AS build

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
COPY --chown=${CONTAINER_DEFAULT_USER} images/brand/logo.png images/brand/logo.png

RUN pnpm run build:web \
  && pnpm --filter @openshift-online/hypershell-web-console-bff deploy --prod /tmp/web-console \
  && cp -R components/web-console/build/client /tmp/web-console/public

FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:6b2089244198989d5cb89722abed9fad93cff1bd5165cbf4beaee5aecd276be2

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
