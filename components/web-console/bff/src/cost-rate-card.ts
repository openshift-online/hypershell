import { readFileSync, watch, type FSWatcher } from "node:fs";

import { z } from "zod";

/**
 * Rate card for the cost dashboard (WEB-COST-03). It prices one unit of
 * reserved capacity per hour. It is deployment configuration: the BFF reads it
 * from COST_RATE_CARD_FILE and never sends rates to the browser.
 *
 * The file is JSON (JSON is a subset of YAML, so a ConfigMap can hold it as-is).
 */

const rate = z.number().min(0);

const partialRates = z
  .object({
    cpu_core_hour: rate.optional(),
    memory_gib_hour: rate.optional(),
  })
  .strict();

const labelKey = z
  .string()
  .min(1)
  .max(253)
  .regex(/^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$/u);

const rateCardSchema = z
  .object({
    currency: z
      .string()
      .regex(/^[A-Z]{3}$/u, "must be an ISO 4217 currency code"),
    version: z.string().trim().min(1).max(64),
    defaults: partialRates.optional(),
    providers: z.record(labelKey, partialRates).optional(),
    clusters: z.record(labelKey, partialRates).optional(),
    fixed_monthly: z.record(labelKey, rate).optional(),
  })
  .strict();

export type RateCardDocument = z.infer<typeof rateCardSchema>;

export interface ResolvedRates {
  cpuCoreHour: number;
  memoryGibHour: number;
}

export interface RateCard {
  currency: string;
  /** Flat monthly cost of a cluster (control plane fee, etc.); 0 when absent. */
  fixedMonthly(cluster: string): number;
  /**
   * Rates for a cluster, resolving each resource independently in the order
   * clusters.<cluster> -> providers.<provider> -> defaults. Undefined when the
   * cluster has no CPU or no memory rate at any level ("unpriced").
   */
  resolve(
    cluster: string,
    provider: string | undefined,
  ): ResolvedRates | undefined;
  version: string;
}

export function parseRateCard(text: string): RateCard {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new Error("Cost rate card is not valid JSON");
  }
  const result = rateCardSchema.safeParse(raw);
  if (!result.success) {
    const issue = result.error.issues[0];
    const where = issue?.path.join(".") ?? "";
    throw new Error(
      `Cost rate card is invalid${where ? ` at ${where}` : ""}: ${issue?.message ?? "unknown error"}`,
    );
  }
  return rateCardFromDocument(result.data);
}

export function rateCardFromDocument(document: RateCardDocument): RateCard {
  const pick = (
    key: "cpu_core_hour" | "memory_gib_hour",
    cluster: string,
    provider: string | undefined,
  ): number | undefined =>
    document.clusters?.[cluster]?.[key] ??
    (provider === undefined
      ? undefined
      : document.providers?.[provider]?.[key]) ??
    document.defaults?.[key];

  return {
    currency: document.currency,
    version: document.version,
    fixedMonthly: (cluster) => document.fixed_monthly?.[cluster] ?? 0,
    resolve: (cluster, provider) => {
      const cpuCoreHour = pick("cpu_core_hour", cluster, provider);
      const memoryGibHour = pick("memory_gib_hour", cluster, provider);
      if (cpuCoreHour === undefined || memoryGibHour === undefined) {
        return undefined;
      }
      return { cpuCoreHour, memoryGibHour };
    },
  };
}

export type RateCardState =
  | { status: "not-configured" }
  | { status: "invalid"; error: string }
  | { status: "ready"; rateCard: RateCard };

export interface RateCardSource {
  close(): void;
  current(): RateCardState;
}

function readRateCard(file: string): RateCardState {
  let text: string;
  try {
    text = readFileSync(file, "utf8");
  } catch (error) {
    const missing =
      error instanceof Error && "code" in error && error.code === "ENOENT";
    // Fixed message: never log the path or file contents.
    return {
      status: "invalid",
      error: `Cost rate card file is ${missing ? "missing" : "unreadable"}`,
    };
  }
  try {
    return { status: "ready", rateCard: parseRateCard(text) };
  } catch (error) {
    return {
      status: "invalid",
      error:
        error instanceof Error ? error.message : "Cost rate card is invalid",
    };
  }
}

/**
 * Load the rate card at startup and reload it when the file changes
 * (WEB-COST-03). Kubernetes updates a mounted ConfigMap by swapping a symlink
 * in the parent directory, so the watcher watches the directory, not the file.
 * A reload that fails keeps the state "invalid" so readiness fails, rather
 * than silently serving the previous rates.
 */
export function createRateCardSource(
  file: string | undefined,
  onChange?: (state: RateCardState) => void,
): RateCardSource {
  if (file === undefined) {
    return {
      close: () => undefined,
      current: () => ({ status: "not-configured" }),
    };
  }

  let state = readRateCard(file);
  let watcher: FSWatcher | undefined;
  let timer: NodeJS.Timeout | undefined;
  const reload = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = setTimeout(() => {
      state = readRateCard(file);
      onChange?.(state);
    }, 250);
    timer.unref();
  };
  try {
    const directory = file.replace(/\/[^/]*$/u, "") || "/";
    watcher = watch(directory, { persistent: false }, reload);
    watcher.on("error", () => undefined);
  } catch {
    watcher = undefined;
  }

  return {
    close: () => {
      if (timer !== undefined) clearTimeout(timer);
      watcher?.close();
    },
    current: () => state,
  };
}
