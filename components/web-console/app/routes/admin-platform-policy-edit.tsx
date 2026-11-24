import type { ReactElement } from "react";
import {
  Button,
  EmptyState,
  EmptyStateActions,
  EmptyStateFooter,
  EmptyStateVariant,
  PageSection,
  Spinner,
} from "@patternfly/react-core";
import { useIntl } from "react-intl";
import { useNavigate, useParams } from "react-router";

import { platformPolicyOperations } from "../composition/platform-policy-composition";
import { editorMessages } from "../features/platform-policies/platform-policy-editor-messages";
import { PlatformPolicyFormPage } from "../features/platform-policies/platform-policy-form-page";
import { RequirePlatformPolicyAdmin } from "../features/platform-policies/require-platform-policy-admin";
import { usePlatformPolicyEditor } from "../features/platform-policies/use-platform-policy-editor";
import { createPageMeta } from "../lib/page-meta";
import { platformPolicyDetailPath } from "../../shared/routes";

export const meta = createPageMeta(
  "app.platformPolicies.editor.pageTitleEdit",
  "app.platformPolicies.editor.pageMetaDescription",
);

function EditPlatformPolicyContent({ policyId }: { policyId: string }) {
  const intl = useIntl();
  const navigate = useNavigate();
  const detailHref = platformPolicyDetailPath(policyId);
  const editor = usePlatformPolicyEditor({
    onSaved: (policy) => {
      void navigate(platformPolicyDetailPath(policy.id));
    },
    operations: platformPolicyOperations,
    policyId,
  });

  if (editor.load?.isPending) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <Spinner
          aria-label={intl.formatMessage(editorMessages.loadingPolicy)}
        />
      </PageSection>
    );
  }
  if (editor.load?.isError) {
    const { refetch } = editor.load;
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <EmptyState
          headingLevel="h1"
          titleText={intl.formatMessage(editorMessages.loadFailedTitle)}
          variant={EmptyStateVariant.full}
        >
          <EmptyStateFooter>
            <EmptyStateActions>
              <Button onClick={refetch} variant="primary">
                {intl.formatMessage(editorMessages.retry)}
              </Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      </PageSection>
    );
  }
  return (
    <PlatformPolicyFormPage
      {...editor.view}
      cancelHref={detailHref}
      onCancel={() => {
        void navigate(detailHref);
      }}
    />
  );
}

export default function EditPlatformPolicyRoute(): ReactElement {
  const { policyId = "" } = useParams();
  return (
    <RequirePlatformPolicyAdmin>
      <EditPlatformPolicyContent key={policyId} policyId={policyId} />
    </RequirePlatformPolicyAdmin>
  );
}
