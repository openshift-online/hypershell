# HyperShell web console

The web console is a React 19 single-page application built with React Router Framework Mode, Vite, PatternFly 6, TanStack Query, React Intl, and the generated HyperShell SDK. A separate Fastify package serves the production assets and browser-facing security controls.

## Prerequisites

- Node.js 24.18.1 or newer in the 24.x line (see the repository `.node-version`)
- pnpm 11.15.1

Install the pinned pnpm artifact without relying on Node's bundled Corepack:

```sh
bash scripts/bootstrap_pnpm.sh
pnpm install --frozen-lockfile
```

The bootstrap script downloads the exact npm tarball over TLS and verifies its committed SHA-256 digest before installation.

## Hot-reload development

Start the API in its isolated no-auth development mode in one terminal:

```sh
cd components/api-server
make run-no-auth
```

From the repository root, start Vite with React Router hot-module replacement:

```sh
pnpm dev
```

Open <http://127.0.0.1:5173>. Browser requests retain the production-shaped relative `/api` path and Vite proxies them to `http://127.0.0.1:8000`. Set `WEB_CONSOLE_API_ORIGIN` to another loopback HTTP(S) origin when needed; credentials and non-loopback targets are rejected. No browser-visible flag enables authentication bypass.

## Verification

```sh
pnpm check:web
pnpm --filter @openshift-online/hypershell-web-console exec playwright install chromium
pnpm test:e2e:chromium
```

The complete browser matrix is available through `pnpm test:e2e` after installing Chromium, Firefox, and WebKit.

## Production container

Build from the repository root so the shared lockfile and SDK are available:

```sh
podman build --file components/web-console/Dockerfile --tag localhost/hypershell-web-console:dev .
podman run --rm \
  --read-only \
  --user 65532:65532 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --publish 8080:8080 \
  localhost/hypershell-web-console:dev
```

The image is multi-stage and pins Red Hat's `hi/nodejs` builder and runtime variants by digest. Its runtime contains the production BFF dependency closure and built assets, runs as a numeric non-root user, writes no files, exposes port 8080, and provides `/health/live` and `/health/ready` probes. The initial public Hello world scaffold intentionally exposes no API proxy or authentication bypass; OIDC session and authenticated API proxy work belongs to the next architecture delivery increment.
