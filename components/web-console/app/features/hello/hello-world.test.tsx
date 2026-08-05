import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";

import { englishMessages } from "../../i18n/catalog";
import { HelloWorld } from "./hello-world";

describe("HelloWorld", () => {
  it("presents an accessible page purpose and localized content", () => {
    render(
      <IntlProvider locale="en" messages={englishMessages}>
        <HelloWorld />
      </IntlProvider>,
    );

    expect(
      screen.getByRole("heading", { level: 1, name: "Hello world" }),
    ).toBeTruthy();
    expect(
      screen.getByText("The HyperShell web console is ready for development."),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Skip to content" })
        .getAttribute("href"),
    ).toBe("#main-content");
  });
});
