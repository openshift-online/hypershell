import { describe, expect, it, vi } from "vitest";

import {
  evaluateGithubOrgGate,
  githubIdentityAllowed,
  parseUsernameAllowlist,
} from "../src/github-org-gate.js";

describe("parseUsernameAllowlist", () => {
  it("splits, trims, and lowercases comma-separated usernames", () => {
    expect(parseUsernameAllowlist(" Alice,Bob , ,CAROL ")).toEqual([
      "alice",
      "bob",
      "carol",
    ]);
  });

  it("returns an empty list when the value is missing or blank", () => {
    expect(parseUsernameAllowlist(undefined)).toEqual([]);
    expect(parseUsernameAllowlist("")).toEqual([]);
    expect(parseUsernameAllowlist("  , ")).toEqual([]);
  });
});

describe("githubIdentityAllowed", () => {
  it("admits an organization member", () => {
    expect(
      githubIdentityAllowed({
        allowlist: [],
        orgGate: "openshift-online",
        orgLogins: ["kubernetes", "openshift-online"],
        username: "alice",
      }),
    ).toBe(true);
  });

  it("compares organization membership case-insensitively", () => {
    expect(
      githubIdentityAllowed({
        allowlist: [],
        orgGate: "OpenShift-Online",
        orgLogins: ["OpenShift-Online"],
        username: "alice",
      }),
    ).toBe(true);
  });

  it("admits an allowlisted username who is not an org member", () => {
    expect(
      githubIdentityAllowed({
        allowlist: ["outside-contributor"],
        orgGate: "openshift-online",
        orgLogins: ["acme"],
        username: "Outside-Contributor",
      }),
    ).toBe(true);
  });

  it("denies a user who is neither an org member nor allowlisted", () => {
    expect(
      githubIdentityAllowed({
        allowlist: ["someone-else"],
        orgGate: "openshift-online",
        orgLogins: ["acme"],
        username: "alice",
      }),
    ).toBe(false);
  });

  it("denies when the org list is empty and the username is not allowlisted", () => {
    expect(
      githubIdentityAllowed({
        allowlist: [],
        orgGate: "openshift-online",
        orgLogins: [],
        username: "alice",
      }),
    ).toBe(false);
  });
});

function hrefOf(input: Parameters<typeof fetch>[0]): string {
  if (typeof input === "string") {
    return input;
  }
  if (input instanceof URL) {
    return input.href;
  }
  return input.url;
}

