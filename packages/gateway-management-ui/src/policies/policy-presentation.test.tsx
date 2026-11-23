import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createIntl, IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import {
  formatPolicyTime,
  policyFindingDetail,
  PolicyModeExplanationPopover,
  policyOperationText,
  PolicyRulesTable,
} from "./policy-presentation";

const intl = createIntl({ locale: "en" });

describe("policy presentation", () => {
  it("formats operations, times, and findings without values", () => {
    expect(policyOperationText(intl, "CreateSandbox")).toBe("Create sandbox");
    expect(policyOperationText(intl, "ResumeSandbox")).toBe("ResumeSandbox");
    expect(formatPolicyTime(intl, "not-a-date")).toBe("not-a-date");
    expect(
      policyFindingDetail(intl, {
        gatewayId: "g",
        id: "f",
        occurredAt: "",
        operation: "CreateSandbox",
        outcome: "would-reject",
        policyNames: ["a", "b"],
        principalSubject: "u",
        requestSummary: { image: 1 },
      }),
    ).toBe("No resource values recorded (a and b)");
  });

  it("explains enforcement in a popover", async () => {
    const user = userEvent.setup();
    render(
      <IntlProvider locale="en">
        <PolicyModeExplanationPopover globalMode="enforce" />
      </IntlProvider>,
    );
    await user.click(
      screen.getByRole("button", { name: "How enforcement works" }),
    );
    expect(
      await screen.findByText("Platform enforcement is Enforced."),
    ).toBeTruthy();
  });

  it("shows fleet defaults in a policy's rules table", () => {
    render(
      <IntlProvider locale="en">
        <PolicyRulesTable
          defaults={{ memoryLimit: "4Gi" }}
          name="cap"
          rules={[
            {
              bounds: [
                {
                  display: "32Gi",
                  field: "memoryLimitBytes",
                  label: "Memory limit",
                  op: ">",
                  text: "Memory limit: at most 32Gi",
                  value: 34359738368,
                },
              ],
              cedar: "forbid",
              recognized: true,
              summary: ["Memory limit: at most 32Gi"],
            },
          ]}
        />
      </IntlProvider>,
    );
    expect(screen.getByText("4Gi")).toBeTruthy();
    expect(screen.getByText("32Gi")).toBeTruthy();
    expect(screen.getByText("No rule")).toBeTruthy();
  });
});
