// The map's detail panel. Selecting a node opens a tabbed record (Details /
// Bundle / Links): Details carries a gateway donut + legend and a field grid;
// Bundle lists the pull requests in the instance's deployed release; Links holds
// the deep links. Selecting a gate shows its flow + badge; selecting a release
// bundle shows its facts, where it is deployed, and the PRs it contains. All copy
// is translated; all values are opaque server data.

import {
  Badge,
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
  Spinner,
  Tab,
  Tabs,
  TabTitleText,
  Title,
  Tooltip,
  Truncate,
} from "@patternfly/react-core";
import ChartLineIcon from "@patternfly/react-icons/dist/esm/icons/chart-line-icon";
import ExternalLinkAltIcon from "@patternfly/react-icons/dist/esm/icons/external-link-alt-icon";
import InfoAltIcon from "@patternfly/react-icons/dist/esm/icons/info-alt-icon";
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
import { otherGateways } from "../../../domain/fleet";
import { shortDigest } from "../../../domain/map/digest";
import { identiName } from "../../../domain/map/identiname";
import type { MapModel, MapNode } from "../../../domain/map/model";
import type {
  PromotionState,
  PullRequest,
  ReleaseBundle,
} from "../../../domain/promotion";
import { healthBadge, syncBadge } from "../../../domain/status";
import { messages } from "../../../messages";
import { StatusLabel } from "../status-label";
import { GATEWAY_COLOR, TEXT_COLOR } from "./colors";
import { GatewayDonut } from "./gateway-donut";
import { Identicon } from "./identicon";
import styles from "./map-details.module.css";
import { MetricTiles } from "./metric-tiles";
import { ReleaseTime } from "./release-time";

/**
 * The release-bundle identicon, inline. A release bundle is always identified by
 * its identicon (+ identiname) wherever it appears, matching the node cards and
 * freight bar, so a bundle is recognisable at a glance across every view. When
 * `onSelect` is given, the identicon is clickable and opens that bundle's details.
 */
function BundleIdenticon({
  seed,
  size = 28,
  onSelect,
}: {
  seed: string;
  size?: number;
  onSelect?: (seed: string) => void;
}): React.ReactElement {
  const icon = (
    <svg
      width={size}
      height={size}
      aria-hidden="true"
      style={{ flexShrink: 0, display: "block" }}
    >
      <Identicon seed={seed} x={0} y={0} size={size} />
    </svg>
  );
  if (!onSelect) {
    return icon;
  }
  return (
    <span
      role="button"
      tabIndex={0}
      aria-label={identiName(seed)}
      onClick={() => {
        onSelect(seed);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(seed);
        }
      }}
      style={{
        cursor: "pointer",
        display: "inline-flex",
        borderRadius: 4,
      }}
    >
      {icon}
    </span>
  );
}

/**
 * A bundle as a whole clickable "chip": its identicon beside a label (version +
 * identiname), the ENTIRE chip opening the bundle's details - not just the small
 * identicon. Matches the prototype's compact "Promoting [icon] vX" chip. When no
 * `onSelect` is given it is inert (plain icon + label).
 */
