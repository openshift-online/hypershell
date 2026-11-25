import { describe, expect, it } from "vitest";

import { createTerminalSessionCheck } from "./terminal-session-check";

describe("terminal session check", () => {
  // Verifies: WEB-TERM-10.02
  it("reports an expired session only when sign-in is in use", async () => {
    const check = (session: { authEnabled: boolean; authenticated: boolean }) =>
      createTerminalSessionCheck({
        getSession: () => Promise.resolve({ ...session, roles: [] }),
      }).isSessionExpired();

    await expect(
      check({ authEnabled: true, authenticated: false }),
    ).resolves.toBe(true);
    await expect(
      check({ authEnabled: true, authenticated: true }),
    ).resolves.toBe(false);
    await expect(
      check({ authEnabled: false, authenticated: false }),
    ).resolves.toBe(false);
  });
});
