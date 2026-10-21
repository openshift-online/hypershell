import {
  GatewayOperationError,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";

export interface QuickCreateRandomSource {
  nextHex(digits: number): string;
  nextInt(maxExclusive: number): number;
}

export interface QuickCreateDependencies {
  generateName: () => string;
  knownNames: ReadonlySet<string>;
  provisionGateway: (name: string) => Promise<GatewayRecord>;
}

export const quickCreatePlacement = {
  excludedSelector: ["runtime/kata"],
  selector: ["network/public", "provider/ibm"],
};

export const quickCreateMaximumAttempts = 5;

const adjectives = [
  "amber",
  "brisk",
  "calm",
  "deep",
  "ember",
  "fleet",
  "golden",
  "keen",
  "lunar",
  "nimble",
  "polar",
  "quiet",
  "rapid",
  "silver",
  "tidal",
  "vivid",
] as const;

const nouns = [
  "beacon",
  "compass",
  "current",
  "engine",
  "falcon",
  "harbor",
  "lantern",
  "orbit",
  "pilot",
  "quarry",
  "signal",
  "summit",
  "tundra",
  "vector",
  "voyage",
  "zenith",
] as const;

const gatewayNamePattern = /^[a-z]([a-z0-9-]*[a-z0-9])?$/u;

export function isValidGatewayName(name: string): boolean {
  return name.length <= 63 && gatewayNamePattern.test(name);
}

export function createQuickCreateNameGenerator(
  random: QuickCreateRandomSource,
): () => string {
  return () => {
    const adjective = adjectives[random.nextInt(adjectives.length)] ?? "swift";
    const noun = nouns[random.nextInt(nouns.length)] ?? "gateway";
    return `${adjective}-${noun}-${random.nextHex(4)}`;
  };
}

export async function createQuickCreateGateway({
  generateName,
  knownNames,
  provisionGateway,
}: QuickCreateDependencies): Promise<GatewayRecord> {
  let lastConflict: unknown;
  for (let attempt = 0; attempt < quickCreateMaximumAttempts; attempt += 1) {
    const name = generateName();
    if (!isValidGatewayName(name)) {
      throw new Error("Generated gateway name is invalid");
    }
    if (knownNames.has(name)) {
      lastConflict = new Error("Generated gateway name already exists");
      continue;
    }
    try {
      return await provisionGateway(name);
    } catch (error) {
      if (
        !(error instanceof GatewayOperationError) ||
        error.kind !== "conflict"
      ) {
        throw error;
      }
      lastConflict = error;
    }
  }
  throw new Error("Could not generate a unique gateway name", {
    cause: lastConflict,
  });
}
