import { describe, expect, it } from "vitest";

import { hasDashboardAdminRole } from "./session-roles";

describe("hasDashboardAdminRole", () => {
  it("returns false when only hypershell-admins is present", () => {
    expect(
      hasDashboardAdminRole(["hypershell-users", "hypershell-admins"]),
    ).toBe(false);
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
