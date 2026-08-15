import { createSessionAdapter } from "../adapters/session/session-adapter";

/** Host composition root for the browser session resource. */
export const sessionGateway = createSessionAdapter();
