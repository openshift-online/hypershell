import { describe, expect, it } from "vitest";

import { extractRealmRoles } from "./auth.js";

describe("extractRealmRoles", () => {
  it("reads roles from the roles claim", () => {
    expect(
      extractRealmRoles({
        roles: ["hypershell-admins", "hypershell-users"],
      }),
    ).toEqual(["hypershell-admins", "hypershell-users"]);
  });

  it("reads realm roles from the groups claim used by HyperShell Keycloak", () => {
    expect(
      extractRealmRoles({
        groups: ["hypershell-admins", "hypershell-users"],
      }),
    ).toEqual(["hypershell-admins", "hypershell-users"]);
  });

  it("normalizes leading slashes on groups claim values", () => {
    expect(
      extractRealmRoles({
        groups: ["/hypershell-admins", "/hypershell-users"],
      }),
    ).toEqual(["hypershell-admins", "hypershell-users"]);
  });

  it("prefers the roles claim over groups and realm_access", () => {
    expect(
      extractRealmRoles({
        groups: ["hypershell-users"],
        realm_access: { roles: ["platform:admin"] },
        roles: ["hypershell-admins"],
      }),
    ).toEqual(["hypershell-admins"]);
  });

  it("falls back to realm_access.roles when roles and groups are absent", () => {
    expect(
      extractRealmRoles({
        realm_access: { roles: ["platform:admin", "hypershell-users"] },
      }),
    ).toEqual(["platform:admin", "hypershell-users"]);
  });

  it("ignores non-string role entries", () => {
    expect(
      extractRealmRoles({
        roles: ["hypershell-admins", 42, null],
      }),
    ).toEqual(["hypershell-admins"]);
  });
});
