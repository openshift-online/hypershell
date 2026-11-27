import { describe, expect, it } from "vitest";

import {
  hsctlCommandPathOf,
  knownHsctlCommandPaths,
} from "./hsctl-command-paths";

describe("hsctlCommandPathOf", () => {
  // Verifies: WEB-TERM-13.01
  it("names a command by its path only, without arguments or flags", () => {
    expect(hsctlCommandPathOf(["delete", "gateway", "3f2a", "--yes"])).toBe(
      "delete gateway",
    );
    expect(hsctlCommandPathOf(["list", "gateways", "--search", "x"])).toBe(
      "list gateways",
    );
  });

  // Verifies: WEB-TERM-13
  it("maps aliases to the canonical command path", () => {
    expect(hsctlCommandPathOf(["list", "gateway"])).toBe("list gateways");
    expect(hsctlCommandPathOf(["get", "gateways", "g1"])).toBe("get gateway");
    expect(hsctlCommandPathOf(["list", "service-accounts"])).toBe(
      "list serviceAccounts",
    );
    expect(hsctlCommandPathOf(["get", "repositories", "r"])).toBe(
      "get repository",
    );
    expect(hsctlCommandPathOf(["revoke", "service-account", "s"])).toBe(
      "revoke serviceAccount",
    );
  });

  // Verifies: WEB-TERM-13
  it("uses the bare verb, root, leaf commands, or other", () => {
    expect(hsctlCommandPathOf([])).toBe("hsctl");
    expect(hsctlCommandPathOf(["list"])).toBe("list");
    expect(hsctlCommandPathOf(["list", "--help"])).toBe("list");
    expect(hsctlCommandPathOf(["version"])).toBe("version");
    expect(hsctlCommandPathOf(["login", "--token", "secret"])).toBe("login");
    expect(hsctlCommandPathOf(["list", "secret-material-3f2a"])).toBe("other");
    expect(hsctlCommandPathOf(["--help"])).toBe("other");
    expect(hsctlCommandPathOf(["frobnicate"])).toBe("other");
  });

  // Verifies: WEB-TERM-13
  it("keeps the path list bounded and unique", () => {
    expect(new Set(knownHsctlCommandPaths).size).toBe(
      knownHsctlCommandPaths.length,
    );
    expect(knownHsctlCommandPaths).toContain("list gateways");
    expect(knownHsctlCommandPaths).toContain("other");
    expect(knownHsctlCommandPaths.length).toBeLessThan(150);
  });
});
