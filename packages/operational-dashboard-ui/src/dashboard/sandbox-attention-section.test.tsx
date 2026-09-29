import { render, screen } from "@testing-library/react";
import { createIntl, createIntlCache, IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import {
  hasSandboxAttentionData,
  hasSandboxAttentionRequired,
  SandboxAttentionSection,
} from "./sandbox-attention-section";

const intlMessages = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);
const intl = createIntl(
  { locale: "en", messages: intlMessages },
  createIntlCache(),
);

function renderAttention(metric: OperationalMetric) {
  return render(
    <IntlProvider locale={intl.locale} messages={intl.messages}>
      <SandboxAttentionSection metric={metric} />
    </IntlProvider>,
  );
}

describe("hasSandboxAttentionData", () => {
  it("is true when any attention field is present including zero", () => {
    const metric: OperationalMetric = {
      id: "provisioned-sandboxes",
      orphanedSandboxes: 0,
      value: "214",
    };
    expect(hasSandboxAttentionData(metric)).toBe(true);
  });

  it("is false when all attention fields are absent", () => {
    const metric: OperationalMetric = {
      id: "provisioned-sandboxes",
      value: "214",
    };
    expect(hasSandboxAttentionData(metric)).toBe(false);
  });
});

describe("hasSandboxAttentionRequired", () => {
  it("is false when all present attention counts are zero", () => {
    const metric: OperationalMetric = {
      expiringSandboxes: 0,
      id: "provisioned-sandboxes",
      idleSandboxes: 0,
      orphanedSandboxes: 0,
      value: "214",
    };
    expect(hasSandboxAttentionRequired(metric)).toBe(false);
  });

  it("is true when any attention count is greater than zero", () => {
    const metric: OperationalMetric = {
      expiringSandboxes: 0,
      id: "provisioned-sandboxes",
      idleSandboxes: 0,
      orphanedSandboxes: 3,
      value: "214",
    };
    expect(hasSandboxAttentionRequired(metric)).toBe(true);
  });

  it("is false when all attention fields are absent", () => {
    const metric: OperationalMetric = {
      id: "provisioned-sandboxes",
      value: "214",
    };
    expect(hasSandboxAttentionRequired(metric)).toBe(false);
  });
});

describe("SandboxAttentionSection", () => {
  it("renders non-zero attention labels with count text", () => {
    renderAttention({
      expiringSandboxes: 12,
      id: "provisioned-sandboxes",
      idleSandboxes: 7,
      orphanedSandboxes: 3,
      value: "214",
    });

    expect(screen.getByText("Attention required")).toBeTruthy();
    expect(screen.getByText("12 Expiring soon")).toBeTruthy();
    expect(screen.getByText("3 Orphaned")).toBeTruthy();
    expect(screen.getByText("7 Idle")).toBeTruthy();
  });

  it("hides the section when all present attention counts are zero", () => {
    const { container } = renderAttention({
      expiringSandboxes: 0,
      id: "provisioned-sandboxes",
      idleSandboxes: 0,
      orphanedSandboxes: 0,
      value: "214",
    });

    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("Attention required")).toBeNull();
  });

  it("shows only non-zero chips when counts are mixed", () => {
    renderAttention({
      expiringSandboxes: 0,
      id: "provisioned-sandboxes",
      idleSandboxes: 0,
      orphanedSandboxes: 3,
      value: "214",
    });

    expect(screen.getByText("3 Orphaned")).toBeTruthy();
    expect(screen.queryByText(/Expiring soon/)).toBeNull();
    expect(screen.queryByText(/Idle/)).toBeNull();
  });

  it("shows unavailable when attention fields are absent", () => {
    renderAttention({
      id: "provisioned-sandboxes",
      value: "214",
    });

    expect(screen.getByText("Attention required")).toBeTruthy();
    expect(screen.getByText("Attention counts unavailable")).toBeTruthy();
  });
});
