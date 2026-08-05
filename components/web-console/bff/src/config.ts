import path from "node:path";

import { z } from "zod";

const configSchema = z.object({
  HOST: z.string().trim().min(1).default("0.0.0.0"),
  LOG_LEVEL: z
    .enum(["fatal", "error", "warn", "info", "debug", "trace", "silent"])
    .default("info"),
  NODE_ENV: z.enum(["development", "test", "production"]).default("production"),
  PORT: z.coerce.number().int().min(1).max(65_535).default(8080),
  STATIC_ROOT: z
    .string()
    .trim()
    .min(1)
    .default(path.resolve(process.cwd(), "../build/client")),
});

export interface ServerConfig {
  host: string;
  logLevel: z.infer<typeof configSchema>["LOG_LEVEL"];
  nodeEnv: z.infer<typeof configSchema>["NODE_ENV"];
  port: number;
  staticRoot: string;
}

export function loadConfig(
  environment: NodeJS.ProcessEnv = process.env,
): ServerConfig {
  const result = configSchema.safeParse(environment);
  if (!result.success) {
    const problems = result.error.issues.map(
      (issue) => `${issue.path.join(".") || "configuration"}: ${issue.message}`,
    );
    throw new Error(
      `Invalid web-console BFF configuration: ${problems.join("; ")}`,
    );
  }

  return {
    host: result.data.HOST,
    logLevel: result.data.LOG_LEVEL,
    nodeEnv: result.data.NODE_ENV,
    port: result.data.PORT,
    staticRoot: path.resolve(result.data.STATIC_ROOT),
  };
}
