import type { IntlShape } from "react-intl";

import type { OperationalMetricPodPhases } from "../application/dashboard-types";
import { messages } from "../messages";
import { STATUS_DONUT_COLORS } from "./status-donut-colors";
import {
  buildStatusDonutData,
  type StatusDonutDatum,
  type StatusDonutLegendDatum,
  type StatusDonutSeries,
} from "./status-donut-data";

export const POD_CAPACITY_PHASE_ORDER = [
  "running",
  "pending",
  "failed",
  "succeeded",
  "unknown",
  "unused",
] as const;

type PodCapacityPhaseKey = (typeof POD_CAPACITY_PHASE_ORDER)[number];

export type PodCapacityDatum = StatusDonutDatum;
export type PodCapacityLegendDatum = StatusDonutLegendDatum;

function podCapacityPhaseLabel(
  intl: IntlShape,
  phase: Exclude<PodCapacityPhaseKey, "unused">,
): string {
  switch (phase) {
    case "running":
      return intl.formatMessage(messages.podStatusRunning);
    case "pending":
      return intl.formatMessage(messages.podStatusPending);
    case "failed":
      return intl.formatMessage(messages.podStatusFailed);
    case "succeeded":
      return intl.formatMessage(messages.podStatusSucceeded);
    case "unknown":
      return intl.formatMessage(messages.podStatusUnknown);
  }
}

function podCapacityPhaseColor(
  phase: Exclude<PodCapacityPhaseKey, "unused">,
): string {
  switch (phase) {
    case "running":
      return STATUS_DONUT_COLORS.healthy;
    case "pending":
      return STATUS_DONUT_COLORS.provisioning;
    case "failed":
      return STATUS_DONUT_COLORS.failed;
    case "succeeded":
      return STATUS_DONUT_COLORS.degraded;
    case "unknown":
      return "#8b8d8f";
  }
}

export function buildPodCapacityData(
  intl: IntlShape,
  usedPods: number,
  capacityPods: number,
  podPhases: OperationalMetricPodPhases,
): StatusDonutSeries {
  const unusedPods = Math.max(capacityPods - usedPods, 0);
  const phaseCounts: Record<Exclude<PodCapacityPhaseKey, "unused">, number> = {
    failed: podPhases.failed,
    pending: podPhases.pending,
    running: podPhases.running,
    succeeded: podPhases.succeeded,
    unknown: podPhases.unknown,
  };

  const phaseEntries = POD_CAPACITY_PHASE_ORDER.filter(
    (phase): phase is Exclude<PodCapacityPhaseKey, "unused"> =>
      phase !== "unused",
  ).map((phase) => {
    const count = phaseCounts[phase];
    const label = podCapacityPhaseLabel(intl, phase);

    return {
      color: podCapacityPhaseColor(phase),
      count,
      label,
      legendName: intl.formatMessage(messages.statusDonutLegend, {
        count,
        status: label,
      }),
    };
  });

  const unusedLabel = intl.formatMessage(messages.podStatusUnused);

  return buildStatusDonutData([
    ...phaseEntries,
    {
      color: STATUS_DONUT_COLORS.unused,
      count: unusedPods,
      label: unusedLabel,
      legendName: intl.formatMessage(messages.statusDonutLegend, {
        count: unusedPods,
        status: unusedLabel,
      }),
    },
  ]);
}
