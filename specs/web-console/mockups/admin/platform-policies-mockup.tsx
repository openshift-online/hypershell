import { MockupShell } from "../shell/mockup-template";
import { PlatformPoliciesPageSurface } from "./platform-policies";

/*
 * Full-shell design-review wrapper. It lives apart from the page surface so
 * the surface stays shell-free for the Storybook parity gate
 * (skills/build/ui-build-gate).
 */
export function PlatformPoliciesMockup(
  props: Parameters<typeof PlatformPoliciesPageSurface>[0],
) {
  return (
    <MockupShell activeSection="platform-policies" showAdministration>
      <PlatformPoliciesPageSurface {...props} />
    </MockupShell>
  );
}
