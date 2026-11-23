import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";

import type { PlatformPolicyDialog } from "../features/platform-policies/platform-policy-summary";
import { PlatformPoliciesPage } from "../features/platform-policies/platform-policies-page";
import type { PlatformPolicyWriteFailure } from "../features/platform-policies/platform-policy-summary";
import { RequirePlatformPolicyAdmin } from "../features/platform-policies/require-platform-policy-admin";
import {
  usePlatformPolicies,
  usePlatformPolicySettings,
  usePlatformPolicyWrites,
} from "../features/platform-policies/use-platform-policies";
import { createPageMeta } from "../lib/page-meta";
import {
  platformPolicyDetailPath,
  platformPolicyEditPath,
  routePaths,
} from "../../shared/routes";

export const meta = createPageMeta(
  "app.platformPolicies.page.title",
  "app.platformPolicies.page.description",
);

function PlatformPoliciesRouteContent() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const policiesQuery = usePlatformPolicies();
  const settingsQuery = usePlatformPolicySettings();
  const [dialog, setDialog] = useState<PlatformPolicyDialog>();
  const [writeFailure, setWriteFailure] =
    useState<PlatformPolicyWriteFailure>();
  const writes = usePlatformPolicyWrites({
    globalMode: settingsQuery.data?.mode ?? "audit",
    onSettled: (_dialog, failure) => {
      setDialog(undefined);
      setWriteFailure(failure);
    },
  });
  const updateParam = (name: string, value: string, fallback: string) => {
    setSearchParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        if (value === fallback) {
          next.delete(name);
        } else {
          next.set(name, value);
        }
        return next;
      },
      { replace: true },
    );
  };

  return (
    <PlatformPoliciesPage
      dialog={dialog}
      filterText={searchParams.get("q") ?? ""}
      isError={policiesQuery.isError || settingsQuery.isError}
      isLoading={policiesQuery.isPending || settingsQuery.isPending}
      isWriting={writes.isWriting}
      links={{
        createHref: routePaths.adminPlatformPolicyNew,
        detailHref: platformPolicyDetailPath,
        editHref: platformPolicyEditPath,
        gatewaysHref: (policyId) =>
          `${platformPolicyDetailPath(policyId)}?tab=gateways`,
        onNavigate: (href) => {
          void navigate(href);
        },
      }}
      onConfirmDialog={(confirmed) => {
        setWriteFailure(undefined);
        writes.write(confirmed);
      }}
      onDialogChange={setDialog}
      onDismissWriteFailure={() => {
        setWriteFailure(undefined);
      }}
      onFilterTextChange={(text) => {
        updateParam("q", text, "");
      }}
      onRetry={() => {
        void policiesQuery.refetch();
        void settingsQuery.refetch();
      }}
      onSortDirectionChange={(direction) => {
        updateParam("order", direction, "asc");
      }}
      policies={policiesQuery.data ?? []}
      settings={settingsQuery.data}
      sortDirection={searchParams.get("order") === "desc" ? "desc" : "asc"}
      writeFailure={writeFailure}
    />
  );
}

export default function PlatformPoliciesRoute() {
  return (
    <RequirePlatformPolicyAdmin>
      <PlatformPoliciesRouteContent />
    </RequirePlatformPolicyAdmin>
  );
}
