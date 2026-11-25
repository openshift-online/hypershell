# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/nodejs:24.18.1-builder, hi/nodejs:24.18.1
# and hi/go:1.26.7 (the hsctl browser module).
# BUILDARCH selects a native Node toolchain so vite/esbuild are not qemu-emulated.
# TARGETARCH selects production native addons (sodium-native) and the runtime.
# Bump amd64 and arm64 together when changing tags.
ARG BUILDARCH
ARG TARGETARCH

# Stamp the source commit and build time into the image so the BFF can report
# the console build version and build time in the browser runtime config.
# Konflux pipelines pass the cloned commit as GIT_VERSION; BUILD_TIME defaults to
# the image build time computed in the runtime stage when no value is supplied, so
# a production image never reports build_time "unknown".
ARG GIT_VERSION=unknown
ARG BUILD_TIME=unknown

# ---------------------------------------------------------------------------
# hsctl browser module for the terminal (specs/platform/hsctl-browser-build.spec.md,
# CLI-WASM-05): hsctl.wasm built from components/cli at the same source commit,
# and wasm_exec.js from the same Go toolchain. GOOS=js GOARCH=wasm output does
# not depend on the build architecture, so the native BUILDARCH toolchain builds
# it. HI pins are the per-arch manifests from hi/go:1.26.7; bump them with
# components/cli/Dockerfile and components/fleet-dashboard/Dockerfile.
# ---------------------------------------------------------------------------
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:2a2215ce496fdd33fdc19739eaadde902f7719c8be222f22f92fd7da02c889cb AS go-amd64
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:ba850dda34c47052bb8db5561c80c43fb08d0f9a729c90fddd755a2d5b46f74b AS go-arm64
FROM go-${BUILDARCH} AS hsctl-wasm

WORKDIR /workspace

# Module files first for layer caching; in a hermetic build the modules come
# from the Hermeto-prefetched gomod set for components/cli.
COPY components/cli/go.mod components/cli/go.sum components/cli/
RUN cd components/cli && go mod download

COPY components/cli/cmd/ components/cli/cmd/
COPY components/cli/pkg/ components/cli/pkg/
COPY components/cli/data/ components/cli/data/
COPY scripts/build-hsctl-wasm.sh scripts/build-hsctl-wasm.sh
RUN GOFLAGS=-mod=mod sh scripts/build-hsctl-wasm.sh /workspace/hsctl-runtime

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

# Hermetic-build registry alignment. In a Hermeto build, Hermeto injects
# `registry=file://.../deps/pnpm/npm/` into .npmrc, but pnpm-workspace.yaml's
# `registries.default` (public registry) and `trustLockfile: false` take
# precedence over .npmrc and would make pnpm fetch from the network (which is off)
# and reject the Hermeto-rewritten lockfile. Point the default registry at the
# prefetched cache and trust the lockfile FOR THE HERMETIC BUILD ONLY. Local
# (non-Hermeto) builds have no injected file:// registry, so this is a no-op and
# the committed registry default + supply-chain settings are preserved.
RUN if grep -qE '^registry=file:' .npmrc 2>/dev/null; then \
      reg="$(grep -E '^registry=file:' .npmrc | head -1 | cut -d= -f2-)"; \
      sed -i -E "s#^(  default: ).*#\\1${reg}#" pnpm-workspace.yaml; \
      sed -i -E "s#^(trustLockfile: ).*#\\1true#" pnpm-workspace.yaml; \
    fi

RUN pnpm install --frozen-lockfile

COPY --chown=${CONTAINER_DEFAULT_USER} components/sdk-typescript components/sdk-typescript
COPY --chown=${CONTAINER_DEFAULT_USER} packages/gateway-management-ui packages/gateway-management-ui
COPY --chown=${CONTAINER_DEFAULT_USER} packages/operational-dashboard-ui packages/operational-dashboard-ui
COPY --chown=${CONTAINER_DEFAULT_USER} components/web-console components/web-console
COPY --chown=${CONTAINER_DEFAULT_USER} images/brand images/brand
# The Vite build emits the hsctl runtime files from HSCTL_RUNTIME_DIR as
# content-hashed assets (CLI-WASM-05).
COPY --from=hsctl-wasm --chown=${CONTAINER_DEFAULT_USER} /workspace/hsctl-runtime/ components/web-console/.hsctl-runtime/

RUN HSCTL_RUNTIME_DIR=/app/components/web-console/.hsctl-runtime pnpm run build:web
# Precompress the large hsctl runtime once; the BFF serves the .br/.gz siblings.
RUN node components/web-console/scripts/precompress-assets.mjs components/web-console/build/client/assets

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

# Lead-time build metadata (OBS-SLI-09): the Konflux trusted-artifacts git-clone
# task exposes no commit-timestamp result, so the build time is computed here when
# no BUILD_TIME build arg is supplied and stamped into the deployed bundle below,
# so a production image never reports build_time "unknown".
ARG BUILD_TIME

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
  && cp -R components/web-console/build/client /tmp/web-console/public \
  && build_time="${BUILD_TIME}" \
  && { [ -n "${build_time}" ] && [ "${build_time}" != "unknown" ] \
       || build_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; } \
  && printf '%s' "${build_time}" > /tmp/web-console/.build-time

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
  HYPERSHELL_WEB_BUILD_TIME_FILE=/app/.build-time \
  NODE_ENV=production \
  PORT=8080 \
  STATIC_ROOT=/app/public

WORKDIR /app
# The build-time stamp (/app/.build-time) arrives from the bundle stage, where it
# is written as the unprivileged build user, so no write to /app is needed here
# (the runtime base runs as a non-root user). The BFF reads it at startup through
# HYPERSHELL_WEB_BUILD_TIME_FILE (OBS-SLI-09).
COPY --from=bundle /tmp/web-console/ ./

EXPOSE 8080

CMD ["node", "dist/bff/src/index.js"]
