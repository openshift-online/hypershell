import { CostDashboardPageSurface } from "./cost-dashboard";
import { MockupShell } from "../shell/mockup-template";

export function CostDashboardMockup() {
  return (
    <MockupShell activeSection="dashboard">
      <CostDashboardPageSurface />
    </MockupShell>
  );
}
