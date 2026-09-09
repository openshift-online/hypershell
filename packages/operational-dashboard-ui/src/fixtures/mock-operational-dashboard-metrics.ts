import type { OperationalDashboardMetrics } from "../application/dashboard-types";

/**
 * Storybook and local-dev fixture shaped like `createDashboardControlPlaneAdapter`
 * output: instantaneous values only (no trend series in production v1).
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
        value: "97",
      }),
      Object.freeze({
        id: "provisioned-sandboxes",
        value: "214",
      }),
      Object.freeze({
        id: "registered-users",
        value: "450",
      }),
      Object.freeze({
        id: "memory",
        total: "237",
        unit: "GiB",
        value: "220",
      }),
      Object.freeze({
        id: "cpu",
        total: "60",
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
          mean: "5.25",
          p50: "4.80",
          p95: "12.10",
        }),
        unit: "minutes",
        value: "5.25",
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
