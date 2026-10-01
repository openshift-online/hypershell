// The map's detail panel. Selecting a node opens a tabbed record (Details /
// Bundle / Links): Details carries a gateway donut + legend and a field grid;
// Bundle lists the pull requests in the instance's deployed release; Links holds
// the deep links. Selecting a gate shows its flow + badge; selecting a release
// bundle shows its facts, where it is deployed, and the PRs it contains. All copy
// is translated; all values are opaque server data.

import {
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Label,
  type LabelProps,
  List,
  ListItem,
  Tab,
  Tabs,
  TabTitleText,
  Title,
} from "@patternfly/react-core";
import ExternalLinkAltIcon from "@patternfly/react-icons/dist/esm/icons/external-link-alt-icon";
import LongArrowAltRightIcon from "@patternfly/react-icons/dist/esm/icons/long-arrow-alt-right-icon";
import TimesIcon from "@patternfly/react-icons/dist/esm/icons/times-icon";
import { useState } from "react";
import type { MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import {
  bundleList,
  deployedFor,
  seedForBundle,
} from "../../../domain/map/bundles";
import { identiName } from "../../../domain/map/identiname";
import type { MapModel, MapNode } from "../../../domain/map/model";
import type {
  PromotionState,
  PullRequest,
  ReleaseBundle,
} from "../../../domain/promotion";
import { gatePhaseBadge, healthBadge, syncBadge } from "../../../domain/status";
import { messages } from "../../../messages";
import { StatusLabel } from "../status-label";
import { TEXT_COLOR } from "./colors";
import { GatewayDonut } from "./gateway-donut";
import { Identicon } from "./identicon";

/**
 * The release-bundle identicon, inline. A release bundle is always identified by
 * its identicon (+ identiname) wherever it appears, matching the node cards and
 * freight bar, so a bundle is recognisable at a glance across every view.
 */
function BundleIdenticon({
  seed,
  size = 28,
}: {
  seed: string;
  size?: number;
}): React.ReactElement {
  return (
    <svg
      width={size}
      height={size}
      aria-hidden="true"
      style={{ flexShrink: 0, display: "block" }}
    >
      <Identicon seed={seed} x={0} y={0} size={size} />
    </svg>
  );
}

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

const PROMO_LABEL: Record<
  PromotionState,
  { readonly msg: MessageDescriptor; readonly status: LabelProps["status"] }
> = {
  "up-to-date": { msg: messages.promoUpToDate, status: "success" },
  promoting: { msg: messages.promoPromoting, status: "warning" },
  behind: { msg: messages.promoBehind, status: undefined },
};

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
        icon={<ExternalLinkAltIcon />}
        iconPosition="end"
      >
        {label}
      </Button>
    </FlexItem>
  );
}

/** The pull requests in a release bundle. Each row links to the PR (new tab);
 *  the author shows on hover. Empty/absent -> a graceful empty state. */
function PrList({ prs }: { prs: readonly PullRequest[] }): React.ReactElement {
  const intl = useIntl();
  if (prs.length === 0) {
    return (
      <Content component="small">
        <FormattedMessage {...messages.bundleNoPrs} />
      </Content>
    );
  }
  return (
    <List isPlain>
      {prs.map((pr) => {
        const label = `#${String(pr.number)} ${pr.title}`;
        return (
          <ListItem key={pr.number}>
            <Button
              component="a"
              href={pr.url}
              target="_blank"
              rel="noreferrer noopener"
              variant="link"
              isInline
              title={
                pr.author
                  ? intl.formatMessage(messages.prAuthoredBy, {
                      author: pr.author,
                    })
                  : undefined
              }
            >
              {label}
            </Button>
          </ListItem>
        );
      })}
    </List>
  );
}

/** Section heading + optional PR-count summary, then the PR list for `bundle`.
 *  When `seed` is given, the bundle's identicon sits beside the heading so the
 *  tab's bundle is identifiable on its own. */
