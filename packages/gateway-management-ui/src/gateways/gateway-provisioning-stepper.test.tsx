import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { ProvisioningCondition } from "../application/gateway-types";

import { GatewayProvisioningStepper } from "./gateway-provisioning-stepper";

function renderStepper(
  conditions: readonly ProvisioningCondition[],
  phase?: string,
  consoleReady?: boolean,
) {
  return render(
    <IntlProvider locale="en">
      <GatewayProvisioningStepper
        conditions={conditions}
        consoleReady={consoleReady}
        phase={phase}
      />
    </IntlProvider>,
  );
}

const allPending: ProvisioningCondition[] = [
  { conditionStatus: "Pending", message: "", type: "EnvironmentReady" },
  { conditionStatus: "Pending", message: "", type: "DatabaseReady" },
  { conditionStatus: "Pending", message: "", type: "GatewayDeployed" },
  { conditionStatus: "Pending", message: "", type: "GatewayHealthy" },
];

const inProgress: ProvisioningCondition[] = [
  { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
  { conditionStatus: "InProgress", message: "", type: "DatabaseReady" },
  { conditionStatus: "Pending", message: "", type: "GatewayDeployed" },
  { conditionStatus: "Pending", message: "", type: "GatewayHealthy" },
];

const allComplete: ProvisioningCondition[] = [
  { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
  { conditionStatus: "Complete", message: "", type: "DatabaseReady" },
  { conditionStatus: "Complete", message: "", type: "GatewayDeployed" },
  { conditionStatus: "Complete", message: "", type: "GatewayHealthy" },
];

const withIdp: ProvisioningCondition[] = [
  { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
  { conditionStatus: "Complete", message: "", type: "DatabaseReady" },
  {
    conditionStatus: "InProgress",
    message: "",
    type: "IdentityProviderReady",
  },
  { conditionStatus: "Pending", message: "", type: "GatewayDeployed" },
  { conditionStatus: "Pending", message: "", type: "GatewayHealthy" },
];

const dbFailed: ProvisioningCondition[] = [
  { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
  {
    conditionStatus: "Failed",
    message:
      "Database provisioning failed - the database service is unavailable",
    type: "DatabaseReady",
  },
  { conditionStatus: "Pending", message: "", type: "GatewayDeployed" },
  { conditionStatus: "Pending", message: "", type: "GatewayHealthy" },
];

const degradedHealth: ProvisioningCondition[] = [
  { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
  { conditionStatus: "Complete", message: "", type: "DatabaseReady" },
  { conditionStatus: "Complete", message: "", type: "GatewayDeployed" },
  {
    conditionStatus: "Failed",
    message:
      "Gateway health check timed out - the gateway workload is not yet ready",
    type: "GatewayHealthy",
  },
];

describe("GatewayProvisioningStepper", () => {
  it("renders default pending steps when conditions list is empty", () => {
    renderStepper([]);

    expect(screen.getByText("Preparing environment")).toBeTruthy();
    expect(screen.getByText("Provisioning database")).toBeTruthy();
    expect(screen.getByText("Deploying gateway")).toBeTruthy();
    expect(screen.getByText("Verifying gateway health")).toBeTruthy();
    expect(screen.getByText("Starting console")).toBeTruthy();
    expect(screen.getByText("Provisioned")).toBeTruthy();
  });

  it("renders step labels from conditions", () => {
    renderStepper(allPending, "Provisioning");

    expect(screen.getByText("Preparing environment")).toBeTruthy();
    expect(screen.getByText("Provisioning database")).toBeTruthy();
    expect(screen.getByText("Deploying gateway")).toBeTruthy();
    expect(screen.getByText("Verifying gateway health")).toBeTruthy();
  });

  it("renders identity provider step when present", () => {
    renderStepper(withIdp, "Provisioning");

    expect(screen.getByText("Configuring identity provider")).toBeTruthy();
  });

  it("marks in-progress step as current", () => {
    const { container } = renderStepper(inProgress, "Provisioning");

    const currentStep = container.querySelector(
      ".pf-v6-c-progress-stepper__step.pf-m-current",
    );
    expect(currentStep).toBeTruthy();
    expect(currentStep?.textContent).toContain("Provisioning database");
  });

  it("renders all-complete steps with success variant", () => {
    const { container } = renderStepper(allComplete, "Running", true);

    const successSteps = container.querySelectorAll(
      ".pf-v6-c-progress-stepper__step.pf-m-success",
    );
    expect(successSteps.length).toBe(6);
  });

  it("renders failed step with danger variant and shows message", () => {
    renderStepper(dbFailed, "Failed");

    expect(
      screen.getByText(
        "Database provisioning failed - the database service is unavailable",
      ),
    ).toBeTruthy();
  });

  it("renders degraded health step with warning variant", () => {
    const { container } = renderStepper(degradedHealth, "Degraded");

    const warningStep = container.querySelector(
      ".pf-v6-c-progress-stepper__step.pf-m-warning",
    );
    expect(warningStep).toBeTruthy();
    expect(warningStep?.textContent).toContain("Verifying gateway health");
  });

  it("renders health step as danger when phase is Failed", () => {
    const failedHealth: ProvisioningCondition[] = [
      { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
      { conditionStatus: "Complete", message: "", type: "DatabaseReady" },
      { conditionStatus: "Complete", message: "", type: "GatewayDeployed" },
      {
        conditionStatus: "Failed",
        message:
          "Gateway health check timed out - the gateway workload is not yet ready",
        type: "GatewayHealthy",
      },
    ];
    const { container } = renderStepper(failedHealth, "Failed");

    const dangerStep = container.querySelector(
      ".pf-v6-c-progress-stepper__step.pf-m-danger",
    );
    expect(dangerStep).toBeTruthy();
    expect(dangerStep?.textContent).toContain("Verifying gateway health");
  });

  it("appends ConsoleReady and Provisioned steps to server conditions", () => {
    renderStepper(allPending, "Provisioning");

    expect(screen.getByText("Starting console")).toBeTruthy();
    expect(screen.getByText("Provisioned")).toBeTruthy();
  });

  it("marks console step as complete when consoleReady is true", () => {
    const { container } = renderStepper(allComplete, "Running", true);

    const steps = container.querySelectorAll(".pf-v6-c-progress-stepper__step");
    const consoleStep = Array.from(steps).find((s) =>
      s.textContent.includes("Console ready"),
    );
    expect(consoleStep).toBeDefined();
    if (consoleStep) {
      expect(consoleStep.classList.contains("pf-m-success")).toBe(true);
    }
  });
});
