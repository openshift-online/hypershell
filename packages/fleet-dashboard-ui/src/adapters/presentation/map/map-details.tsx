// The map's detail panel: when the user selects a node, gate or release bundle it
// shows that entity's full record (identity, promotion state, Argo health/sync,
// gateway breakdown, RED metrics and deep links for a node; flow + badge for a
// gate; release facts + deployment list for a bundle). All copy is translated; all
// values are opaque server data.

import {
  Button,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Title,
} from "@patternfly/react-core";
import LongArrowAltRightIcon from "@patternfly/react-icons/dist/esm/icons/long-arrow-alt-right-icon";
import TimesIcon from "@patternfly/react-icons/dist/esm/icons/times-icon";
import { FormattedMessage, useIntl } from "react-intl";

import {
  bundleList,
  deployedFor,
  seedForBundle,
} from "../../../domain/map/bundles";
import { identiName } from "../../../domain/map/identiname";
import type { MapModel, MapNode } from "../../../domain/map/model";
import type { ReleaseBundle } from "../../../domain/promotion";
import { gatePhaseBadge, healthBadge, syncBadge } from "../../../domain/status";
import { messages } from "../../../messages";
import { StatusLabel } from "../status-label";

/** What the map currently has selected. `id` is a node id, gate id or bundle seed. */
export interface MapSelection {
  readonly kind: "node" | "gate" | "bundle";
  readonly id: string;
}

export interface MapDetailsProps {
  readonly model: MapModel;
  readonly releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
  readonly selection: MapSelection;
  readonly onClose: () => void;
}

function Row({
  term,
  children,
}: {
  term: React.ReactNode;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <DescriptionListGroup>
      <DescriptionListTerm>{term}</DescriptionListTerm>
      <DescriptionListDescription>{children}</DescriptionListDescription>
    </DescriptionListGroup>
  );
}

function Link({
  href,
  label,
}: {
  href: string | null;
  label: React.ReactNode;
}): React.ReactElement | null {
  if (!href) {
    return null;
  }
  return (
    <FlexItem>
      <Button
        component="a"
        href={href}
        target="_blank"
        rel="noreferrer noopener"
        variant="link"
        isInline
      >
        {label}
      </Button>
    </FlexItem>
  );
}

function NodeDetails({ node }: { node: MapNode }): React.ReactElement {
  const g = node.gateways;
  return (
    <DescriptionList isCompact>
      <Row term={<FormattedMessage {...messages.columnRole} />}>
        {node.role ?? <FormattedMessage {...messages.valueNone} />}
      </Row>
      <Row term={<FormattedMessage {...messages.columnProvider} />}>
        {node.provider ?? <FormattedMessage {...messages.valueNone} />}
      </Row>
      {node.cluster ? (
        <Row term={<FormattedMessage {...messages.detailCluster} />}>
          {node.cluster}
        </Row>
      ) : null}
      <Row term={<FormattedMessage {...messages.columnRelease} />}>
        {node.version ?? <FormattedMessage {...messages.valueNone} />}
      </Row>
      {node.digest ? (
        <Row term={<FormattedMessage {...messages.detailDigest} />}>
          {node.digest}
        </Row>
      ) : null}
      {node.proposedVersion ? (
        <Row term={<FormattedMessage {...messages.detailProposed} />}>
          {node.proposedVersion}
        </Row>
      ) : null}
      <Row term={<FormattedMessage {...messages.columnHealth} />}>
        <Flex spaceItems={{ default: "spaceItemsXs" }}>
          <FlexItem>
            <StatusLabel badge={healthBadge(node.argoHealth)} />
          </FlexItem>
          <FlexItem>
            <StatusLabel badge={syncBadge(node.argoSync)} />
          </FlexItem>
        </Flex>
      </Row>
      <Row term={<FormattedMessage {...messages.columnGates} />}>
        <FormattedMessage
          {...messages.detailGatewayBreakdown}
          values={{
            total: node.gatewaysTotal,
            running: g.running ?? 0,
            provisioning: g.provisioning ?? 0,
            failed: g.failed ?? 0,
          }}
        />
      </Row>
      {node.managedClusters !== null ? (
        <Row term={<FormattedMessage {...messages.detailClusters} />}>
          {node.managedClusters}
        </Row>
      ) : null}
      {node.users !== null ? (
        <Row term={<FormattedMessage {...messages.detailUsers} />}>
          {node.users}
        </Row>
      ) : null}
      <Row term={<FormattedMessage {...messages.detailMetrics} />}>
        <FormattedMessage
          {...messages.detailMetricTriple}
          values={{
            rpc: node.metrics.rpc.p95Ms,
            reconcile: node.metrics.reconcile.p95Ms,
            bff: node.metrics.bff.p95Ms,
          }}
        />
      </Row>
      {node.links.console ||
      node.links.argo ||
      node.links.pr ||
      node.links.analysis ? (
        <Row term={<FormattedMessage {...messages.detailLinks} />}>
          <Flex spaceItems={{ default: "spaceItemsSm" }}>
            <Link
              href={node.links.console}
              label={<FormattedMessage {...messages.linkConsole} />}
            />
            <Link
              href={node.links.argo}
              label={<FormattedMessage {...messages.linkArgo} />}
            />
            <Link
              href={node.links.pr}
              label={<FormattedMessage {...messages.linkPr} />}
            />
            <Link
              href={node.links.analysis}
              label={<FormattedMessage {...messages.linkAnalysis} />}
            />
          </Flex>
        </Row>
      ) : null}
    </DescriptionList>
  );
}

