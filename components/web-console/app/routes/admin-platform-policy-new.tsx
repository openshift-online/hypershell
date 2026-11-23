import type { ReactElement } from "react";
import { useNavigate } from "react-router";

import { platformPolicyOperations } from "../composition/platform-policy-composition";
import { PlatformPolicyFormPage } from "../features/platform-policies/platform-policy-form-page";
import { RequirePlatformPolicyAdmin } from "../features/platform-policies/require-platform-policy-admin";
import { usePlatformPolicyEditor } from "../features/platform-policies/use-platform-policy-editor";
import { createPageMeta } from "../lib/page-meta";
import { platformPolicyDetailPath, routePaths } from "../../shared/routes";

export const meta = createPageMeta(
  "app.platformPolicies.editor.pageTitleCreate",
  "app.platformPolicies.editor.pageMetaDescription",
);

function CreatePlatformPolicyContent() {
  const navigate = useNavigate();
  const editor = usePlatformPolicyEditor({
    onSaved: (policy) => {
      void navigate(platformPolicyDetailPath(policy.id));
    },
    operations: platformPolicyOperations,
  });
  return (
    <PlatformPolicyFormPage
      {...editor.view}
      cancelHref={routePaths.adminPlatformPolicies}
      onCancel={() => {
        void navigate(routePaths.adminPlatformPolicies);
      }}
    />
  );
}

export default function CreatePlatformPolicyRoute(): ReactElement {
  return (
    <RequirePlatformPolicyAdmin>
      <CreatePlatformPolicyContent />
    </RequirePlatformPolicyAdmin>
  );
}
