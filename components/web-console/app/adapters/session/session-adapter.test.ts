import { describe, expect, it, vi } from "vitest";

import { createSessionAdapter } from "./session-adapter";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

describe("session adapter", () => {
  it("maps an authenticated session resource to camelCase identity", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(
        jsonResponse({
          authenticated: true,
          expires_at: 1_723_401_600,
          roles: ["hypershell-admins", 42],
          user: {
            email: "admin@example.test",
            name: "Admin User",
            preferred_username: "admin",
            sub: "user-123",
          },
        }),
      );

    const session =
      await createSessionAdapter(fetchImplementation).getSession();

    expect(fetchImplementation).toHaveBeenCalledWith(
      "/auth/session",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    expect(session).toEqual({
      authenticated: true,
      expiresAt: 1_723_401_600,
      roles: ["hypershell-admins"],
      user: {
        email: "admin@example.test",
        name: "Admin User",
        preferredUsername: "admin",
        sub: "user-123",
      },
    });
  });

  it("treats an unauthenticated resource as no session", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(jsonResponse({ authenticated: false }));

    const session =
      await createSessionAdapter(fetchImplementation).getSession();

    expect(session).toEqual({ authenticated: false, roles: [] });
  });

  it("treats an absent endpoint (no-auth mode) as no session", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(new Response("not found", { status: 404 }));

    const session =
      await createSessionAdapter(fetchImplementation).getSession();

    expect(session).toEqual({ authenticated: false, roles: [] });
  });

  it("treats a network failure as no session", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockRejectedValue(new Error("network down"));

    const session =
      await createSessionAdapter(fetchImplementation).getSession();

    expect(session).toEqual({ authenticated: false, roles: [] });
  });
});
