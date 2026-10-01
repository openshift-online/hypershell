// Anti-corruption layer: the Go BFF serialises each plane in the shape defined by
// data-architecture.spec §5 ("/api/promotion ... preserves the promotion.json
// shape"). That wire contract is authoritative; the UI's domain models are a
// deliberately smaller, firewall-clean projection of it (see domain/promotion.ts).
// These mappers are the ONE place the wire shape is known - everything downstream
// consumes the domain types. Without this mapping the adapter was handing raw wire
// objects to components that expect the domain shape (e.g. `env.gates`, which the
// wire spells `activeGates`), throwing at render time.

import type { InstancesData } from "../../application/ports";
import type {
  PromotionData,
  PromotionEnvironment,
  PromotionGate,
} from "../../domain/promotion";

/** A release as the BFF reports it (opaque to the UI beyond its version string). */
interface WireRelease {
  readonly version?: string;
  readonly tag?: string;
  readonly date?: string;
  readonly sha?: string;
}

/** A promotion gate on the wire: `{key, phase}` (the domain renames key -> name). */
interface WireGate {
  readonly key?: string;
  readonly phase?: string;
}

interface WirePromotionEnvironment {
  readonly active?: WireRelease | null;
  readonly proposed?: WireRelease | null;
  readonly activeGates?: readonly WireGate[] | null;
  readonly proposedGates?: readonly WireGate[] | null;
}

interface WirePromotion {
  readonly order?: readonly string[] | null;
  readonly environments?: Readonly<
    Record<string, WirePromotionEnvironment>
  > | null;
  readonly releases?: Readonly<Record<string, unknown>> | null;
}

/** `/api/instances` returns a bare list of instance names. */
interface WireInstances {
  readonly instances?: readonly string[] | null;
}

function mapGate(gate: WireGate): PromotionGate {
  return {
    name: gate.key ?? "",
    phase: gate.phase ?? null,
    // The wire does not attribute a gate to a governing instance; the domain
    // keeps the field so the server can supply it later without a shape change.
    governingInstance: null,
  };
}

function mapEnvironment(
  name: string,
  env: WirePromotionEnvironment,
): PromotionEnvironment {
  return {
    name,
    activeRelease: env.active?.version ?? null,
    proposedRelease: env.proposed?.version ?? null,
    // The "Gates" column reflects the gates governing what is currently active.
    gates: (env.activeGates ?? []).map(mapGate),
  };
}

export function mapPromotion(raw: unknown): PromotionData {
  const wire = (raw ?? {}) as WirePromotion;
  const environments: Record<string, PromotionEnvironment> = {};
  for (const [key, env] of Object.entries(wire.environments ?? {})) {
    environments[key] = mapEnvironment(key, env);
  }
  return {
    order: wire.order ?? [],
    environments,
    // Wire `releases` is a version-keyed map; the domain only needs the keys.
    releases: Object.keys(wire.releases ?? {}),
  };
}

export function mapInstances(raw: unknown): InstancesData {
  const wire = (raw ?? {}) as WireInstances;
  return {
    instances: (wire.instances ?? []).map((name) => ({
      name,
      // Role/provider/region/health are not carried on /api/instances today;
      // they render as "none" until the server includes them.
      role: null,
      provider: null,
      region: null,
      health: null,
    })),
  };
}
