import { describe, expect, it } from "vitest";

import { gatePhaseBadge, healthBadge, syncBadge } from "./status";

describe("gatePhaseBadge", () => {
  it("maps passing phases to success", () => {
    expect(gatePhaseBadge("success").tone).toBe("success");
    expect(gatePhaseBadge("passed").tone).toBe("success");
    // Argo/Tekton spell a passed analysis "Succeeded"/"Successful".
    expect(gatePhaseBadge("Succeeded").tone).toBe("success");
    expect(gatePhaseBadge("Successful").tone).toBe("success");
  });

  it("maps failing phases to danger, never brand red", () => {
    expect(gatePhaseBadge("failed").tone).toBe("danger");
  });

  it("maps in-flight phases to info (blue), not warning", () => {
    expect(gatePhaseBadge("pending").tone).toBe("info");
    expect(gatePhaseBadge("running").tone).toBe("info");
  });

  it("never infers success from an unrecognized or absent phase", () => {
    expect(gatePhaseBadge("banana").tone).toBe("unknown");
    expect(gatePhaseBadge(null).tone).toBe("unknown");
    expect(gatePhaseBadge(undefined).tone).toBe("unknown");
  });
});

describe("healthBadge", () => {
  it("maps Argo health strings to tones", () => {
    expect(healthBadge("Healthy").tone).toBe("success");
    expect(healthBadge("Progressing").tone).toBe("warning");
    expect(healthBadge("Degraded").tone).toBe("danger");
    expect(healthBadge("Missing").tone).toBe("danger");
  });

  it("resolves unknown health to neutral", () => {
    expect(healthBadge("").tone).toBe("unknown");
    expect(healthBadge(null).tone).toBe("unknown");
  });
});

describe("syncBadge", () => {
  it("maps sync states to tones", () => {
    expect(syncBadge("Synced").tone).toBe("success");
    expect(syncBadge("OutOfSync").tone).toBe("warning");
  });

  it("resolves unknown sync to neutral", () => {
    expect(syncBadge(undefined).tone).toBe("unknown");
  });
});
