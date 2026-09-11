import { createIntl, createIntlCache } from "react-intl";
import { describe, expect, it } from "vitest";

import { messages } from "../messages";
import {
  formatOperationalMetricDisplayValue,
  isDisplayableOperationalMetricValue,
} from "./operational-metric-display";

const intl = createIntl(
  {
    locale: "en",
    messages: Object.fromEntries(
      Object.values(messages).map((message) => [
        message.id,
        message.defaultMessage,
      ]),
    ),
  },
  createIntlCache(),
);

describe("isDisplayableOperationalMetricValue", () => {
  it("accepts finite decimal strings", () => {
    expect(isDisplayableOperationalMetricValue("0")).toBe(true);
    expect(isDisplayableOperationalMetricValue("42")).toBe(true);
    expect(isDisplayableOperationalMetricValue("5.25")).toBe(true);
  });

  it("rejects non-finite literals and coercions", () => {
    expect(isDisplayableOperationalMetricValue("NaN")).toBe(false);
    expect(isDisplayableOperationalMetricValue("Infinity")).toBe(false);
    expect(isDisplayableOperationalMetricValue("-Infinity")).toBe(false);
    expect(isDisplayableOperationalMetricValue("not-a-number")).toBe(false);
  });
});

describe("formatOperationalMetricDisplayValue", () => {
  it("returns the localized fallback for non-displayable values", () => {
    expect(formatOperationalMetricDisplayValue("NaN", intl)).toBe(
      "Metric could not be determined",
    );
  });

  it("returns the original value when displayable", () => {
    expect(formatOperationalMetricDisplayValue("12", intl)).toBe("12");
  });
});
