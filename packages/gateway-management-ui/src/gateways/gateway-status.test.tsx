import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { GatewayStatus } from "./gateway-status";
import styles from "./gateway-status.module.css";

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

  it("renders unknown statuses with the neutral status color", () => {
    const { container } = render(<GatewayStatus status="Future status" />);
    const unknownClassName = styles.unknown;

    expect(screen.getByText("Future status")).toBeTruthy();
    expect(unknownClassName).toBeDefined();
    if (!unknownClassName) {
      throw new Error("Unknown status style is unavailable");
    }
    expect(container.querySelector(`.${unknownClassName}`)).toBeTruthy();
  });
});
