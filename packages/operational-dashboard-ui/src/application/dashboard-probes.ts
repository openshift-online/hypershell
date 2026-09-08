import type {
  DomainProbe,
  DomainProbePublisher,
} from "@openshift-online/hypershell-domain-probes";

export type DashboardWorkflowAction = "get-operational-metrics";
export type DashboardLayoutAction = "persist-layout-template";
export type DashboardProbeAction =
  DashboardWorkflowAction | DashboardLayoutAction;

export type DashboardProbeOutcome =
  "started" | "succeeded" | "failed" | "cancelled";

export type DashboardProbeName =
  | "dashboard.workflow.started"
  | "dashboard.workflow.completed"
  | "dashboard.layout.template.invalid"
  | "dashboard.layout.template.persistence-failed";

export type DashboardProbe = DomainProbe<
  DashboardProbeName,
  1,
  {
    readonly action: DashboardProbeAction;
    readonly outcome: DashboardProbeOutcome;
  }
>;

export type DashboardProbePublisher = DomainProbePublisher<DashboardProbe>;

export const noopDashboardProbePublisher: DashboardProbePublisher = {
  publish() {
    return undefined;
  },
};