function BundleContents({
  bundle,
  seed,
}: {
  bundle: ReleaseBundle | undefined;
  seed?: string;
}): React.ReactElement {
  const prs = bundle?.prs ?? [];
  return (
    <>
      <Flex
        alignItems={{ default: "alignItemsCenter" }}
        spaceItems={{ default: "spaceItemsSm" }}
      >
        {seed ? (
          <FlexItem>
            <BundleIdenticon seed={seed} size={20} />
          </FlexItem>
        ) : null}
        <FlexItem>
          <Title headingLevel="h4" size="md">
            <FormattedMessage {...messages.sectionInBundle} />
            {prs.length > 0 ? (
              <Content component="small" className="pf-v6-u-ml-sm">
                <FormattedMessage
                  {...messages.bundlePrSummary}
                  values={{ count: prs.length }}
                />
              </Content>
            ) : null}
          </Title>
        </FlexItem>
      </Flex>
      <PrList prs={prs} />
    </>
  );
}

function GatewaySummary({ node }: { node: MapNode }): React.ReactElement {
  const intl = useIntl();
  const g = node.gateways;
  const running = g.running ?? 0;
  const provisioning = g.provisioning ?? 0;
  const failed = g.failed ?? 0;
  const label = intl.formatMessage(messages.detailGatewayBreakdown, {
    total: node.gatewaysTotal,
    running,
    provisioning,
    failed,
  });
  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsLg" }}
    >
      <FlexItem>
        <svg
          width={120}
          height={120}
          viewBox="0 0 120 120"
          role="img"
          aria-label={label}
          style={{ color: TEXT_COLOR }}
        >
          <GatewayDonut counts={g} cx={60} cy={60} radius={50} />
        </svg>
      </FlexItem>
      <FlexItem>
        <DescriptionList isCompact isHorizontal>
          <Row term={<FormattedMessage {...messages.legendRunning} />}>
            {running}
          </Row>
          <Row term={<FormattedMessage {...messages.legendProvisioning} />}>
            {provisioning}
          </Row>
          <Row term={<FormattedMessage {...messages.legendFailed} />}>
            {failed}
          </Row>
        </DescriptionList>
      </FlexItem>
    </Flex>
  );
}

function NodeFields({ node }: { node: MapNode }): React.ReactElement {
  const promo = PROMO_LABEL[node.state];
  const releaseLabel = node.version
    ? `${node.version} · ${identiName(node.seed)}`
    : null;
  return (
    <DescriptionList isCompact isHorizontal>
      {node.cluster ? (
        <Row term={<FormattedMessage {...messages.detailCluster} />}>
          {node.cluster}
        </Row>
      ) : null}
      <Row term={<FormattedMessage {...messages.columnProvider} />}>
        {node.provider ?? <FormattedMessage {...messages.valueNone} />}
      </Row>
      <Row term={<FormattedMessage {...messages.columnEnvironment} />}>
        {node.columnKey}
      </Row>
      <Row term={<FormattedMessage {...messages.columnRole} />}>
        {node.role ?? <FormattedMessage {...messages.valueNone} />}
      </Row>
      <Row term={<FormattedMessage {...messages.detailSync} />}>
        <StatusLabel badge={syncBadge(node.argoSync)} />
      </Row>
      <Row term={<FormattedMessage {...messages.columnHealth} />}>
        {node.links.argo ? (
          <Button
            component="a"
            href={node.links.argo}
            target="_blank"
            rel="noreferrer noopener"
            variant="link"
            isInline
            icon={<ExternalLinkAltIcon />}
            iconPosition="end"
          >
            <StatusLabel badge={healthBadge(node.argoHealth)} />
          </Button>
        ) : (
          <StatusLabel badge={healthBadge(node.argoHealth)} />
        )}
      </Row>
      <Row term={<FormattedMessage {...messages.detailPromotion} />}>
        <Label status={promo.status} variant="outline" isCompact>
          <FormattedMessage {...promo.msg} />
        </Label>
      </Row>
      <Row term={<FormattedMessage {...messages.columnRelease} />}>
        {releaseLabel ? (
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            spaceItems={{ default: "spaceItemsSm" }}
          >
            <FlexItem>
              <BundleIdenticon seed={node.seed} size={20} />
            </FlexItem>
            <FlexItem>{releaseLabel}</FlexItem>
          </Flex>
        ) : (
          <FormattedMessage {...messages.valueNone} />
        )}
      </Row>
      {node.proposedVersion ? (
        <Row term={<FormattedMessage {...messages.detailProposed} />}>
          {node.proposedVersion}
        </Row>
      ) : null}
      {node.digest ? (
        <Row term={<FormattedMessage {...messages.detailDigest} />}>
          <code>{node.digest}</code>
        </Row>
      ) : null}
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
    </DescriptionList>
  );
}