function GateDetails({
  gateId,
  model,
}: {
  gateId: string;
  model: MapModel;
}): React.ReactElement | null {
  const gate = model.gates.find((x) => x.id === gateId);
  if (!gate) {
    return null;
  }
  return (
    <DescriptionList isCompact>
      <Row term={<FormattedMessage {...messages.detailFlow} />}>
        <Flex
          spaceItems={{ default: "spaceItemsXs" }}
          alignItems={{ default: "alignItemsCenter" }}
        >
          <FlexItem>{gate.fromColumnKey}</FlexItem>
          <FlexItem>
            <LongArrowAltRightIcon />
          </FlexItem>
          <FlexItem>{gate.toColumnKey}</FlexItem>
        </Flex>
      </Row>
      <Row term={<FormattedMessage {...messages.columnGates} />}>
        <StatusLabel badge={gate.badge} />
      </Row>
      <Row term={<FormattedMessage {...messages.detailPromoting} />}>
        {gate.promoting ? (
          <StatusLabel badge={gatePhaseBadge("pending")} />
        ) : (
          <FormattedMessage {...messages.valueNone} />
        )}
      </Row>
    </DescriptionList>
  );
}

function BundleDetails({
  seed,
  model,
  releaseByDigest,
}: {
  seed: string;
  model: MapModel;
  releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
}): React.ReactElement | null {
  const bundle = bundleList(releaseByDigest).find(
    (b) => seedForBundle(b) === seed,
  );
  if (!bundle) {
    return null;
  }
  const deployed = deployedFor(bundle, model.nodes);
  return (
    <DescriptionList isCompact>
      <Row term={<FormattedMessage {...messages.columnRelease} />}>
        {bundle.version}
      </Row>
      <Row term={<FormattedMessage {...messages.detailAlias} />}>
        {identiName(seed)}
      </Row>
      {bundle.digest ? (
        <Row term={<FormattedMessage {...messages.detailDigest} />}>
          {bundle.digest}
        </Row>
      ) : null}
      {bundle.date ? (
        <Row term={<FormattedMessage {...messages.detailDate} />}>
          {bundle.date}
        </Row>
      ) : null}
      <Row term={<FormattedMessage {...messages.detailDeployments} />}>
        {deployed.length > 0 ? (
          deployed.map((n) => n.id).join(", ")
        ) : (
          <FormattedMessage {...messages.valueNone} />
        )}
      </Row>
    </DescriptionList>
  );
}

export function MapDetails({
  model,
  releaseByDigest,
  selection,
  onClose,
}: MapDetailsProps): React.ReactElement {
  const intl = useIntl();
  const node =
    selection.kind === "node"
      ? model.nodes.find((n) => n.id === selection.id)
      : undefined;
  const title =
    selection.kind === "node" ? (node?.id ?? selection.id) : selection.id;

  return (
    <div>
      <Flex
        justifyContent={{ default: "justifyContentSpaceBetween" }}
        alignItems={{ default: "alignItemsCenter" }}
      >
        <FlexItem>
          <Title headingLevel="h3" size="md">
            {title}
          </Title>
        </FlexItem>
        <FlexItem>
          <Button
            variant="plain"
            aria-label={intl.formatMessage(messages.drawerClose)}
            onClick={onClose}
            icon={<TimesIcon />}
          />
        </FlexItem>
      </Flex>
      {selection.kind === "node" && node ? <NodeDetails node={node} /> : null}
      {selection.kind === "gate" ? (
        <GateDetails gateId={selection.id} model={model} />
      ) : null}
      {selection.kind === "bundle" ? (
        <BundleDetails
          seed={selection.id}
          model={model}
          releaseByDigest={releaseByDigest}
        />
      ) : null}
    </div>
  );
}
