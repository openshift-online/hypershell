import type { PolicyFindingOutcome } from "@openshift-online/hypershell-gateway-management-ui";
import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";

import { platformPolicyFailureKind } from "../application/platform-policies/platform-policy-operations";
import {
  PlatformPolicyDetailPage,
  toPlatformPolicyDetailTab,
} from "../features/platform-policies/platform-policy-detail-page";
import type {
  PlatformPolicyDialog,
  PlatformPolicyWriteFailure,
} from "../features/platform-policies/platform-policy-summary";
import { RequirePlatformPolicyAdmin } from "../features/platform-policies/require-platform-policy-admin";
import {
  usePlatformPolicy,
  usePlatformPolicyFindings,
  usePlatformPolicySettings,
  usePlatformPolicyWrites,
} from "../features/platform-policies/use-platform-policies";
import { createPageMeta } from "../lib/page-meta";
import {
  gatewayDetailPath,
  platformPolicyEditPath,
  routePaths,
} from "../../shared/routes";

export const meta = createPageMeta(
  "app.platformPolicies.page.detailTitle",
  "app.platformPolicies.page.detailDescription",
);

const findingsPageSizes = new Set([10, 20, 50, 100]);

function toOutcome(value: string | null): PolicyFindingOutcome | undefined {
  return value === "would-reject" || value === "would-add-defaults"
    ? value
    : undefined;
}

function PlatformPolicyRouteContent() {
  const { policyId = "" } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const policyQuery = usePlatformPolicy(policyId);
  const settingsQuery = usePlatformPolicySettings();
  const gatewayId = searchParams.get("gateway") ?? undefined;
  const outcome = toOutcome(searchParams.get("outcome"));
  const page = Math.max(1, Number(searchParams.get("page")) || 1);
  const requestedSize = Number(searchParams.get("size"));
  const size = findingsPageSizes.has(requestedSize) ? requestedSize : 20;
  const findingsQuery = usePlatformPolicyFindings(policyId, {
    ...(gatewayId ? { gatewayId } : {}),
    ...(outcome ? { outcome } : {}),
    page,
    size,
  });
  const [dialog, setDialog] = useState<PlatformPolicyDialog>();
  const [writeFailure, setWriteFailure] =
    useState<PlatformPolicyWriteFailure>();
  const writes = usePlatformPolicyWrites({
    globalMode: settingsQuery.data?.mode ?? "audit",
    onSettled: (settled, failure) => {
      setDialog(undefined);
      setWriteFailure(failure);
      if (!failure && settled.kind === "delete") {
        void navigate(routePaths.adminPlatformPolicies);
      }
    },
  });
  const updateParams = (values: Record<string, string | undefined>) => {
    setSearchParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        for (const [name, value] of Object.entries(values)) {
          if (value === undefined || value === "") {
            next.delete(name);
          } else {
            next.set(name, value);
          }
        }
        return next;
      },
      { replace: true },
    );
  };

  return (
    <PlatformPolicyDetailPage
      activeTab={toPlatformPolicyDetailTab(searchParams.get("tab"))}
      dialog={dialog}
      findings={{
        isError: findingsQuery.isError,
        isLoading: findingsQuery.isPending,
        items: findingsQuery.data?.items ?? [],
        page,
        size,
        total: findingsQuery.data?.total ?? 0,
      }}
      findingsFilter={{
        ...(gatewayId ? { gatewayId } : {}),
        ...(outcome ? { outcome } : {}),
      }}
      isError={policyQuery.isError}
      isLoading={policyQuery.isPending}
      isNotFound={
        policyQuery.isError &&
        platformPolicyFailureKind(policyQuery.error) === "not-found"
      }
      isWriting={writes.isWriting}
      links={{
        editHref: platformPolicyEditPath,
        gatewayHref: gatewayDetailPath,
        listHref: routePaths.adminPlatformPolicies,
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
      onFindingsFilterChange={(filter) => {
        updateParams({
          gateway: filter.gatewayId,
          outcome: filter.outcome,
          page: undefined,
        });
      }}
      onFindingsPageChange={(nextPage, nextSize) => {
        updateParams({
          page: nextPage === 1 ? undefined : String(nextPage),
          size: nextSize === 20 ? undefined : String(nextSize),
        });
      }}
      onRetry={() => {
        void policyQuery.refetch();
      }}
      onTabChange={(tab) => {
        setSearchParams((previous) => {
          const next = new URLSearchParams(previous);
          if (tab === "overview") {
            next.delete("tab");
          } else {
            next.set("tab", tab);
          }
          return next;
        });
      }}
      policy={policyQuery.data}
      settings={settingsQuery.data}
      writeFailure={writeFailure}
    />
  );
}

export default function PlatformPolicyRoute() {
  return (
    <RequirePlatformPolicyAdmin>
      <PlatformPolicyRouteContent />
    </RequirePlatformPolicyAdmin>
  );
}