describe("evaluateGithubOrgGate", () => {
  const baseInput = {
    accessToken: "kc-access-token",
    githubApiOrigin: "https://api.github.com",
    oidcIssuer: "https://sso.example.test/realms/hypershell",
    orgGate: "openshift-online",
    username: "alice",
  };

  it("skips GitHub when the username is allowlisted", async () => {
    const fetchImpl = vi.fn();
    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "alice",
        fetchImpl,
      }),
    ).resolves.toBe(true);
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("admits a public org member without reading the broker token", async () => {
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.includes("/public_members/")) {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(true);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
    const firstCall = fetchImpl.mock.calls[0]?.[0];
    expect(firstCall).toBeDefined();
    if (firstCall === undefined) {
      throw new Error("expected a public-members fetch");
    }
    expect(hrefOf(firstCall)).toContain(
      "/orgs/openshift-online/public_members/alice",
    );
  });

  it("admits a private member via /user/memberships when /user/orgs omits the org", async () => {
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.includes("/public_members/")) {
        return Promise.resolve(new Response("not found", { status: 404 }));
      }
      if (href.endsWith("/broker/github/token")) {
        return Promise.resolve(
          new Response(JSON.stringify({ access_token: "gho-test" }), {
            headers: { "content-type": "application/json" },
            status: 200,
          }),
        );
      }
      if (href.includes("/user/memberships/orgs/")) {
        return Promise.resolve(
          new Response(JSON.stringify({ state: "active" }), {
            headers: { "content-type": "application/json" },
            status: 200,
          }),
        );
      }
      if (href.includes("/user/orgs")) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            headers: { "content-type": "application/json" },
            status: 200,
          }),
        );
      }
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(true);
    expect(
      fetchImpl.mock.calls.some((call) =>
        hrefOf(call[0]).includes("/user/orgs"),
      ),
    ).toBe(false);
  });

  it("denies when GitHub returns 403 because the OAuth App is not org-approved", async () => {
    const onLookupError = vi.fn();
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.includes("/public_members/")) {
        return Promise.resolve(new Response("not found", { status: 404 }));
      }
      if (href.endsWith("/broker/github/token")) {
        return Promise.resolve(
          new Response(JSON.stringify({ access_token: "gho-test" }), {
            headers: { "content-type": "application/json" },
            status: 200,
          }),
        );
      }
      if (href.includes("/user/memberships/orgs/")) {
        return Promise.resolve(
          new Response("OAuth App access restrictions", { status: 403 }),
        );
      }
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
        onLookupError,
      }),
    ).resolves.toBe(false);
    expect(onLookupError).toHaveBeenCalledTimes(1);
    expect(String(onLookupError.mock.calls[0]?.[0])).toMatch(
      /OAuth App may not be approved by openshift-online/u,
    );
  });

  it("admits an org member after reading the brokered GitHub token", async () => {
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.endsWith("/broker/github/token")) {
        return Promise.resolve(
          new Response(JSON.stringify({ access_token: "gho-test" }), {
            headers: { "content-type": "application/json" },
            status: 200,
          }),
        );
      }
      if (href.includes("/user/orgs")) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ login: "openshift-online" }, { login: "other" }]),
            {
              headers: { "content-type": "application/json" },
              status: 200,
            },
          ),
        );
      }
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(true);
    expect(fetchImpl).toHaveBeenCalledTimes(4);
  });

  it("denies when GitHub org lookup fails", async () => {
    const fetchImpl = vi.fn(() =>
      Promise.resolve(new Response("nope", { status: 401 })),
    );
    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(false);
  });

  it("denies a non-member when the org list does not include the gate", async () => {
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.endsWith("/broker/github/token")) {
        return Promise.resolve(
          new Response("access_token=gho-test&token_type=bearer", {
            headers: { "content-type": "application/x-www-form-urlencoded" },
            status: 200,
          }),
        );
      }
      return Promise.resolve(
        new Response(JSON.stringify([{ login: "acme" }]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
      );
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(false);
  });

  it("admits an org member when Keycloak labels a form-encoded broker token as JSON", async () => {
    // Keycloak 26 retrieveToken always sets Content-Type application/json even
    // when GitHub stored access_token=...&token_type=bearer (the default
    // GitHub token response unless githubJsonFormat is enabled).
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.endsWith("/broker/github/token")) {
        return Promise.resolve(
          new Response(
            "access_token=gho-test&token_type=bearer&scope=read%3Aorg",
            {
              headers: { "content-type": "application/json" },
              status: 200,
            },
          ),
        );
      }
      if (href.includes("/user/orgs")) {
        return Promise.resolve(
          new Response(JSON.stringify([{ login: "openshift-online" }]), {
            headers: { "content-type": "application/json" },
            status: 200,
          }),
        );
      }
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(true);
  });

  it("denies when the broker token body is neither JSON nor form-encoded", async () => {
    const fetchImpl = vi.fn((input: Parameters<typeof fetch>[0]) => {
      const href = hrefOf(input);
      if (href.endsWith("/broker/github/token")) {
        return Promise.resolve(
          new Response("gho-not-a-structured-token", {
            headers: { "content-type": "text/plain" },
            status: 200,
          }),
        );
      }
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    await expect(
      evaluateGithubOrgGate({
        ...baseInput,
        allowlistRaw: "",
        fetchImpl,
      }),
    ).resolves.toBe(false);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });
});
