FROM registry.access.redhat.com/hi/nodejs:24.18.1-builder@sha256:63f3ae9ba3ac4af3c809d58c0845effbfbb75a6f2e08148af8ae2e95d889132f AS build

USER 0

ADD --checksum=sha256:27460629b10111604e7f98882753b53398986820c20e0a065f3a4a5e9e7db71f \
    https://registry.npmjs.org/pnpm/-/pnpm-11.15.1.tgz /tmp/pnpm.tgz
RUN npm install --global /tmp/pnpm.tgz --ignore-scripts \
    && test "$(pnpm --version)" = "11.15.1" \
    && rm /tmp/pnpm.tgz

USER 65532:65532
WORKDIR /app

COPY --chown=65532:65532 package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY --chown=65532:65532 components/sdk-typescript/package.json components/sdk-typescript/package.json
COPY --chown=65532:65532 components/web-console/package.json components/web-console/package.json
COPY --chown=65532:65532 components/web-console/bff/package.json components/web-console/bff/package.json
COPY --chown=65532:65532 components/web-console/domain-probes/package.json components/web-console/domain-probes/package.json

RUN pnpm fetch --frozen-lockfile

COPY --chown=65532:65532 components/sdk-typescript components/sdk-typescript
COPY --chown=65532:65532 components/web-console components/web-console
COPY --chown=65532:65532 images/brand/logo.png images/brand/logo.png

RUN pnpm install --frozen-lockfile --offline \
    && pnpm run build:web \
    && pnpm --filter @openshift-online/hypershell-web-console-bff deploy --prod /tmp/web-console \
    && cp -R components/web-console/build/client /tmp/web-console/public

FROM registry.access.redhat.com/hi/nodejs:24.18.1@sha256:b24c3fce1b2803aa622d1bd7add354d0974528fdf1f16f1303226bfa5e3c6666

LABEL org.opencontainers.image.title="HyperShell web console" \
      org.opencontainers.image.description="HyperShell web console BFF and static application" \
      org.opencontainers.image.source="https://github.com/openshift-online/hypershell"

ENV HOST=0.0.0.0 \
    NODE_ENV=production \
    PORT=8080 \
    STATIC_ROOT=/app/public

WORKDIR /app
COPY --from=build --chown=65532:65532 /tmp/web-console/ ./

USER 65532:65532
EXPOSE 8080
CMD ["dist/index.js"]
