import { render } from "@testing-library/react";
import { IntlProvider } from "react-intl";

import { ReleaseTime, selectRelativeUnit } from "./release-time";

function renderTime(
  iso: string | null,
  mode: "full" | "relative",
): HTMLElement {
  const { container } = render(
    <IntlProvider locale="en" defaultLocale="en">
      <ReleaseTime iso={iso} mode={mode} />
    </IntlProvider>,
  );
  return container;
}

describe("selectRelativeUnit", () => {
  const now = 1_000_000_000_000;
  it("uses seconds under a minute", () => {
    expect(selectRelativeUnit(now - 30_000, now)).toEqual({
      value: -30,
      unit: "second",
    });
  });
  it("uses minutes under an hour", () => {
    expect(selectRelativeUnit(now - 5 * 60_000, now)).toEqual({
      value: -5,
      unit: "minute",
    });
  });
  it("rounds 90 minutes up to 2 hours", () => {
    expect(selectRelativeUnit(now - 90 * 60_000, now)).toEqual({
      value: -2,
      unit: "hour",
    });
  });
  it("uses days under a month", () => {
    expect(selectRelativeUnit(now - 3 * 86_400_000, now)).toEqual({
      value: -3,
      unit: "day",
    });
  });
  it("handles future instants as positive values", () => {
    expect(selectRelativeUnit(now + 2 * 3_600_000, now)).toEqual({
      value: 2,
      unit: "hour",
    });
  });
});

describe("ReleaseTime", () => {
  it("renders nothing for a null instant", () => {
    const { container } = render(
      <IntlProvider locale="en" defaultLocale="en">
        <ReleaseTime iso={null} />
      </IntlProvider>,
    );
    expect(container.textContent).toBe("");
  });

  it("falls back to the raw string when unparseable (no tooltip trigger)", () => {
    const container = renderTime("not-a-date", "full");
    expect(container.textContent).toBe("not-a-date");
  });

  it("shows a relative string for a fixed past instant", () => {
    // A fixed past instant; the visible text should contain a relative phrase.
    const container = renderTime("2020-01-01T00:00:00Z", "relative");
    // Visible relative text (English short style, e.g. "... ago").
    expect(container.textContent).toMatch(/ago/);
  });

  it("full mode includes an absolute local time alongside the relative string", () => {
    const container = renderTime("2020-06-15T12:00:00Z", "full");
    // medium date style in en includes the year; relative is in parentheses.
    expect(container.textContent).toMatch(/2020.*\(.*ago\)/);
  });
});
