import { createApiVersionAdapter } from "../adapters/api/api-version";

/** Browser-wide reader for the API image version. */
export const apiVersionReader = createApiVersionAdapter();
