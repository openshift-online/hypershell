import { Breadcrumb, BreadcrumbItem } from "@patternfly/react-core";

import { MockupShell } from "../shell/mockup-template";
import {
  PlatformPolicyDetailPageSurface,
  type PlatformPolicyDetailTab,
} from "./platform-policy-detail";

/*
 * Full-shell design-review wrapper. It lives apart from the page surface so
 * the surface stays shell-free for the Storybook parity gate
 * (skills/build/ui-build-gate).
 */
export function PlatformPolicyDetailMockup({
  activeTab,
}: {
  activeTab?: PlatformPolicyDetailTab;
}) {
  return (
    <MockupShell
      activeSection="platform-policies"
      breadcrumb={
        <Breadcrumb>
          <BreadcrumbItem to="/admin/platform-policies">
            Platform policies
          </BreadcrumbItem>
          <BreadcrumbItem isActive>sandbox-resource-baseline</BreadcrumbItem>
        </Breadcrumb>
      }
      showAdministration
    >
      <PlatformPolicyDetailPageSurface activeTab={activeTab} />
    </MockupShell>
  );
}
