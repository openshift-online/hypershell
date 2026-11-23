import {
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Label,
  Popover,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import {
  EyeIcon,
  LockIcon,
  OutlinedQuestionCircleIcon,
  ShieldAltIcon,
} from "@patternfly/react-icons";

/*
 * Shared deterministic fixtures and renderers for the PlatformGatewayPolicy
 * and GatewayPolicy mockups (GitHub issue #1145).
 *
 * Terminology (design council decision, used on every surface):
 * - "Enforced" / "Audit only" are the two effective modes. Mode is always
 *   shown as a status label, never as a switch.
 * - "reject" (not "block") matches the OpenShell CLI denial text.
 * - "add the default" describes default injection.
 * - "Platform enforcement" is the global minimum mode.
 * - "Not applied" replaces "Conflicted".
 * - "rule" is a policy constraint; "limit" is the Kubernetes field.
 */

export type PolicyEnforcementMode = "audit" | "enforce";
export type PolicyScope = "platform" | "fleet";
export type ResourceKey =
  "cpuRequest" | "cpuLimit" | "memoryRequest" | "memoryLimit" | "gpuCount";

export const resourceRows: { key: ResourceKey; label: string }[] = [
  { key: "cpuRequest", label: "CPU request" },
  { key: "cpuLimit", label: "CPU limit" },
  { key: "memoryRequest", label: "Memory request" },
  { key: "memoryLimit", label: "Memory limit" },
  { key: "gpuCount", label: "GPUs" },
];

export type ResourceRange = { min?: string; default?: string; max?: string };
export type PolicyRules = Partial<Record<ResourceKey, ResourceRange>>;

export type PolicyFinding = {
  time: string;
  user: string;
  gateway: string;
  operation: string;
  outcome: "reject" | "default";
  detail: string;
  policy: string;
};

export type RolloutEntry = {
  gateway: string;
  state: "applied" | "pending" | "unsupported";
  revision: string;
  enforced: boolean;
  enforcementReason: string;
  wouldReject24h: number;
  detail?: string;
};

export type Policy = {
  name: string;
  scope: PolicyScope;
  /** Mode set on the policy itself (by platform admins or gateway owners). */
  mode: PolicyEnforcementMode;
  userMessage: string;
  documentationUrl?: string;
  appliesTo?: string;
  /** Bounds the UI renderer recognized in the Cedar text. */
  rules: PolicyRules;
  /** Plain descriptions of statements the renderer cannot summarize. */
  unrecognized?: string[];
  cedar: string;
  updated: string;
  updatedBy: string;
  wouldReject24h: number;
  wouldDefault24h: number;
  notApplied?: string;
  rollout?: RolloutEntry[];
};

export const baselinePolicy: Policy = {
  name: "sandbox-resource-baseline",
  scope: "platform",
  mode: "audit",
  userMessage:
    "Sandboxes on shared clusters must set CPU and memory within platform limits. Ask #hypershell-help for an exception.",
  documentationUrl: "https://docs.example.com/hypershell/sandbox-resources",
  appliesTo: "All gateways",
  rules: {
    cpuRequest: { min: "250m", max: "4" },
    cpuLimit: { max: "8" },
    memoryRequest: { min: "512Mi", max: "16Gi" },
    memoryLimit: { max: "32Gi" },
  },
  cedar: `forbid (principal, action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"], resource)
when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };

forbid (principal, action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"], resource)
when { context.cpuLimitMillis > 8000 };

forbid (principal, action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"], resource)
when { context.memoryRequestBytes < 536870912 || context.memoryRequestBytes > 17179869184 };

forbid (principal, action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"], resource)
when { context.memoryLimitBytes > 34359738368 };`,
  updated: "Nov 18, 2026",
  updatedBy: "grace.hopper",
  wouldReject24h: 12,
  wouldDefault24h: 41,
  rollout: [
    {
      gateway: "payments-gateway",
      state: "applied",
      revision: "r7",
      enforced: false,
      enforcementReason: "Policy setting",
      wouldReject24h: 2,
    },
    {
      gateway: "research-gateway",
      state: "applied",
      revision: "r7",
      enforced: false,
      enforcementReason: "Policy setting",
      wouldReject24h: 0,
    },
    {
      gateway: "ml-training-gateway",
      state: "pending",
      revision: "r6",
      enforced: false,
      enforcementReason: "Policy setting",
      wouldReject24h: 10,
      detail: "Waiting for the gateway to confirm revision r7.",
    },
    {
      gateway: "edge-lab-gateway",
      state: "unsupported",
      revision: "-",
      enforced: false,
      enforcementReason: "Cannot enforce",
      wouldReject24h: 0,
      detail: "The VM compute driver ignores per-sandbox CPU and memory.",
    },
  ],
};

export const gpuPolicy: Policy = {
  name: "gpu-sandbox-cap",
  scope: "platform",
  mode: "enforce",
  userMessage:
    "Sandboxes can use 1 GPU unless you are in gpu-users. Use an ML training gateway for larger jobs.",
  appliesTo: "All gateways except 2",
  rules: {},
  unrecognized: ["More than 1 GPU only for members of gpu-users"],
  cedar: `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.gpuCount > 1 }
unless { principal.roles.contains("gpu-users") };`,
  updated: "Nov 02, 2026",
  updatedBy: "grace.hopper",
  wouldReject24h: 0,
  wouldDefault24h: 0,
  rollout: [
    {
      gateway: "payments-gateway",
      state: "applied",
      revision: "r3",
      enforced: true,
      enforcementReason: "Policy setting",
      wouldReject24h: 0,
    },
    {
      gateway: "research-gateway",
      state: "applied",
      revision: "r3",
      enforced: true,
      enforcementReason: "Policy setting",
      wouldReject24h: 0,
    },
  ],
};

export const platformPolicies = [baselinePolicy, gpuPolicy];

export const findings: PolicyFinding[] = [
  {
    time: "Nov 20, 2026, 9:12 AM",
    user: "lin.chen",
    gateway: "payments-gateway",
    operation: "Create sandbox",
    outcome: "reject",
    detail: "CPU request 100m is below the minimum 250m",
    policy: "sandbox-resource-baseline",
  },
  {
    time: "Nov 20, 2026, 8:47 AM",
    user: "ada.lovelace",
    gateway: "payments-gateway",
    operation: "Create sandbox",
    outcome: "default",
    detail: "Added CPU limit 2 and memory limit 4Gi",
    policy: "sandbox-resource-baseline",
  },
  {
    time: "Nov 19, 2026, 4:03 PM",
    user: "sam.okafor",
    gateway: "payments-gateway",
    operation: "Resume sandbox",
    outcome: "reject",
    detail: "Memory request 256Mi is below the minimum 512Mi",
    policy: "sandbox-resource-baseline",
  },
];

/** One bound of the effective result, with the policy that sets it. */
export type EffectiveBound = {
  value: string;
  source: string;
  scope: PolicyScope;
  enforced: boolean;
};

export type EffectiveRow = {
  key: ResourceKey;
  min?: EffectiveBound;
  default?: EffectiveBound;
  max?: EffectiveBound;
};

/** Fleet-wide defaults (controller configuration, not policy). */
export const fleetDefaults: Partial<Record<ResourceKey, string>> = {
  cpuRequest: "500m",
  cpuLimit: "2",
  memoryRequest: "1Gi",
  memoryLimit: "4Gi",
};

/** Effective limits for payments-gateway from recognized platform rules. */
export function effectiveLimits(
  platformEnforcement: PolicyEnforcementMode,
): EffectiveRow[] {
  const enforced = platformEnforcement === "enforce";
  const baseline = (value: string): EffectiveBound => ({
    value,
    source: baselinePolicy.name,
    scope: "platform",
    enforced,
  });
  const fleet = (key: ResourceKey): EffectiveBound | undefined =>
    fleetDefaults[key]
      ? {
          value: fleetDefaults[key] as string,
          source: "Fleet default",
          scope: "fleet",
          enforced: true,
        }
      : undefined;
  return [
    {
      key: "cpuRequest",
      min: baseline("250m"),
      default: fleet("cpuRequest"),
      max: baseline("4"),
    },
    { key: "cpuLimit", default: fleet("cpuLimit"), max: baseline("8") },
    {
      key: "memoryRequest",
      min: baseline("512Mi"),
      default: fleet("memoryRequest"),
      max: baseline("16Gi"),
    },
    {
      key: "memoryLimit",
      default: fleet("memoryLimit"),
      max: baseline("32Gi"),
    },
  ];
}

export function ruleChips(rules: PolicyRules): string[] {
  return resourceRows.filter(({ key }) => rules[key]).map(({ label }) => label);
}

/** Effective mode badge. Mode is always a status, never a toggle. */
export function ModeLabel({
  enforced,
  isCompact = false,
}: {
  enforced: boolean;
  isCompact?: boolean;
}) {
  return enforced ? (
    <Label color="blue" icon={<ShieldAltIcon />} isCompact={isCompact}>
      Enforced
    </Label>
  ) : (
    <Label
      color="grey"
      icon={<EyeIcon />}
      isCompact={isCompact}
      variant="outline"
    >
      Audit only
    </Label>
  );
}

/** Effective mode with the visible reason (never a tooltip-only reason). */
export function ModeWithReason({
  enforced,
  reason,
}: {
  enforced: boolean;
  reason: string;
}) {
  return (
    <Stack>
      <StackItem>
        <ModeLabel enforced={enforced} />
      </StackItem>
      <StackItem>
        <Content component="small">{reason}</Content>
      </StackItem>
    </Stack>
  );
}

export function SourceLabel({
  name,
  scope,
}: {
  name: string;
  scope: PolicyScope;
}) {
  return scope === "platform" ? (
    <Label color="grey" icon={<LockIcon />} isCompact>
      {name}
    </Label>
  ) : (
    <Label color="grey" isCompact variant="outline">
      {name}
    </Label>
  );
}

export function ModeExplanationPopover({
  platformEnforcement,
}: {
  platformEnforcement: PolicyEnforcementMode;
}) {
  return (
    <Popover
      bodyContent={
        <Stack hasGutter>
          <StackItem>
            Enforced rules reject sandbox operations that break them and add
            missing defaults. Audit-only rules allow the operation and record
            what they would reject.
          </StackItem>
          <StackItem>
            A policy is enforced when platform enforcement or the policy itself
            is set to Enforced. Platform policy administrators manage both.
          </StackItem>
          <StackItem>
            Platform enforcement is{" "}
            {platformEnforcement === "enforce" ? "Enforced" : "Audit only"}.
          </StackItem>
        </Stack>
      }
      headerContent="How enforcement works"
    >
      <Button
        aria-label="How enforcement works"
        icon={<OutlinedQuestionCircleIcon />}
        variant="plain"
      />
    </Popover>
  );
}

function BoundCell({ bound }: { bound?: EffectiveBound }) {
  if (!bound) {
    return <Content component="small">No rule</Content>;
  }
  return (
    <Stack>
      <StackItem>
        <Content component="p">
          <strong>{bound.value}</strong>
        </Content>
      </StackItem>
      <StackItem>
        <Flex flexWrap={{ default: "wrap" }} gap={{ default: "gapXs" }}>
          <FlexItem>
            <SourceLabel name={bound.source} scope={bound.scope} />
          </FlexItem>
          {bound.enforced || bound.scope === "fleet" ? null : (
            <FlexItem>
              <ModeLabel enforced={false} isCompact />
            </FlexItem>
          )}
        </Flex>
      </StackItem>
    </Stack>
  );
}

/**
 * Effective limits matrix. Rows are a fixed resource list, not a data set,
 * so columns are not sortable (technical exception in ui-conventions.md).
 */
export function EffectiveLimitsTable({ rows }: { rows: EffectiveRow[] }) {
  return (
    <Table aria-label="Sandbox limits on this gateway">
      <Thead>
        <Tr>
          <Th width={20}>Resource</Th>
          <Th width={25}>Minimum</Th>
          <Th width={30}>Default if missing</Th>
          <Th width={25}>Maximum</Th>
        </Tr>
      </Thead>
      <Tbody>
        {rows.map((row) => (
          <Tr key={row.key}>
            <Th dataLabel="Resource">
              {resourceRows.find(({ key }) => key === row.key)?.label}
            </Th>
            <Td dataLabel="Minimum">
              <BoundCell bound={row.min} />
            </Td>
            <Td dataLabel="Default if missing">
              <BoundCell bound={row.default} />
            </Td>
            <Td dataLabel="Maximum">
              <BoundCell bound={row.max} />
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}

/** One policy's own rules as a min/default/max matrix (fixed rows). */
export function PolicyRulesTable({
  name,
  rules,
}: {
  name: string;
  rules: PolicyRules;
}) {
  return (
    <Table aria-label={`Rules in ${name}`} variant="compact">
      <Thead>
        <Tr>
          <Th modifier="wrap">Resource</Th>
          <Th modifier="wrap">Minimum</Th>
          <Th modifier="wrap">Default if missing</Th>
          <Th modifier="wrap">Maximum</Th>
        </Tr>
      </Thead>
      <Tbody>
        {resourceRows
          .filter(({ key }) => rules[key])
          .map(({ key, label }) => (
            <Tr key={key}>
              <Th dataLabel="Resource">{label}</Th>
              <Td dataLabel="Minimum">{rules[key]?.min ?? "No rule"}</Td>
              <Td dataLabel="Default if missing">
                {rules[key]?.default ?? "No rule"}
              </Td>
              <Td dataLabel="Maximum">{rules[key]?.max ?? "No rule"}</Td>
            </Tr>
          ))}
      </Tbody>
    </Table>
  );
}

/** Why a policy is Enforced or Audit only on one gateway. */
export function ModeChain({
  platformEnforcement,
  policyMode,
}: {
  platformEnforcement: PolicyEnforcementMode;
  policyMode: PolicyEnforcementMode;
}) {
  const enforced =
    platformEnforcement === "enforce" || policyMode === "enforce";
  const word = (mode: PolicyEnforcementMode) =>
    mode === "enforce" ? "Enforced" : "Audit only";
  return (
    <DescriptionList isCompact isHorizontal>
      <DescriptionListGroup>
        <DescriptionListTerm>Platform enforcement</DescriptionListTerm>
        <DescriptionListDescription>
          {word(platformEnforcement)}
        </DescriptionListDescription>
      </DescriptionListGroup>
      <DescriptionListGroup>
        <DescriptionListTerm>Policy setting</DescriptionListTerm>
        <DescriptionListDescription>
          {word(policyMode)}
        </DescriptionListDescription>
      </DescriptionListGroup>
      <DescriptionListGroup>
        <DescriptionListTerm>Result</DescriptionListTerm>
        <DescriptionListDescription>
          <ModeLabel enforced={enforced} />
        </DescriptionListDescription>
      </DescriptionListGroup>
    </DescriptionList>
  );
}

export function OutcomeLabel({
  outcome,
}: {
  outcome: PolicyFinding["outcome"];
}) {
  return outcome === "reject" ? (
    <Label color="red" isCompact>
      Would reject
    </Label>
  ) : (
    <Label color="blue" isCompact variant="outline">
      Would add defaults
    </Label>
  );
}
