import { describe, expect, it } from "vitest";

import { isDashboardHost } from "./dashboard-host";

describe("isDashboardHost", () => {
  it("matches dashboard hostnames with any domain suffix", () => {
    expect(isDashboardHost("dashboard.hypershell.localhost")).toBe(true);
    expect(isDashboardHost("dashboard.example.com")).toBe(true);
  });

  it("rejects non-dashboard hostnames", () => {
    expect(isDashboardHost("console.hypershell.localhost")).toBe(false);
    expect(isDashboardHost("hypershell.localhost")).toBe(false);
    expect(isDashboardHost("not-dashboard.example.com")).toBe(false);
  });
});
