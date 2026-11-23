import { Breadcrumb, BreadcrumbItem } from "@patternfly/react-core";

import { MockupShell } from "../shell/mockup-template";
import { PlatformPolicyFormPageSurface } from "./platform-policy-form";

/*
 * Full-shell design-review wrapper. It lives apart from the page surface so
 * the surface stays shell-free for the Storybook parity gate
 * (skills/build/ui-build-gate).
 */
export function PlatformPolicyFormMockup(
  props: Parameters<typeof PlatformPolicyFormPageSurface>[0],
) {
  return (
    <MockupShell
      activeSection="platform-policies"
      breadcrumb={
        <Breadcrumb>
          <BreadcrumbItem to="/admin/platform-policies">
            Platform policies
          </BreadcrumbItem>
          <BreadcrumbItem isActive>Create platform policy</BreadcrumbItem>
        </Breadcrumb>
      }
      showAdministration
    >
      <PlatformPolicyFormPageSurface {...props} />
    </MockupShell>
  );
}
