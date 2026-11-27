import { MyCostDashboardPageSurface } from "./my-cost-dashboard";
import { MockupShell } from "../shell/mockup-template";

export function MyCostDashboardMockup() {
  return (
    <MockupShell activeSection="dashboard">
      <MyCostDashboardPageSurface />
    </MockupShell>
  );
}
