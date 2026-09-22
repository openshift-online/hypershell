import type { OperationalDashboardMetrics } from "../application/dashboard-types";

function buildSevenDayTrendPoints(
  baseValue: number,
  step: number,
): { label: string; value: number }[] {
  const points: { label: string; value: number }[] = [];
  const today = new Date();
  const utcToday = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  for (let offset = 6; offset >= 0; offset -= 1) {
    const day = new Date(utcToday - offset * 24 * 60 * 60 * 1000);
    points.push({
      label: day.toISOString().slice(0, 10),
      value: baseValue + (6 - offset) * step,
    });
  }

  return points;
}

function buildHourlyTrendPoints(
  baseValue: number,
  step: number,
): { label: string; value: number }[] {
  const points: { label: string; value: number }[] = [];
  const end = new Date();
  const endHour = Date.UTC(
    end.getUTCFullYear(),
    end.getUTCMonth(),
    end.getUTCDate(),
    end.getUTCHours(),
  );

  for (let offset = 23; offset >= 0; offset -= 1) {
    const hour = new Date(endHour - offset * 60 * 60 * 1000);
    const year = hour.getUTCFullYear();
    const month = String(hour.getUTCMonth() + 1).padStart(2, "0");
    const day = String(hour.getUTCDate()).padStart(2, "0");
    const hourLabel = String(hour.getUTCHours()).padStart(2, "0");
    points.push({
      label: `${String(year)}-${month}-${day}T${hourLabel}:00`,
      value: baseValue + (23 - offset) * step,
    });
  }

  return points;
}

function buildRegisteredUsersTrendPoints(): {
  label: string;
  value: number;
}[] {
  const points: { label: string; value: number }[] = [];
  const today = new Date();
  const utcToday = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  for (let offset = 29; offset >= 0; offset -= 1) {
    const day = new Date(utcToday - offset * 24 * 60 * 60 * 1000);
    points.push({
      label: day.toISOString().slice(0, 10),
      value: 120 + (29 - offset) * 6,
    });
  }

  return points;
}

/**
 * Storybook and local-dev fixture shaped like `createDashboardControlPlaneAdapter`
 * output, including registered-user adoption fields.
 */
export const mockOperationalDashboardMetrics: OperationalDashboardMetrics =
  Object.freeze({
    metrics: Object.freeze([
      Object.freeze({
        id: "provisioned-gateways",
        status: Object.freeze({
          degraded: 6,
          failed: 2,
          healthy: 80,
          provisioning: 9,
        }),
        trend: Object.freeze({
          points: Object.freeze(buildSevenDayTrendPoints(80, 2)),
        }),
        value: "97",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(180, 1)),
        }),
        id: "provisioned-sandboxes",
        trend: Object.freeze({
          points: Object.freeze(buildSevenDayTrendPoints(190, 3)),
        }),
        value: "214",
      }),
      Object.freeze({
        id: "gateway-releases",
        releaseDistribution: Object.freeze({
          "OpenShell 2.0": 62,
          "OpenShell 2.1-canary": 12,
          unknown: 3,
        }),
        value: "77",
      }),
      Object.freeze({
        createdLast7Days: "12",
        createdLast30Days: "48",
        id: "registered-users",
        trend: Object.freeze({
          points: Object.freeze(buildRegisteredUsersTrendPoints()),
        }),
        uniqueLoginsLast7Days: "186",
        uniqueLoginsLast30Days: "312",
        value: "450",
      }),
      Object.freeze({
        id: "memory",
        total: "237",
        trend: Object.freeze({
          points: Object.freeze(buildSevenDayTrendPoints(200, 3)),
        }),
        unit: "GiB",
        value: "220",
      }),
      Object.freeze({
        id: "cpu",
        total: "60",
        trend: Object.freeze({
          points: Object.freeze(buildSevenDayTrendPoints(40, 1)),
        }),
        unit: "cores",
        value: "48",
      }),
      Object.freeze({
        id: "pods",
        podPhases: Object.freeze({
          failed: 16,
          pending: 12,
          running: 500,
          succeeded: 20,
          unknown: 0,
        }),
        total: "2000",
        trend: Object.freeze({
          points: Object.freeze(buildSevenDayTrendPoints(500, 7)),
        }),
        unit: "pods",
        value: "548",
      }),
      Object.freeze({
        id: "nodes",
        status: Object.freeze({
          failed: 1,
          healthy: 7,
        }),
        value: "8",
      }),
      Object.freeze({
        id: "provision-time",
        provisionDuration: Object.freeze({
          mean: "315.00",
          p50: "288.00",
          p95: "726.00",
        }),
        unit: "sec",
        value: "315.00",
      }),
      Object.freeze({
        id: "provision-reliability",
        provisionOutcomes: Object.freeze({
          failureCount24h: "1",
          successCount24h: "9",
          successRatePercent: "90.0",
        }),
        successRateTrend: Object.freeze({
          points: Object.freeze([
            Object.freeze({ label: "2026-08-09T12:00", value: 80 }),
            Object.freeze({ label: "2026-08-09T13:00", value: 100 }),
          ]),
        }),
        value: "90.0",
      }),
      Object.freeze({
        createdLast30Days: "2",
        id: "managed-clusters",
        inventoryProviders: Object.freeze({
          aws: 5,
          gcp: 2,
          ibm: 1,
        }),
        inventoryRegions: Object.freeze({
          "eu-west-1 (aws)": 1,
          "eu-west-1 (gcp)": 2,
          "us-east-1 (aws)": 4,
          "us-east-1 (ibm)": 1,
        }),
        inventoryStatus: Object.freeze({
          Ready: 6,
          unknown: 2,
        }),
        value: "8",
      }),
      Object.freeze({
        id: "managed-databases",
        inventoryStatus: Object.freeze({
          Ready: 2,
          unknown: 1,
        }),
        value: "3",
      }),
    ]),
    lastSuccessfulRefresh: new Date("2026-08-25T10:55:00.000Z"),
  });