function BundleChip({
  seed,
  label,
  onSelect,
}: {
  seed: string;
  label: React.ReactNode;
  onSelect?: (seed: string) => void;
}): React.ReactElement {
  const content = (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      spaceItems={{ default: "spaceItemsSm" }}
      flexWrap={{ default: "nowrap" }}
    >
      <FlexItem>
        <BundleIdenticon seed={seed} size={20} />
      </FlexItem>
      <FlexItem>{label}</FlexItem>
    </Flex>
  );
  if (!onSelect) {
    return content;
  }
  // A PatternFly inline link button: standard link colour + hover/focus underline
  // (PF link affordance), so a clickable bundle reference reads as a link. The
  // identicon is an SVG with its own fills, unaffected by the link text colour.
  return (
    <Button
      variant="link"
      isInline
      aria-label={identiName(seed)}
      onClick={() => {
        onSelect(seed);
      }}
    >
      {content}
    </Button>
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
  /** Opens a release bundle's details (from any identicon in the panel). */
  readonly onSelectBundle: (seed: string) => void;
  /** Selects an instance node (from a "Deployed on" chip in the bundle panel). */
  readonly onSelectNode: (id: string) => void;
}

const PROMO_LABEL: Record<
  PromotionState,
  { readonly msg: MessageDescriptor; readonly status: LabelProps["status"] }
> = {
  "up-to-date": { msg: messages.promoUpToDate, status: "success" },
  // Promoting is in-flight, not a problem: blue (info) with a live spinner, not an
  // amber warning.
  promoting: { msg: messages.promoPromoting, status: "info" },
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

/** A field label paired with a PatternFly info tooltip: hovering/focusing the
 *  small "i" shows contextual help (matching the prototype's per-field help). */
function TermWithInfo({
  label,
  info,
}: {
  label: React.ReactNode;
  info: MessageDescriptor;
}): React.ReactElement {
  const intl = useIntl();
  return (
    <span className={styles.termWithInfo}>
      {label}
      <Tooltip content={<FormattedMessage {...info} />}>
        <span
          className={styles.infoTip}
          role="button"
          tabIndex={0}
          aria-label={intl.formatMessage(messages.moreInfo)}
        >
          <InfoAltIcon />
        </span>
      </Tooltip>
    </span>
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

/** How many trailing title characters the middle-truncation pins on the right,
 *  so the end of the title stays legible when the row is clipped. */
const PR_TITLE_TAIL = 8;

/** The pull requests in a release bundle. Each row rides a single line that grows
 *  with the drawer: the "#<number>" is pinned and the title truncates in the
 *  middle (CSS flexbox, no JS). Hovering shows a card with the full title and the
 *  author; the row links to the PR in a new tab. Empty/absent -> a graceful
 *  empty state. */
function PrList({ prs }: { prs: readonly PullRequest[] }): React.ReactElement {
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
        const num = `#${String(pr.number)}`;
        // Split the title so the tail survives mid-truncation. Clamp so short
        // titles (shorter than the tail) don't split oddly.
        const tailLen = Math.min(PR_TITLE_TAIL, pr.title.length);
        const head = pr.title.slice(0, pr.title.length - tailLen);
        const tail = pr.title.slice(pr.title.length - tailLen);
        // The card separates the PR identity (number + full title) from its
        // authorship: the title is the headline line, the author sits beneath it
        // as a distinct, muted byline with the handle emphasised.
        const tipTitle = `${num} ${pr.title}`;
        const hover = (
          <span className={styles.prTip}>
            <span className={styles.prTipTitle}>{tipTitle}</span>
            {pr.author ? (
              <span className={styles.prTipAuthor}>
                <FormattedMessage
                  {...messages.prAuthoredBy}
                  values={{
                    author: (
                      <span className={styles.prTipHandle}>{pr.author}</span>
                    ),
                  }}
                />
              </span>
            ) : null}
          </span>
        );
        return (
          <ListItem key={pr.number}>
            <Tooltip content={hover} position="top-start">
              <a
                href={pr.url}
                target="_blank"
                rel="noreferrer noopener"
                className={styles.prLink}
              >
                <span className={styles.prNum}>{num}</span>
                <span className={styles.prTitle}>
                  <span className={styles.prHead}>{head}</span>
                  <span className={styles.prTail}>{tail}</span>
                </span>
              </a>
            </Tooltip>
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
  onSelectBundle,
}: {
  bundle: ReleaseBundle | undefined;
  seed?: string;
  onSelectBundle?: (seed: string) => void;
}): React.ReactElement {
  const intl = useIntl();
  const prs = bundle?.prs ?? [];
  // The heading reads as one band: identicon, "In this bundle" eyebrow, then the
  // PR count pinned to the right as a badge so the tab's size is legible at a glance.
  const prCountSummary = intl.formatMessage(messages.bundlePrSummary, {
    count: prs.length,
  });
  return (
    <>
      <div className={styles.bundleHead}>
        {seed ? (
          <BundleIdenticon seed={seed} size={20} onSelect={onSelectBundle} />
        ) : null}
        <h4 className={styles.sectionTitle}>
          <FormattedMessage {...messages.sectionInBundle} />
        </h4>
        {prs.length > 0 ? (
          // The badge's visible text is the bare count; aria-label gives it the
          // full "N pull requests" accessible name (title alone is not reliably
          // announced on a non-interactive element).
          <Badge
            isRead
            className={styles.bundleCount}
            aria-label={prCountSummary}
            title={prCountSummary}
          >
            {prs.length}
          </Badge>
        ) : null}
      </div>
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
  // Gateways in any phase beyond the three named rows, so the legend sums to the
  // donut's centre total instead of under-counting it.
  const other = otherGateways(g);
  const label = intl.formatMessage(messages.detailGatewayBreakdown, {
    total: node.gatewaysTotal,
    running,
    provisioning,
    failed,
  });
  // Donut on the left, a compact chart legend (swatch · label · count) on the right.
  // A chart legend - not a horizontal DescriptionList - so the three phase rows stay
  // tight beside the donut in the narrow drawer instead of wrapping below it.
  const legend: { color: string; term: MessageDescriptor; value: number }[] = [
    {
      color: GATEWAY_COLOR.running,
      term: messages.legendRunning,
      value: running,
    },
    {
      color: GATEWAY_COLOR.provisioning,
      term: messages.legendProvisioning,
      value: provisioning,
    },
    { color: GATEWAY_COLOR.failed, term: messages.legendFailed, value: failed },
  ];
  if (other > 0) {
    legend.push({
      color: GATEWAY_COLOR.idle,
      term: messages.legendOther,
      value: other,
    });
  }
  return (
    <div className={styles.gatewayBody}>
      <div className={styles.gatewayDonutCol}>
        <h4 className={styles.gatewayDonutTitle}>
          <FormattedMessage {...messages.sectionGateways} />
        </h4>
        <svg
          className={styles.gatewayDonut}
          width={96}
          height={96}
          viewBox="0 0 96 96"
          role="img"
          aria-label={label}
          style={{ color: TEXT_COLOR }}
        >
          <GatewayDonut counts={g} cx={48} cy={48} radius={44} />
        </svg>
      </div>
      <ul className={styles.gatewayLegend}>
        {legend.map((row) => (
          <li key={row.term.id} className={styles.gatewayLegendRow}>
            <span
              className={styles.gatewaySwatch}
              style={{ background: row.color }}
              aria-hidden="true"
            />
            <span className={styles.gatewayLegendLabel}>
              <FormattedMessage {...row.term} />
            </span>
            <span className={styles.gatewayLegendCount}>{row.value}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function NodeFields({
  node,
  onSelectBundle,
}: {
  node: MapNode;
  onSelectBundle: (seed: string) => void;
}): React.ReactElement {
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
      <Row
        term={
          <TermWithInfo
            label={<FormattedMessage {...messages.columnRole} />}
            info={messages.infoRole}
          />
        }
      >
        {node.role ?? <FormattedMessage {...messages.valueNone} />}
      </Row>
      <Row
        term={
          <TermWithInfo
            label={<FormattedMessage {...messages.detailSync} />}
            info={messages.infoSync}
          />
        }
      >
        <StatusLabel badge={syncBadge(node.argoSync)} />
      </Row>
      <Row
        term={
          <TermWithInfo
            label={<FormattedMessage {...messages.columnHealth} />}
            info={messages.infoHealth}
          />
        }
      >
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
      <Row
        term={
          <TermWithInfo
            label={<FormattedMessage {...messages.detailPromotion} />}
            info={messages.infoPromotion}
          />
        }
      >
        <Label
          status={promo.status}
          className={
            node.state === "promoting" ? styles.spinnerLabel : undefined
          }
          icon={
            node.state === "promoting" ? (
              <Spinner size="sm" aria-hidden />
            ) : undefined
          }
          variant="outline"
          isCompact
        >
          <FormattedMessage {...promo.msg} />
        </Label>
      </Row>
      <Row
        term={
          <TermWithInfo
            label={<FormattedMessage {...messages.columnRelease} />}
            info={messages.infoRelease}
          />
        }
      >
        {releaseLabel ? (
          <BundleChip
            seed={node.seed}
            label={releaseLabel}
            onSelect={onSelectBundle}
          />
        ) : (
          <FormattedMessage {...messages.valueNone} />
        )}
      </Row>
      {node.proposedVersion ? (
        <Row
          term={
            <TermWithInfo
              label={<FormattedMessage {...messages.detailProposed} />}
              info={messages.infoIncoming}
            />
          }
        >
          <BundleChip
            seed={node.proposedDigest ?? node.proposedVersion}
            label={node.proposedVersion}
            onSelect={onSelectBundle}
          />
        </Row>
      ) : null}
      {node.digest ? (
        <Row
          term={
            <TermWithInfo
              label={<FormattedMessage {...messages.detailDigest} />}
              info={messages.infoDigest}
            />
          }
        >
          <BundleChip
            seed={node.seed}
            label={<code title={node.digest}>{shortDigest(node.digest)}</code>}
            onSelect={onSelectBundle}
          />
        </Row>
      ) : null}
      {node.driftsFromColumn ? (
        <Row
          term={
            <TermWithInfo
              label={<FormattedMessage {...messages.detailDrift} />}
              info={messages.infoDrift}
            />
          }
        >
          <Label color="orange" isCompact icon={<InfoAltIcon />}>
            <FormattedMessage {...messages.driftFromHub} />
          </Label>
        </Row>
      ) : null}
      {node.managedClusters !== null ? (
        <Row term={<FormattedMessage {...messages.detailClusters} />}>
          {node.managedClusters}
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

/** One link as a full-width "card" row: an icon, a bold label with a muted
 *  sub-label beneath it, and a trailing external-link glyph. `primary` gives the
 *  main instance link a brand-tinted, heavier treatment so it sits visually above
 *  the operational links. Renders nothing when the href is absent. */
function LinkCard({
  href,
  icon,
  label,
  desc,
  primary = false,
}: {
  href: string | null;
  icon: React.ReactNode;
  label: React.ReactNode;
  desc: React.ReactNode;
  primary?: boolean;
}): React.ReactElement | null {
  if (!href) {
    return null;
  }
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer noopener"
      className={[styles.linkCard, primary ? styles.linkCardPrimary : null]
        .filter(Boolean)
        .join(" ")}
    >
      <span className={styles.linkCardIcon} aria-hidden="true">
        {icon}
      </span>
      <span className={styles.linkCardBody}>
        <span className={styles.linkCardLabel}>{label}</span>
        <span className={styles.linkCardDesc}>{desc}</span>
      </span>
      <span className={styles.linkCardChevron} aria-hidden="true">
        <ExternalLinkAltIcon />
      </span>
    </a>
  );
}

function NodeLinks({ node }: { node: MapNode }): React.ReactElement {
  // The instance's own front door is the headline action; Argo/PR/analysis are
  // operational follow-ups, grouped under their own eyebrow below it. The group
  // heading only shows when at least one operational link is present.
  const hasOps =
    Boolean(node.links.grafana) ||
    Boolean(node.links.argo) ||
    Boolean(node.links.pr) ||
    Boolean(node.links.analysis);
  return (
    <div className={styles.linkList}>
      <LinkCard
        href={node.links.console}
        primary
        icon={<ExternalLinkAltIcon />}
        label={<FormattedMessage {...messages.linkConsole} />}
        desc={<FormattedMessage {...messages.linkConsoleDesc} />}
      />
      {hasOps ? (
        <h4 className={[styles.sectionTitle, styles.linkGroupTitle].join(" ")}>
          <FormattedMessage {...messages.linksOperations} />
        </h4>
      ) : null}
      <LinkCard
        href={node.links.grafana}
        icon={<ChartLineIcon />}
        label={<FormattedMessage {...messages.linkGrafana} />}
        desc={<FormattedMessage {...messages.linkGrafanaDesc} />}
      />
      <LinkCard
        href={node.links.argo}
        icon={<ExternalLinkAltIcon />}
        label={<FormattedMessage {...messages.linkArgo} />}
        desc={<FormattedMessage {...messages.linkArgoDesc} />}
      />
      <LinkCard
        href={node.links.pr}
        icon={<ExternalLinkAltIcon />}
        label={<FormattedMessage {...messages.linkPr} />}
        desc={<FormattedMessage {...messages.linkPrDesc} />}
      />
      <LinkCard
        href={node.links.analysis}
        icon={<ExternalLinkAltIcon />}
        label={<FormattedMessage {...messages.linkAnalysis} />}
        desc={<FormattedMessage {...messages.linkAnalysisDesc} />}
      />
    </div>
  );
}

function NodeDetails({
  node,
  releaseByDigest,
  onSelectBundle,
}: {
  node: MapNode;
  releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
  onSelectBundle: (seed: string) => void;
}): React.ReactElement {
  const [activeKey, setActiveKey] = useState<string | number>("details");
  const bundle = node.digest ? releaseByDigest[node.digest] : undefined;
  return (
    <Tabs
      activeKey={activeKey}
      onSelect={(_event, key) => {
        setActiveKey(key);
      }}
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
          <div className={styles.gatewayWidget}>
            <GatewaySummary node={node} />
          </div>
          <div className="pf-v6-u-mt-md">
            <MetricTiles node={node} />
          </div>
          <div className={styles.nodeFields}>
            <NodeFields node={node} onSelectBundle={onSelectBundle} />
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
          <BundleContents
            bundle={bundle}
            seed={node.seed}
            onSelectBundle={onSelectBundle}
          />
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
  onSelectBundle,
}: {
  gateId: string;
  model: MapModel;
  onSelectBundle: (seed: string) => void;
}): React.ReactElement | null {
  const gate = model.gates.find((x) => x.id === gateId);
  if (!gate) {
    return null;
  }
  const promotingLabel =
    gate.promotingSeed !== null
      ? gate.promotingVersion
        ? `${gate.promotingVersion} · ${identiName(gate.promotingSeed)}`
        : identiName(gate.promotingSeed)
      : null;
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
          <FlexItem>
            {gate.terminal ? (
              <em>
                <FormattedMessage {...messages.detailFinalStage} />
              </em>
            ) : (
              gate.toColumnKey
            )}
          </FlexItem>
        </Flex>
      </Row>
      <Row term={<FormattedMessage {...messages.columnGates} />}>
        {gate.checks.length > 0 ? (
          <Flex
            direction={{ default: "column" }}
            spaceItems={{ default: "spaceItemsXs" }}
          >
            {gate.checks.map((check) => (
              <Flex
                key={check.name}
                spaceItems={{ default: "spaceItemsSm" }}
                alignItems={{ default: "alignItemsCenter" }}
                flexWrap={{ default: "nowrap" }}
              >
                <FlexItem className={styles.gateCheckName}>
                  {check.name}
                </FlexItem>
                <FlexItem>
                  <StatusLabel badge={check.badge} />
                </FlexItem>
              </Flex>
            ))}
          </Flex>
        ) : (
          <StatusLabel badge={gate.badge} />
        )}
      </Row>
      {gate.analysisUrl ? (
        <Row term={<FormattedMessage {...messages.detailAnalysisRun} />}>
          <Flex spaceItems={{ default: "spaceItemsSm" }}>
            <Link
              href={gate.analysisUrl}
              label={<FormattedMessage {...messages.linkAnalysis} />}
            />
          </Flex>
        </Row>
      ) : null}
      {gate.argoUrl ? (
        <Row term={<FormattedMessage {...messages.detailAnalysisLogs} />}>
          <Flex spaceItems={{ default: "spaceItemsSm" }}>
            <Link
              href={gate.argoUrl}
              label={<FormattedMessage {...messages.linkArgo} />}
            />
          </Flex>
        </Row>
      ) : null}
      <Row term={<FormattedMessage {...messages.detailPromoting} />}>
        {gate.promotingSeed !== null && promotingLabel !== null ? (
          <BundleChip
            seed={gate.promotingSeed}
            label={promotingLabel}
            onSelect={onSelectBundle}
          />
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
  onSelectNode,
}: {
  seed: string;
  model: MapModel;
  releaseByDigest: Readonly<Record<string, ReleaseBundle>>;
  onSelectNode: (id: string) => void;
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
            <code title={bundle.digest}>{shortDigest(bundle.digest)}</code>
          </Row>
        ) : null}
        {bundle.date ? (
          <Row term={<FormattedMessage {...messages.detailDate} />}>
            <ReleaseTime iso={bundle.date} mode="full" />
          </Row>
        ) : null}
      </DescriptionList>

      <div className={styles.section}>
        <h4 className={styles.sectionTitle}>
          <FormattedMessage {...messages.sectionDeployedOn} />
        </h4>
        {deployed.length > 0 ? (
          <Flex spaceItems={{ default: "spaceItemsXs" }}>
            {deployed.map((n) => (
              <FlexItem key={n.id}>
                <Label
                  variant="outline"
                  isCompact
                  onClick={() => {
                    onSelectNode(n.id);
                  }}
                >
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
      </div>

      <div className={styles.section}>
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
  onSelectBundle,
  onSelectNode,
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
        spaceItems={{ default: "spaceItemsSm" }}
        flexWrap={{ default: "nowrap" }}
      >
        <FlexItem grow={{ default: "grow" }} className={styles.headerMain}>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            spaceItems={{ default: "spaceItemsSm" }}
            flexWrap={{ default: "nowrap" }}
          >
            {selection.kind === "bundle" ? (
              <FlexItem>
                <BundleIdenticon seed={selection.id} />
              </FlexItem>
            ) : null}
            <FlexItem grow={{ default: "grow" }} className={styles.headerTitle}>
              {/* The title (a node id or a long bundle digest) always fits the
                  available width on one line: PatternFly Truncate keeps the head
                  and a fixed tail and drops a middle ellipsis in between, scaling
                  responsively as the drawer resizes. Short titles show in full. */}
              <Title headingLevel="h3" size="lg">
                <Truncate
                  content={title}
                  position="middle"
                  trailingNumChars={12}
                />
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
        <NodeDetails
          node={node}
          releaseByDigest={releaseByDigest}
          onSelectBundle={onSelectBundle}
        />
      ) : null}
      {selection.kind === "gate" ? (
        <GateDetails
          gateId={selection.id}
          model={model}
          onSelectBundle={onSelectBundle}
        />
      ) : null}
      {selection.kind === "bundle" ? (
        <BundleDetails
          seed={selection.id}
          model={model}
          releaseByDigest={releaseByDigest}
          onSelectNode={onSelectNode}
        />
      ) : null}
    </div>
  );
}
