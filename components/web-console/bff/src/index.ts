import { createBffTracing } from "./adapters/observability/otel-tracing.js";
import { buildApp } from "./app.js";
import { loadConfig } from "./config.js";

const config = loadConfig();
// The bootstrap is the one server path exempt from the telemetry import ban; it
// owns the OTel SDK lifecycle and injects the tracing port into the app.
const tracing = createBffTracing(config.tracing);
const app = await buildApp(config, tracing);

const shutdown = async (signal: NodeJS.Signals): Promise<void> => {
  app.log.info({ signal }, "shutting down");
  try {
    await app.close();
    await tracing.shutdown();
    process.exitCode = 0;
  } catch (error) {
    app.log.error({ err: error }, "graceful shutdown failed");
    process.exitCode = 1;
  }
};

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.once(signal, () => {
    void shutdown(signal);
  });
}

try {
  await app.listen({ host: config.host, port: config.port });
} catch (error) {
  app.log.fatal({ err: error }, "web-console BFF failed to start");
  process.exitCode = 1;
}
