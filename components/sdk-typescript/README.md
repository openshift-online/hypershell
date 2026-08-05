# HyperShell TypeScript SDK

The SDK source in `src/` is generated from the API server's OpenAPI specification. Do not edit generated files directly.

Generate it with the vendored `rh-trex-ai` SDK generator:

```sh
cd components/api-server
make generate-sdk
```

`make generate-sdk-ts` is an equivalent target matching the upstream generator's naming.

Install dependencies and validate or build the generated SDK with:

```sh
npm ci
npm run check
npm run build
```
