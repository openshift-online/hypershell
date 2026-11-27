/*
 * Bounded `hsctl` command paths for terminal domain facts (WEB-TERM-13,
 * UIS-OBS-07). A command-finished fact names the command only by one of these
 * paths, or `other`; it never carries arguments, flag values, or identifiers.
 *
 * Resource names follow the cobra `Use` names in `components/cli/cmd/hsctl`.
 * A resource typed with an alias (for example `gateway` for `list gateways`,
 * or `service-account`) maps to the same path. A command added to `hsctl` but
 * not listed here is reported as `other` until this table is updated.
 */

const resourceCommands = {
  create: [
    "agentRuntime",
    "ciProvider",
    "ciTrust",
    "gateway",
    "gatewayAccess",
    "inferenceRoute",
    "managedCluster",
    "platformGatewayPolicy",
    "providerBinding",
    "providerSpec",
    "role",
    "roleBinding",
    "sandboxTemplate",
    "secretSource",
    "serviceAccount",
    "user",
  ],
  delete: [
    "agentRuntime",
    "ciProvider",
    "ciTrust",
    "gateway",
    "gatewayAccess",
    "inferenceRoute",
    "managedCluster",
    "platformGatewayPolicy",
    "providerBinding",
    "providerSpec",
    "roleBinding",
    "sandboxTemplate",
    "secretSource",
    "serviceAccount",
  ],
  get: [
    "agentRuntime",
    "ciProvider",
    "ciTrust",
    "effectivePolicies",
    "gateway",
    "inferenceRoute",
    "managedCluster",
    "platformGatewayPolicy",
    "platformPolicySettings",
    "providerBinding",
    "providerSpec",
    "repository",
    "role",
    "roleBinding",
    "sandboxTemplate",
    "secretSource",
    "serviceAccount",
    "user",
  ],
  list: [
    "agentRuntimes",
    "ciProviders",
    "ciTrusts",
    "gatewayAccess",
    "gateways",
    "inferenceRoutes",
    "managedClusters",
    "platformGatewayPolicies",
    "providerBindings",
    "providerSpecs",
    "repositories",
    "roleBindings",
    "roles",
    "sandboxTemplates",
    "secretSources",
    "serviceAccounts",
    "users",
  ],
  revoke: ["serviceAccount"],
  "secret-source": ["upload"],
  update: [
    "gateway",
    "gatewayAccess",
    "managedCluster",
    "platformGatewayPolicy",
    "platformPolicySettings",
    "serviceAccount",
  ],
} as const;

const leafCommands = [
  "apply",
  "completion",
  "config",
  "help",
  "login",
  "logout",
  "tui",
  "version",
  "whoami",
] as const;

type ResourceVerb = keyof typeof resourceCommands;

type ResourceCommandPath = {
  [Verb in ResourceVerb]: `${Verb} ${(typeof resourceCommands)[Verb][number]}`;
}[ResourceVerb];

export type HsctlCommandPath =
  | "hsctl"
  | "other"
  | ResourceCommandPath
  | ResourceVerb
  | (typeof leafCommands)[number];

function isResourceVerb(word: string): word is ResourceVerb {
  return Object.hasOwn(resourceCommands, word);
}

function isLeafCommand(word: string): word is (typeof leafCommands)[number] {
  return (leafCommands as readonly string[]).includes(word);
}

// Folds case, kebab-case, and simple plurals so aliases such as
// `service-account`, `gateway`, or `repositories` match their command.
function stem(word: string): string {
  const folded = word.toLowerCase().replaceAll("-", "");
  if (folded.endsWith("ies")) {
    return `${folded.slice(0, -3)}y`;
  }
  return folded.endsWith("s") ? folded.slice(0, -1) : folded;
}

/** Every bounded path, for the probe catalog and its tests. */
export const knownHsctlCommandPaths: readonly HsctlCommandPath[] =
  Object.freeze([
    "hsctl",
    ...leafCommands,
    ...(Object.keys(resourceCommands) as ResourceVerb[]).flatMap((verb) => [
      verb,
      ...resourceCommands[verb].map(
        (resource) => `${verb} ${resource}` as ResourceCommandPath,
      ),
    ]),
    "other",
  ]);

/**
 * Maps a bridge argv (without the program name) to its bounded command path.
 * Flags and every word after the command path are ignored.
 */
export function hsctlCommandPathOf(argv: readonly string[]): HsctlCommandPath {
  const [verb, resource] = argv;
  if (verb === undefined) {
    return "hsctl";
  }
  if (isLeafCommand(verb)) {
    return verb;
  }
  if (!isResourceVerb(verb)) {
    return "other";
  }
  if (resource === undefined || resource.startsWith("-")) {
    return verb;
  }
  const wanted = stem(resource);
  const candidates: readonly string[] = resourceCommands[verb];
  const match = candidates.find((candidate) => stem(candidate) === wanted);
  // The match comes from this verb's own list, so the path is in the union.
  return match === undefined
    ? "other"
    : (`${verb} ${match}` as ResourceCommandPath);
}
