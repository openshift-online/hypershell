import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type AdoptionDailyPoint,
  queryAdoptionMetrics,
} from "../src/metrics-adoption.js";

function scalarBody(value: number) {
  return JSON.stringify({
    status: "success",
    data: {
      result: [
        {
          metric: {},
          value: ["1704067200", String(value)],
        },
      ],
    },
  });
}

function vectorBody(labels: Record<string, string>, value: number) {
  return JSON.stringify({
    status: "success",
    data: {
      result: [
        {
          metric: labels,
          value: ["1704067200", String(value)],
        },
      ],
    },
  });
}

function prometheusResponse(body: string, status = 200) {
  return new Response(body, { status });
}

describe("queryAdoptionMetrics", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("maps aggregate Prometheus samples to the adoption contract", async () => {
    const today = new Date().toISOString().slice(0, 10);
    const fetchMock = vi.fn((input: URL) => {
      const query = input.searchParams.get("query") ?? "";
      if (query === "hypershell_users_registered_total") {
        return prometheusResponse(scalarBody(450));
      }
      if (query === "hypershell_users_created_last_7_days_total") {
        return prometheusResponse(scalarBody(48));
      }
      if (query === "hypershell_users_created_last_30_days_total") {
        return prometheusResponse(scalarBody(123));
      }
      if (query === "hypershell_users_unique_logins_last_7_days_total") {
        return prometheusResponse(scalarBody(312));
      }
      if (query === "hypershell_users_unique_logins_last_30_days_total") {
        return prometheusResponse(scalarBody(398));
      }
      if (query === "hypershell_users_inactive_last_30_days_total") {
        return prometheusResponse(scalarBody(52));
      }
      if (query === "hypershell_role_bindings_gateway_owner_users_total") {
        return prometheusResponse(scalarBody(306));
      }
      if (query === "hypershell_users_retention_7_days_percent") {
        return prometheusResponse(scalarBody(78));
      }
      if (query === "hypershell_users_created_daily_total") {
        return prometheusResponse(vectorBody({ created_date: today }, 18));
      }
      if (query === "hypershell_users_unique_logins_daily_total") {
        return prometheusResponse(vectorBody({ activity_date: today }, 42));
      }
      return prometheusResponse(JSON.stringify({ status: "error" }), 500);
    });
    vi.stubGlobal("fetch", fetchMock);

    const response = await queryAdoptionMetrics(
      "https://prometheus.example",
      5_000,
    );

    expect(response).toMatchObject({
      registered_users: 450,
      created_last_7_days: 48,
      created_last_30_days: 123,
      unique_logins_last_7_days: 312,
      unique_logins_last_30_days: 398,
      inactive_users_last_30_days: 52,
      users_with_gateway: 306,
      gateway_conversion_percent: 68,
      retention_7_days_percent: 78,
    });
    const expectedCreatedPoint: AdoptionDailyPoint = {
      date: today,
      count: 18,
    };
    const expectedLoginPoint: AdoptionDailyPoint = { date: today, count: 42 };
    expect(response.daily_created_users).toContainEqual(expectedCreatedPoint);
    expect(response.daily_unique_logins).toContainEqual(expectedLoginPoint);
    expect(response.daily_created_users).toHaveLength(30);
    expect(response.daily_unique_logins).toHaveLength(30);
    expect(JSON.stringify(response)).not.toContain("user_id");
  });

  it("preserves the required aggregate when optional sources fail", async () => {
    const fetchMock = vi.fn((input: URL) => {
      const query = input.searchParams.get("query") ?? "";
      if (query === "hypershell_users_registered_total") {
        return prometheusResponse(scalarBody(42));
      }
      return prometheusResponse("{}", 500);
    });
    vi.stubGlobal("fetch", fetchMock);

    const response = await queryAdoptionMetrics(
      "https://prometheus.example",
      5_000,
    );

    expect(response).toEqual({ registered_users: 42 });
  });
});
