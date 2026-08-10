import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { GatewayStatus } from "./gateway-status";

describe("GatewayStatus", () => {
  it("renders success as an icon and plain text without a label chip", () => {
    const { container } = render(<GatewayStatus status="Running" />);

    expect(screen.getByText("Running")).toBeTruthy();
    expect(container.querySelector(".pf-v6-c-label")).toBeNull();
    expect(
      container.querySelector(".pf-v6-c-icon__content.pf-m-success svg"),
    ).toBeTruthy();
  });

  it("renders transitional status with an inline progress indicator", () => {
    const { container } = render(<GatewayStatus status="Updating" />);

    expect(screen.getByText("Updating")).toBeTruthy();
    expect(container.querySelector(".pf-v6-c-spinner")).toBeTruthy();
  });
});
