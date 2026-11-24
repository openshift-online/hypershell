import {
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Label,
  Popover,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import {
  EyeIcon,
  LockIcon,
  OutlinedQuestionCircleIcon,
  ShieldAltIcon,
} from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import type { ReactNode } from "react";
import { useIntl, type IntlShape } from "react-intl";

import type {
  PolicyFinding,
  PolicyFindingOutcome,
  PolicyFleetDefaults,
  PolicyMode,
  PolicyModeReason,
  PolicyRenderedRule,
  PolicyResourceKey,
} from "../application/policy-types";
import { CedarCodeBlock } from "./cedar-code-block";
import { policyMessages } from "./policy-messages";
import {
  fleetDefaultFor,
  formatPolicyFieldValue,
  policyResourceFields,
  policyResourceForField,
  rowBounds,
  type PolicyRowBound,
} from "./policy-quantities";

/*
 * Shared rendered-rule presentation (specs/web-console/mockups/common/
 * policy-rules.tsx) used by the read-only gateway Policies tab (WEB-GLIS-26)
 * and the platform policy admin panel (WEB-PADM-02/04/09). Mode is always a
 * status label, never a switch.
 */

const resourceMessages = {
  cpuLimit: policyMessages.resourceCpuLimit,
  cpuRequest: policyMessages.resourceCpuRequest,
  gpuCount: policyMessages.resourceGpuCount,
  memoryLimit: policyMessages.resourceMemoryLimit,
  memoryRequest: policyMessages.resourceMemoryRequest,
} as const;

export function policyResourceLabel(
  intl: IntlShape,
  resource: PolicyResourceKey,
): string {
  return intl.formatMessage(resourceMessages[resource]);
}

/** Resource names a policy's recognized rules bound, in display order. */
export function policyRuleResources(
  rules: readonly PolicyRenderedRule[],
): readonly PolicyResourceKey[] {
  const present = new Set(rowBounds(rules).map(({ resource }) => resource));
  return policyResourceFields
    .map(([resource]) => resource)
    .filter((resource) => present.has(resource));
}

/** Effective mode badge. */
export function PolicyModeLabel({
  enforced,
  isCompact = false,
}: {
  enforced: boolean;
  isCompact?: boolean;
}) {
  const intl = useIntl();
  return enforced ? (
    <Label color="blue" icon={<ShieldAltIcon />} isCompact={isCompact}>
      {intl.formatMessage(policyMessages.modeEnforced)}
    </Label>
  ) : (
    <Label
      color="grey"
      icon={<EyeIcon />}
      isCompact={isCompact}
      variant="outline"
    >
      {intl.formatMessage(policyMessages.modeAuditOnly)}
    </Label>
  );
}

export function policyModeReasonText(
  intl: IntlShape,
  reason: PolicyModeReason,
): string {
  return intl.formatMessage(
    reason === "platform-enforced"
      ? policyMessages.reasonPlatformEnforced
      : policyMessages.reasonPolicySetting,
  );
}

/** Effective mode with the visible reason (never a tooltip-only reason). */
export function PolicyModeWithReason({
  enforced,
  reason,
}: {
  enforced: boolean;
  reason: ReactNode;
}) {
  return (
    <Stack>
      <StackItem>
        <PolicyModeLabel enforced={enforced} />
      </StackItem>
      <StackItem>
        <Content component="small">{reason}</Content>
      </StackItem>
    </Stack>
  );
}

/** Attribution label for the policy a limit comes from. */
export function PolicySourceLabel({ name }: { name: string }) {
  return (
    <Label color="grey" icon={<LockIcon />} isCompact>
      {name}
    </Label>
  );
}

export function PolicyOutcomeLabel({
  outcome,
}: {
  outcome: PolicyFindingOutcome;
}) {
  const intl = useIntl();
  return outcome === "would-reject" ? (
    <Label color="red" isCompact>
      {intl.formatMessage(policyMessages.outcomeWouldReject)}
    </Label>
  ) : (
    <Label color="blue" isCompact variant="outline">
      {intl.formatMessage(policyMessages.outcomeWouldAddDefaults)}
    </Label>
  );
}

export function PolicyModeExplanationPopover({
  globalMode,
}: {
  globalMode: PolicyMode;
}) {
  const intl = useIntl();
  return (
    <Popover
      bodyContent={
        <Stack hasGutter>
          <StackItem>
            {intl.formatMessage(policyMessages.howEnforcementWorksBody)}
          </StackItem>
          <StackItem>
            {intl.formatMessage(policyMessages.howEnforcementWorksWho)}
          </StackItem>
          <StackItem>
            {intl.formatMessage(policyMessages.platformEnforcementIs, {
              mode: globalMode,
            })}
          </StackItem>
        </Stack>
      }
      headerContent={intl.formatMessage(policyMessages.howEnforcementWorks)}
    >
      <Button
        aria-label={intl.formatMessage(policyMessages.howEnforcementWorks)}
        icon={<OutlinedQuestionCircleIcon />}
        variant="plain"
      />
    </Popover>
  );
}

/** Why a policy is Enforced or Audit only on one gateway. */
export function PolicyModeChain({
  effectiveMode,
  globalMode,
  policyMode,
}: {
  effectiveMode: PolicyMode;
  globalMode: PolicyMode;
  policyMode: PolicyMode;
}) {
  const intl = useIntl();
  return (
    <DescriptionList isCompact isHorizontal>
      <DescriptionListGroup>
        <DescriptionListTerm>
          {intl.formatMessage(policyMessages.platformEnforcement)}
        </DescriptionListTerm>
        <DescriptionListDescription>
          {intl.formatMessage(policyMessages.modeWord, { mode: globalMode })}
        </DescriptionListDescription>
      </DescriptionListGroup>
      <DescriptionListGroup>
        <DescriptionListTerm>
          {intl.formatMessage(policyMessages.policySetting)}
        </DescriptionListTerm>
        <DescriptionListDescription>
          {intl.formatMessage(policyMessages.modeWord, { mode: policyMode })}
        </DescriptionListDescription>
      </DescriptionListGroup>
      <DescriptionListGroup>
        <DescriptionListTerm>
          {intl.formatMessage(policyMessages.modeResult)}
        </DescriptionListTerm>
        <DescriptionListDescription>
          <PolicyModeLabel enforced={effectiveMode === "enforce"} />
        </DescriptionListDescription>
      </DescriptionListGroup>
    </DescriptionList>
  );
}

function boundsText(bounds: readonly PolicyRowBound[]): string | undefined {
  return bounds.length
    ? bounds.map(({ bound }) => bound.display).join(", ")
    : undefined;
}

/**
 * One policy's recognized rules as a min/default/max matrix. Rows are a fixed
 * resource list, not a data set, so columns are not sortable.
 */
export function PolicyRulesTable({
  defaults,
  name,
  rules,
}: {
  defaults?: PolicyFleetDefaults;
  name: string;
  rules: readonly PolicyRenderedRule[];
}) {
  const intl = useIntl();
  const bounds = rowBounds(rules);
  const noRule = intl.formatMessage(policyMessages.noRule);
  return (
    <Table
      aria-label={intl.formatMessage(policyMessages.rulesInPolicy, { name })}
      variant="compact"
    >
      <Thead>
        <Tr>
          <Th modifier="wrap">
            {intl.formatMessage(policyMessages.columnResource)}
          </Th>
          <Th modifier="wrap">
            {intl.formatMessage(policyMessages.columnMinimum)}
          </Th>
          <Th modifier="wrap">
            {intl.formatMessage(policyMessages.columnDefault)}
          </Th>
          <Th modifier="wrap">
            {intl.formatMessage(policyMessages.columnMaximum)}
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {policyRuleResources(rules).map((resource) => {
          const resourceBounds = bounds.filter(
            (bound) => bound.resource === resource,
          );
          const label = policyResourceLabel(intl, resource);
          return (
            <Tr key={resource}>
              <Th dataLabel={intl.formatMessage(policyMessages.columnResource)}>
                {label}
              </Th>
              <Td dataLabel={intl.formatMessage(policyMessages.columnMinimum)}>
                {boundsText(
                  resourceBounds.filter(({ side }) => side === "minimum"),
                ) ?? noRule}
              </Td>
              <Td dataLabel={intl.formatMessage(policyMessages.columnDefault)}>
                {(defaults && fleetDefaultFor(defaults, resource)) ?? noRule}
              </Td>
              <Td dataLabel={intl.formatMessage(policyMessages.columnMaximum)}>
                {boundsText(
                  resourceBounds.filter(({ side }) => side === "maximum"),
                ) ?? noRule}
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

/** A rule shown as its Cedar source because it has no plain-words summary. */
export function PolicyCedarRule({ rule }: { rule: PolicyRenderedRule }) {
  const intl = useIntl();
  return (
    <Stack>
      <StackItem>
        <Label color="grey" isCompact variant="outline">
          {intl.formatMessage(policyMessages.shownAsCedar)}
        </Label>
      </StackItem>
      <StackItem>
        <CedarCodeBlock source={rule.cedar} />
      </StackItem>
    </Stack>
  );
}

/** Formats an audit finding's request values and policy names. */
export function policyFindingDetail(
  intl: IntlShape,
  finding: PolicyFinding,
): string {
  const values = Object.entries(finding.requestSummary).flatMap(
    ([field, value]) => {
      const resource = policyResourceForField(field);
      return resource
        ? [
            intl.formatMessage(policyMessages.findingRequestValue, {
              resource: policyResourceLabel(intl, resource),
              value: formatPolicyFieldValue(field, value),
            }),
          ]
        : [];
    },
  );
  return intl.formatMessage(policyMessages.findingDetail, {
    policies: intl.formatList(finding.policyNames),
    request: values.length
      ? intl.formatList(values)
      : intl.formatMessage(policyMessages.findingNoValues),
  });
}

/** Names the operation of an audit finding. */
export function policyOperationText(
  intl: IntlShape,
  operation: string,
): string {
  return operation === "CreateSandbox" || operation === "create_sandbox"
    ? intl.formatMessage(policyMessages.operationCreateSandbox)
    : operation;
}

/** Formats a timestamp the way policy tables show it. */
export function formatPolicyTime(intl: IntlShape, value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : intl.formatDate(date, { dateStyle: "medium", timeStyle: "short" });
}
