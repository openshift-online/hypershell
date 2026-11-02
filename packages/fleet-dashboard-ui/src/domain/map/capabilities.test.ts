import { describe, expect, it } from "vitest";

import {
  capabilityParts,
  capabilityValue,
  orderedCapabilities,
} from "./capabilities";

describe("capabilityValue", () => {
  // FLT-CAP-02.01
  it("returns the value of the first capability in a namespace", () => {
    expect(
      capabilityValue(["runtime/kata", "region/us-east-1"], "runtime"),
    ).toBe("kata");
  });

  it("returns null for a namespace not present", () => {
    expect(capabilityValue(["runtime/kata"], "provider")).toBeNull();
  });

  it("returns the first match when a namespace repeats", () => {
    expect(capabilityValue(["network/vpn", "network/public"], "network")).toBe(
      "vpn",
    );
  });
});

describe("capabilityParts", () => {
  // FLT-CAP-02.02
  it("splits on the first slash only, keeping internal characters", () => {
    expect(capabilityParts("runtime/kata-qemu.v2")).toEqual({
      namespace: "runtime",
      value: "kata-qemu.v2",
    });
  });

  it("keeps later slashes in the value", () => {
    expect(capabilityParts("region/eu/de")).toEqual({
      namespace: "region",
      value: "eu/de",
    });
  });

  it("treats a slash-less capability as a bare value", () => {
    expect(capabilityParts("standalone")).toEqual({
      namespace: "",
      value: "standalone",
    });
  });
});

describe("orderedCapabilities", () => {
  // FLT-CAP-02.03
  it("orders lead namespaces first and keeps unknown namespaces", () => {
    expect(
      orderedCapabilities([
        "zone/a",
        "region/eu-de",
        "runtime/runc",
        "provider/ibm",
      ]),
    ).toEqual(["provider/ibm", "runtime/runc", "region/eu-de", "zone/a"]);
  });

  it("sorts multiple unknown namespaces alphabetically after the lead set", () => {
    expect(orderedCapabilities(["zone/a", "alpha/x", "provider/aws"])).toEqual([
      "provider/aws",
      "alpha/x",
      "zone/a",
    ]);
  });

  it("returns a new array and does not mutate the input", () => {
    const input = ["region/eu-de", "provider/ibm"];
    const out = orderedCapabilities(input);
    expect(out).not.toBe(input);
    expect(input).toEqual(["region/eu-de", "provider/ibm"]);
  });

  it("is empty for an empty set", () => {
    expect(orderedCapabilities([])).toEqual([]);
  });
});
