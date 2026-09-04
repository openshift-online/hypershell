import { describe, expect, it } from "vitest";

import { hasDashboardAdminRole } from "./roles.js";

describe("hasDashboardAdminRole", () => {
  it("returns true when hypershell-admins is present", () => {
    expect(
      hasDashboardAdminRole(["hypershell-users", "hypershell-admins"]),
    ).toBe(true);
  });

  it("returns true when platform:admin is present", () => {
    expect(hasDashboardAdminRole(["hypershell-users", "platform:admin"])).toBe(
      true,
    );
  });

  it("returns false for non-admin roles", () => {
    expect(hasDashboardAdminRole(["hypershell-users", "gateway:creator"])).toBe(
      false,
    );
  });
});
