import { describe, expect, it, vi } from "vitest";
import { GatewayOperationError } from "@openshift-online/hypershell-gateway-management-ui";

import {
  createQuickCreateGateway,
  createQuickCreateNameGenerator,
  isValidGatewayName,
  quickCreatePlacement,
} from "../../application/homepage/quick-create";

describe("quick create gateway", () => {
  it("generates a valid two-word name with entropy", () => {
    const random = {
      nextHex: () => "ab12",
      nextInt: (max: number) => max - 1,
    };
    const generateName = createQuickCreateNameGenerator(random);
    const name = generateName();

    expect(name).toBe("vivid-zenith-ab12");
    expect(isValidGatewayName(name)).toBe(true);
  });

  it("submits public IBM placement and excludes Kata", async () => {
    const provisionGateway = vi.fn().mockResolvedValue({ id: "gateway-1" });

    await createQuickCreateGateway({
      generateName: () => "swift-gateway-ab12",
      knownNames: new Set(),
      provisionGateway,
    });

    expect(provisionGateway).toHaveBeenCalledWith("swift-gateway-ab12");
  });

  it("avoids a known name and retries conflicts a bounded number of times", async () => {
    const provisionGateway = vi.fn().mockResolvedValue({ id: "gateway-2" });
    const generated = ["swift-gateway-ab12", "swift-gateway-cd34"];
    let index = 0;

    await createQuickCreateGateway({
      generateName: () => generated[index++] ?? "unused",
      knownNames: new Set(["swift-gateway-ab12"]),
      provisionGateway,
    });

    expect(provisionGateway).toHaveBeenCalledTimes(1);
    expect(provisionGateway).toHaveBeenCalledWith("swift-gateway-cd34");
  });

  it("bounds conflict retries", async () => {
    const provisionGateway = vi
      .fn()
      .mockRejectedValue(new GatewayOperationError("conflict"));
    let suffix = 0;

    await expect(
      createQuickCreateGateway({
        generateName: () =>
          `swift-gateway-${String(suffix++).padStart(4, "0")}`,
        knownNames: new Set(),
        provisionGateway,
      }),
    ).rejects.toThrow("Could not generate a unique gateway name");

    expect(provisionGateway).toHaveBeenCalledTimes(5);
  });

  it("keeps the intended placement selectors", () => {
    expect(quickCreatePlacement.selector).toEqual([
      "network/public",
      "provider/ibm",
    ]);
    expect(quickCreatePlacement.excludedSelector).toEqual(["runtime/kata"]);
  });
});