function NodeLinks({ node }: { node: MapNode }): React.ReactElement {
  return (
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
  );
}

function NodeDetails({
  node,
  releaseByDigest,
}: {
  node: MapNode;
  releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
}): React.ReactElement {
  const [activeKey, setActiveKey] = useState<string | number>("details");
  const bundle = node.digest ? releaseByDigest[node.digest] : undefined;
  return (
    <Tabs
      activeKey={activeKey}
      onSelect={(_event, key) => {
        setActiveKey(key);
      }}
      isBox
    >
      <Tab
        eventKey="details"
        title={
          <TabTitleText>
            <FormattedMessage {...messages.tabDetails} />
          </TabTitleText>
        }
      >
        <div className="pf-v6-u-mt-md">
          <GatewaySummary node={node} />
          <div className="pf-v6-u-mt-md">
            <NodeFields node={node} />
          </div>
        </div>
      </Tab>
      <Tab
        eventKey="bundle"
        title={
          <TabTitleText>
            <FormattedMessage {...messages.tabBundle} />
          </TabTitleText>
        }
      >
        <div className="pf-v6-u-mt-md">
          <BundleContents bundle={bundle} seed={node.seed} />
        </div>
      </Tab>
      <Tab
        eventKey="links"
        title={
          <TabTitleText>
            <FormattedMessage {...messages.tabLinks} />
          </TabTitleText>
        }
      >
        <div className="pf-v6-u-mt-md">
          <NodeLinks node={node} />
        </div>
      </Tab>
    </Tabs>
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
    <>
      <DescriptionList isCompact isHorizontal>
        <Row term={<FormattedMessage {...messages.detailRelease} />}>
          {bundle.version}
        </Row>
        <Row term={<FormattedMessage {...messages.detailAlias} />}>
          {identiName(seed)}
        </Row>
        {bundle.digest ? (
          <Row term={<FormattedMessage {...messages.detailDigest} />}>
            <code>{bundle.digest}</code>
          </Row>
        ) : null}
        {bundle.date ? (
          <Row term={<FormattedMessage {...messages.detailDate} />}>
            {bundle.date}
          </Row>
        ) : null}
      </DescriptionList>

      <Title headingLevel="h4" size="md" className="pf-v6-u-mt-md">
        <FormattedMessage {...messages.sectionDeployedOn} />
      </Title>
      {deployed.length > 0 ? (
        <Flex spaceItems={{ default: "spaceItemsXs" }}>
          {deployed.map((n) => (
            <FlexItem key={n.id}>
              <Label variant="outline" isCompact>
                {n.id}
              </Label>
            </FlexItem>
          ))}
        </Flex>
      ) : (
        <Content component="small">
          <FormattedMessage {...messages.valueNone} />
        </Content>
      )}

      <div className="pf-v6-u-mt-md">
        <BundleContents bundle={bundle} />
      </div>
    </>
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
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            spaceItems={{ default: "spaceItemsSm" }}
          >
            {selection.kind === "bundle" ? (
              <FlexItem>
                <BundleIdenticon seed={selection.id} />
              </FlexItem>
            ) : null}
            <FlexItem>
              <Title headingLevel="h3" size="md">
                {title}
              </Title>
            </FlexItem>
          </Flex>
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
      {selection.kind === "node" && node ? (
        <NodeDetails node={node} releaseByDigest={releaseByDigest} />
      ) : null}
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
