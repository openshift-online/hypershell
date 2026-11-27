import "@patternfly/react-core/dist/styles/base.css";

import {
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  EmptyState,
  EmptyStateBody,
  Flex,
  FlexItem,
  Grid,
  GridItem,
  Icon,
  PageSection,
  Popover,
  ProgressStep,
  ProgressStepper,
  Spinner,
  Stack,
  StackItem,
  Title,
  Tooltip,
  Truncate,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  CubesIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  ExternalLinkAltIcon,
  InProgressIcon,
  PendingIcon,
  QuestionCircleIcon,
  RedoIcon,
  SyncAltIcon,
} from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";

import { useRef, useState } from "react";

import { HealthStatus } from "../../common/health-status";
import { initialsDataUri } from "./initials";
import styles from "./delivery-pipeline.module.css";
import {
  FIXTURES,
  RETRIED_DETAIL,
  RETRIED_SHA,
  SELECTED_DETAIL,
  SELECTED_SHA,
  STAGES,
  type Change,
  type ChangeDetail,
  type DeliveryFixture,
  type DeliveryState,
  type NextItem,
  type NowNextRow,
  type PipelineNow,
  type PromotionAttempt,
  type PromotionOutcome,
  type Severity,
  type StageCell,
  type EnvOutcome,
  type EnvironmentRow,
  type StageState,
  type WhereToLook,
  SHIPPED_HUBS,
  SHIPPED_ROWS,
} from "./delivery-pipeline-fixtures";

// Glass Delivery tab page surface (fleet-dashboard-delivery-pipeline.spec,
// FLT-DLV-01..24). Page-owned: header region, Pipeline now panel, the Changes
// on main strip of change panels, and every reachable state. The Glass tab bar
// and masthead are shell chrome and live in the design-review story only.

// FLT-DLV-24: every clock time reveals its full date and time on hover and
// keyboard focus. The mockup's day is fixed.
const CLOCK = /\b\d{1,2}:\d{2}\b/g;

function FullTime({ clock }: { clock: string }) {
  const full = `Mon 9 Nov 2026, ${clock}:00 CET`;
  // triggerRef keeps the Tooltip from wrapping the time in a div, so a time
  // can sit inside a paragraph.
  const ref = useRef<HTMLTimeElement>(null);
  return (
    <>
      <time
        className={styles.time}
        dateTime={`2026-11-09T${clock.padStart(5, "0")}:00+01:00`}
        ref={ref}
        tabIndex={0}
      >
        {clock}
      </time>
      <Tooltip content={full} triggerRef={ref} />
    </>
  );
}

function Timed({ text }: { text: string }) {
  const parts: React.ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(CLOCK)) {
    const at = m.index ?? 0;
    if (at > last) parts.push(text.slice(last, at));
    parts.push(<FullTime clock={m[0]} key={at} />);
    last = at + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return <>{parts}</>;
}

// FLT-DLV-24: a bundle is named with its Initials identicon and alias.
function BundleName({ alias }: { alias: string }) {
  return (
    <span className="pf-v6-u-text-nowrap">
      <img
        alt=""
        aria-hidden
        className={styles.identicon}
        height={20}
        src={initialsDataUri(alias)}
        width={20}
      />
      {alias}
    </span>
  );
}

const STATE_LABEL: Readonly<Record<StageState, string>> = {
  "not-started": "Not started",
  waiting: "Waiting",
  running: "Running",
  passed: "Passed",
  failed: "Failed",
  "not-needed": "Not needed",
  unknown: "Unknown",
};

// FLT-DLV-04: every state is an icon paired with a text label.
function StageStateIcon({ state }: { state: StageState }) {
  switch (state) {
    case "running":
      return (
        <Icon isInline status="info">
          <InProgressIcon aria-hidden />
        </Icon>
      );
    case "passed":
      return (
        <Icon isInline status="success">
          <CheckCircleIcon aria-hidden />
        </Icon>
      );
    case "failed":
      return (
        <Icon isInline status="danger">
          <ExclamationCircleIcon aria-hidden />
        </Icon>
      );
    case "waiting":
      return (
        <Icon isInline>
          <PendingIcon aria-hidden />
        </Icon>
      );
    case "unknown":
      return (
        <Icon isInline>
          <QuestionCircleIcon aria-hidden />
        </Icon>
      );
    default:
      return null;
  }
}

// FLT-DLV-20: opens on hover, keyboard focus, and tap; Escape closes it.
function WhereToLookPopover({
  look,
  initiallyOpen = false,
}: {
  look: WhereToLook;
  initiallyOpen?: boolean;
}) {
  const [open, setOpen] = useState(initiallyOpen);
  return (
    <Popover
      aria-label={`Where to look: ${look.phase}`}
      bodyContent={
        <DescriptionList isCompact>
          {look.items.map((item) => (
            <DescriptionListGroup key={item.term}>
              <DescriptionListTerm>{item.term}</DescriptionListTerm>
              <DescriptionListDescription>
                {item.text}
                {item.link ? (
                  <>
                    {" "}
                    <ExternalLink>{item.link}</ExternalLink>
                  </>
                ) : null}
                {item.warning ? (
                  <HealthStatus appearance="warning" label={item.warning} />
                ) : null}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
        </DescriptionList>
      }
      headerContent={`Where to look: ${look.phase}`}
      isVisible={open}
      position="bottom"
      shouldClose={() => setOpen(false)}
      shouldOpen={() => setOpen(true)}
      triggerAction="hover"
    >
      <Button
        className="pf-v6-u-font-size-sm"
        isInline
        onClick={() => setOpen((v) => !v)}
        variant="link"
      >
        Where to look
      </Button>
    </Popover>
  );
}

function StageStatus({ cell }: { cell: StageCell }) {
  const muted =
    cell.state === "not-started" ||
    cell.state === "not-needed" ||
    cell.state === "unknown";
  const secondary = [cell.primary, cell.detail].filter(Boolean).join(" - ");

  return (
    <Stack>
      <StackItem className="pf-v6-u-text-nowrap">
        <span className={muted ? "pf-v6-u-text-color-subtle" : undefined}>
          <StageStateIcon state={cell.state} /> {STATE_LABEL[cell.state]}
        </span>
      </StackItem>
      {secondary ? (
        <StackItem className="pf-v6-u-font-size-sm pf-v6-u-text-color-subtle">
          <Timed text={secondary} />
        </StackItem>
      ) : null}
      {cell.overdue ? (
        <StackItem className="pf-v6-u-font-size-sm">
          <HealthStatus appearance="warning" label="Overdue" />
        </StackItem>
      ) : null}
      {cell.whereToLook ? (
        <StackItem>
          <WhereToLookPopover look={cell.whereToLook} />
        </StackItem>
      ) : null}
      {cell.failedAttempts ? (
        <StackItem className="pf-v6-u-font-size-sm">
          {/* FLT-DLV-18: a Passed group that needed more than one bundle. */}
          <HealthStatus
            appearance="warning"
            label={
              cell.failedAttempts === 1
                ? "1 failed attempt"
                : `${cell.failedAttempts} failed attempts`
            }
          />
        </StackItem>
      ) : null}
    </Stack>
  );
}

const OUTCOME_LABEL: Readonly<Record<PromotionOutcome, string>> = {
  passed: "Passed",
  failed: "Failed",
  replaced: "Replaced",
  running: "Running",
  unknown: "Unknown",
};

function OutcomeIcon({ outcome }: { outcome: PromotionOutcome }) {
  if (outcome === "replaced") {
    return (
      <Icon isInline>
        <RedoIcon aria-hidden />
      </Icon>
    );
  }
  return <StageStateIcon state={outcome} />;
}

// FLT-DLV-18: what happened in one environment, oldest promotion first.
function PromotionHistory({
  attempts,
}: {
  attempts: readonly PromotionAttempt[];
}) {
  return (
    <Stack className="pf-v6-u-font-size-sm pf-v6-u-mt-xs">
      {attempts.map((a) => (
        <StackItem key={`${a.gitopsPr}-${a.promoted}`}>
          <OutcomeIcon outcome={a.outcome} /> {OUTCOME_LABEL[a.outcome]}:{" "}
          <BundleName alias={a.bundle} /> ({a.gitopsPr}), promoted{" "}
          <Timed text={a.promoted} />
          {a.detail ? (
            <span className="pf-v6-u-text-color-subtle"> - {a.detail}</span>
          ) : null}
        </StackItem>
      ))}
    </Stack>
  );
}

// FLT-DLV-18: an environment reported by the change's first bundle only.
function EnvOutcomeLine({ outcome }: { outcome: EnvOutcome }) {
  switch (outcome.kind) {
    case "deployed":
      return (
        <span>
          <StageStateIcon state="passed" /> Passed -{" "}
          <BundleName alias={outcome.bundle} /> deployed{" "}
          <Timed text={outcome.deployed} />
          {outcome.gatesPassed ? (
            <>
              {" "}
              - Gates passed <Timed text={outcome.gatesPassed} />
            </>
          ) : null}
        </span>
      );
    case "failed":
      return (
        <span>
          <StageStateIcon state="failed" /> Failed -{" "}
          <BundleName alias={outcome.bundle} /> deployed{" "}
          <Timed text={outcome.deployed} /> - {outcome.detail}
        </span>
      );
    default:
      return (
        <span>
          <OutcomeIcon outcome="replaced" /> Not deployed - replaced by{" "}
          <BundleName alias={outcome.replacedBy} />{" "}
          <Timed text={outcome.promoted} />
        </span>
      );
  }
}

function SummaryIcon({ severity }: { severity: Severity }) {
  switch (severity) {
    case "danger":
      return (
        <Icon status="danger">
          <ExclamationCircleIcon aria-hidden />
        </Icon>
      );
    case "warning":
      return (
        <Icon status="warning">
          <ExclamationTriangleIcon aria-hidden />
        </Icon>
      );
    case "good":
      return (
        <Icon status="success">
          <CheckCircleIcon aria-hidden />
        </Icon>
      );
    default:
      return (
        <Icon status="info">
          <InProgressIcon aria-hidden />
        </Icon>
      );
  }
}

function changeName(shortSha: string, pr?: number) {
  return pr ? `${shortSha} (#${pr})` : shortSha;
}

function StateText({ state, text }: { state: StageState; text: string }) {
  return (
    <>
      <StageStateIcon state={state} /> {STATE_LABEL[state]}
      <span className="pf-v6-u-text-color-subtle">
        {" - "}
        <Timed text={text} />
      </span>
    </>
  );
}

function nextKey(item: NextItem): string {
  return typeof item === "string" ? item : `${item.bundle}-${item.text}`;
}

// FLT-DLV-23: an expected event; a bundle event shows the bundle first.
function NextLine({ item }: { item: NextItem }) {
  if (typeof item === "string") return <Timed text={item} />;
  return (
    <>
      <BundleName alias={item.bundle} />
      {" - "}
      <Timed text={item.text} />
    </>
  );
}

// FLT-DLV-23: the situation at this moment - both heads, what is in flight,
// what happens next, the bundle train, and every environment.
function PipelineNowCard({ now }: { now: PipelineNow }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            Pipeline now
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <Stack hasGutter>
          {now.loading ? (
            <StackItem>
              <Flex
                alignItems={{ default: "alignItemsCenter" }}
                spaceItems={{ default: "spaceItemsSm" }}
              >
                <Spinner size="md" aria-hidden="true" />
                <span>{now.loading}</span>
              </Flex>
            </StackItem>
          ) : now.unknown ? (
            <StackItem>
              <HealthStatus appearance="warning" label={now.unknown} />
            </StackItem>
          ) : null}
          <StackItem>
            <Grid hasGutter md={4}>
              <GridItem>
                <Title headingLevel="h3" size="md">
                  Heads
                </Title>
                <DescriptionList isCompact>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Application main</DescriptionListTerm>
                    <DescriptionListDescription>
                      <Timed text={now.appHead} />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>GitOps main</DescriptionListTerm>
                    <DescriptionListDescription>
                      <Timed text={now.gitopsHead} />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </GridItem>
              <GridItem>
                <Title headingLevel="h3" size="md">
                  In flight
                </Title>
                {now.inFlight.length === 0 ? (
                  <Content component="p">Nothing in flight</Content>
                ) : (
                  <DescriptionList isCompact>
                    {now.inFlight.map((item) => (
                      <DescriptionListGroup key={item.term}>
                        <DescriptionListTerm>{item.term}</DescriptionListTerm>
                        <DescriptionListDescription>
                          <StateText state={item.state} text={item.text} />
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    ))}
                  </DescriptionList>
                )}
              </GridItem>
              <GridItem>
                <Title headingLevel="h3" size="md">
                  Next
                </Title>
                {now.next.length === 0 ? (
                  <Content component="p">Nothing expected</Content>
                ) : (
                  <Content component="ol">
                    {now.next.map((item) => (
                      <li key={nextKey(item)}>
                        <NextLine item={item} />
                      </li>
                    ))}
                  </Content>
                )}
              </GridItem>
            </Grid>
          </StackItem>
          {now.train ? (
            <StackItem>
              {/* FLT-DLV-22: one bundle per finished build. */}
              <Table aria-label="Bundle train" variant="compact">
                <Thead>
                  <Tr>
                    <Th>Bundle train</Th>
                    <Th>Published</Th>
                    <Th>Changed</Th>
                    <Th>Where</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {now.train.map((b) => (
                    <Tr key={b.alias}>
                      <Td dataLabel="Bundle train">
                        <StageStateIcon state={b.state} />{" "}
                        <BundleName alias={b.alias} />
                      </Td>
                      <Td dataLabel="Published">
                        <Timed text={b.published} />
                      </Td>
                      <Td dataLabel="Changed">{b.changed}</Td>
                      <Td dataLabel="Where">{b.where}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            </StackItem>
          ) : null}
          <StackItem>
            <Table aria-label="Environments now" variant="compact">
              <Thead>
                <Tr>
                  <Th>Environment</Th>
                  <Th>Group</Th>
                  <Th>Runs</Th>
                  <Th>State</Th>
                </Tr>
              </Thead>
              <Tbody>
                {now.environments.map((env) => (
                  <Tr key={env.instance}>
                    <Td dataLabel="Environment">{env.instance}</Td>
                    <Td dataLabel="Group">{env.group}</Td>
                    <Td dataLabel="Runs">
                      <BundleName alias={env.bundle} />
                    </Td>
                    <Td dataLabel="State">
                      <StateText state={env.state} text={env.text} />
                    </Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  );
}

const STEP_VARIANT: Readonly<
  Record<StageState, "default" | "success" | "info" | "pending" | "danger">
> = {
  "not-started": "pending",
  waiting: "pending",
  running: "info",
  passed: "success",
  failed: "danger",
  "not-needed": "default",
  unknown: "default",
};

function ExternalLink({ children }: { children: React.ReactNode }) {
  return (
    <Button
      component="a"
      href="#"
      icon={<ExternalLinkAltIcon />}
      iconPosition="end"
      isInline
      variant="link"
    >
      {children}
    </Button>
  );
}

// The bundle a Release cell names: its alias is the detail's first part.
function releaseBundle(cell: StageCell): string | undefined {
  if (cell.state !== "passed" || !cell.detail) return undefined;
  return cell.detail.split(",")[0];
}

// FLT-DLV-19: one numbered step per stage, each with its state and a time.
// FLT-DLV-19: a group step's environments, each linked to the Fleet Map and
// reported by the change's first bundle (FLT-DLV-18).
function EnvList({ envs }: { envs: readonly EnvironmentRow[] }) {
  return (
    <Stack className="pf-v6-u-font-size-sm">
      {envs.map((env) => (
        <StackItem key={env.instance}>
          <Button
            aria-label={`Open ${env.instance} on Fleet Map`}
            component="a"
            href="#"
            isInline
            variant="link"
          >
            {env.instance}
          </Button>
          :{" "}
          {env.outcome ? (
            <EnvOutcomeLine outcome={env.outcome} />
          ) : (
            <StageStatus cell={env.cell} />
          )}
          {env.history ? <PromotionHistory attempts={env.history} /> : null}
        </StackItem>
      ))}
    </Stack>
  );
}

function StageStep({
  stageKey,
  cell,
  openWhereToLook,
  envs = [],
}: {
  stageKey: string;
  cell: StageCell;
  openWhereToLook: boolean;
  envs?: readonly EnvironmentRow[];
}) {
  const bundle = stageKey === "release" ? releaseBundle(cell) : undefined;
  const rest = bundle
    ? cell.detail?.slice(bundle.length).replace(/^,\s*/, "")
    : cell.detail;
  return (
    <Stack>
      <StackItem className="pf-v6-u-font-size-sm">
        {STATE_LABEL[cell.state]}
        {cell.primary ? (
          <>
            {" - "}
            <Timed text={cell.primary} />
          </>
        ) : null}
      </StackItem>
      {bundle ? (
        <StackItem className="pf-v6-u-font-size-sm">
          <BundleName alias={bundle} />
          {rest ? (
            <span className="pf-v6-u-text-color-subtle"> - {rest}</span>
          ) : null}
        </StackItem>
      ) : rest ? (
        <StackItem className="pf-v6-u-font-size-sm pf-v6-u-text-color-subtle">
          <Timed text={rest} />
        </StackItem>
      ) : null}
      {cell.overdue ? (
        <StackItem className="pf-v6-u-font-size-sm">
          <HealthStatus appearance="warning" label="Overdue" />
        </StackItem>
      ) : null}
      {cell.whereToLook ? (
        <StackItem>
          <WhereToLookPopover
            initiallyOpen={openWhereToLook}
            look={cell.whereToLook}
          />
        </StackItem>
      ) : null}
      {cell.failedAttempts ? (
        <StackItem className="pf-v6-u-font-size-sm">
          <HealthStatus
            appearance="warning"
            label={
              cell.failedAttempts === 1
                ? "1 failed attempt"
                : `${cell.failedAttempts} failed attempts`
            }
          />
        </StackItem>
      ) : null}
      {envs.length > 0 ? (
        <StackItem>
          <EnvList envs={envs} />
        </StackItem>
      ) : null}
    </Stack>
  );
}

// FLT-DLV-11: the details of one change, shown in its panel without a click.
function PanelDetails({ detail }: { detail: ChangeDetail }) {
  return (
    <Stack hasGutter>
      {detail.checks.length > 0 ? (
        <StackItem>
          <Title headingLevel="h4" size="md">
            Builds and policy checks
          </Title>
          <DescriptionList isCompact>
            {detail.checks.map((check) => (
              <DescriptionListGroup key={check.component}>
                <DescriptionListTerm>{check.component}</DescriptionListTerm>
                <DescriptionListDescription>
                  Build: {check.build}. Policy check: {check.policy}.{" "}
                  <ExternalLink>View check runs</ExternalLink>
                </DescriptionListDescription>
              </DescriptionListGroup>
            ))}
          </DescriptionList>
        </StackItem>
      ) : null}
      <StackItem>
        <Title headingLevel="h4" size="md">
          Bundle
        </Title>
        <DescriptionList isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>Version</DescriptionListTerm>
            <DescriptionListDescription>
              <BundleName alias={detail.bundle.alias} /> - published{" "}
              <Timed text={detail.bundle.published} />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Tag</DescriptionListTerm>
            <DescriptionListDescription>
              <Truncate content={detail.bundle.tag} position="middle" />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Digest</DescriptionListTerm>
            <DescriptionListDescription>
              <Truncate content={detail.bundle.digest} position="middle" />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Downloads</DescriptionListTerm>
            <DescriptionListDescription>
              <ExternalLink>
                <Timed text={detail.bundle.downloads} />
              </ExternalLink>
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>Renovate</DescriptionListTerm>
            <DescriptionListDescription>
              <ExternalLink>
                <Timed text={detail.gitopsPr} />
              </ExternalLink>
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </StackItem>
    </Stack>
  );
}

function panelSeverity(change: Change): Severity {
  const cells = Object.values(change.stages);
  if (cells.some((c) => c.state === "failed")) return "danger";
  if (cells.some((c) => c.overdue)) return "warning";
  if (cells.every((c) => c.state === "passed" || c.state === "not-needed"))
    return "good";
  return "info";
}

// FLT-DLV-03, FLT-DLV-10, FLT-DLV-11: one change, its state, its next step,
// its stepper, and its details.
function ChangePanel({
  change,
  row,
  detail,
  selected,
  openWhereToLook,
}: {
  change: Change;
  row?: NowNextRow;
  detail?: ChangeDetail;
  selected: boolean;
  openWhereToLook: boolean;
}) {
  const name = changeName(change.shortSha, change.pr);
  const prod = change.stages.prod;
  const deliveredAt = row
    ? undefined
    : prod?.state === "passed"
      ? prod.primary
      : undefined;
  return (
    <li className={styles.panel} data-selected={selected ? "true" : undefined}>
      <Card isCompact isFullHeight isPlain>
        <CardHeader>
          <CardTitle>
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              flexWrap={{ default: "nowrap" }}
              spaceItems={{ default: "spaceItemsSm" }}
            >
              <FlexItem>
                <SummaryIcon severity={panelSeverity(change)} />
              </FlexItem>
              <FlexItem>
                <Title
                  className="pf-v6-u-font-family-monospace"
                  headingLevel="h3"
                  size="md"
                >
                  {name}
                </Title>
              </FlexItem>
              {row?.progress.percent !== undefined ? (
                <FlexItem align={{ default: "alignRight" }}>
                  <span className="pf-v6-u-font-size-sm">
                    {row.progress.percent}%
                  </span>
                </FlexItem>
              ) : null}
            </Flex>
          </CardTitle>
          <Content component="p">
            <Truncate content={change.title} />
          </Content>
          <Content className="pf-v6-u-text-color-subtle" component="small">
            by {change.author}
            {" - "}
            <ExternalLink>Pull request</ExternalLink>{" "}
            <ExternalLink>Commit</ExternalLink>
          </Content>
        </CardHeader>
        <CardBody>
          <Stack hasGutter>
            <StackItem>
              {row ? (
                <DescriptionList isCompact>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Now</DescriptionListTerm>
                    <DescriptionListDescription>
                      <Timed text={row.now} />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Next</DescriptionListTerm>
                    <DescriptionListDescription>
                      <Timed text={row.next} />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              ) : change.outsideBundle ? (
                <Content component="p">
                  Done here - ships outside the bundle
                </Content>
              ) : deliveredAt ? (
                <Content component="p">
                  Delivered <Timed text={deliveredAt} />
                </Content>
              ) : null}
            </StackItem>
            <StackItem>
              <ProgressStepper
                aria-label={`Delivery stages of ${name}`}
                isVertical
              >
                {/* FLT-DLV-06.08: an outside-bundle change ends at Release. */}
                {(change.outsideBundle
                  ? STAGES.filter((stage) => stage.key in change.stages)
                  : STAGES
                ).map((stage, i) => {
                  const cell = change.stages[stage.key] ?? {
                    state: "unknown" as const,
                  };
                  const arrival =
                    i > 0 && cell.state === "not-started"
                      ? row?.progress.cells[i - 1]?.time
                      : undefined;
                  return (
                    <ProgressStep
                      description={
                        <StageStep
                          cell={
                            arrival
                              ? { ...cell, primary: `about ${arrival}` }
                              : cell
                          }
                          envs={detail?.environments.filter(
                            (env) => env.group === stage.label,
                          )}
                          openWhereToLook={openWhereToLook}
                          stageKey={stage.key}
                        />
                      }
                      icon={
                        <span className="pf-v6-u-font-size-xs">{i + 1}</span>
                      }
                      id={`${change.shortSha}-${stage.key}`}
                      isCurrent={
                        cell.state === "running" || cell.state === "waiting"
                      }
                      key={stage.key}
                      titleId={`${change.shortSha}-${stage.key}-title`}
                      variant={STEP_VARIANT[cell.state]}
                    >
                      {stage.label}
                    </ProgressStep>
                  );
                })}
              </ProgressStepper>
            </StackItem>
            {detail ? (
              <StackItem>
                <PanelDetails detail={detail} />
              </StackItem>
            ) : null}
          </Stack>
        </CardBody>
      </Card>
    </li>
  );
}

// FLT-DLV-03: every change on main, newest first, side by side. Only the
// strip scrolls sideways.
// FLT-DLV-25: the Shipped release-by-hub-cluster matrix (SDLC-VIS-11), moved
// here from the Agent SDLC tab. Rows expand to their issues, which link to GitHub.
function ShippedCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            Shipped
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <Table aria-label="Shipped" variant="compact">
          <Thead>
            <Tr>
              <Th>Bundle</Th>
              {SHIPPED_HUBS.map((hub) => (
                <Th key={hub}>{hub}</Th>
              ))}
              <Th>Contents</Th>
            </Tr>
          </Thead>
          <Tbody>
            {SHIPPED_ROWS.map((row) => (
              <Tr key={row.alias ?? "unreleased"}>
                <Td dataLabel="Bundle">
                  {row.alias ? (
                    <Button component="a" href="#" isInline variant="link">
                      <BundleName alias={row.alias} />
                    </Button>
                  ) : (
                    "Unreleased"
                  )}
                </Td>
                {SHIPPED_HUBS.map((hub, i) => (
                  <Td dataLabel={hub} key={hub}>
                    {row.current[i] ? (
                      <>
                        <StageStateIcon state="passed" /> current
                      </>
                    ) : (
                      "-"
                    )}
                  </Td>
                ))}
                <Td dataLabel="Contents">{row.contents}</Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      </CardBody>
    </Card>
  );
}

function ChangeStrip({
  fixture,
  selectedSha,
  openWhereToLook,
}: {
  fixture: DeliveryFixture;
  selectedSha?: string;
  openWhereToLook: boolean;
}) {
  const rows = new Map(fixture.summary.map((r) => [r.shortSha, r]));
  const details: Record<string, ChangeDetail> = {
    [SELECTED_SHA]: SELECTED_DETAIL,
    [RETRIED_SHA]: RETRIED_DETAIL,
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            Changes on main
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <ol
          aria-label="Changes on main, newest first"
          className={styles.strip}
          tabIndex={0}
        >
          {fixture.changes.map((change) => (
            <ChangePanel
              change={change}
              detail={details[change.sha]}
              key={change.sha}
              openWhereToLook={
                openWhereToLook &&
                Object.values(change.stages).some((c) => c.overdue)
              }
              row={rows.get(change.shortSha)}
              selected={change.sha === selectedSha}
            />
          ))}
        </ol>
      </CardBody>
    </Card>
  );
}

function PageHeader({
  asOf,
  refreshing = false,
}: {
  asOf?: string;
  refreshing?: boolean;
}) {
  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      justifyContent={{ default: "justifyContentSpaceBetween" }}
    >
      <FlexItem>
        <Content>
          {refreshing ? (
            <Content className="pf-v6-u-text-color-subtle" component="small">
              Refreshing...
            </Content>
          ) : asOf ? (
            <Content className="pf-v6-u-text-color-subtle" component="small">
              Data as of {asOf}
            </Content>
          ) : null}
        </Content>
      </FlexItem>
      <FlexItem>
        <Button
          aria-label="Refresh"
          aria-busy={refreshing}
          icon={
            <SyncAltIcon
              className={refreshing ? styles.refreshSpin : undefined}
            />
          }
          variant="plain"
        />
      </FlexItem>
    </Flex>
  );
}

function LoadedBody({
  fixture,
  selectedSha,
  openWhereToLook = false,
}: {
  fixture: DeliveryFixture;
  selectedSha?: string;
  openWhereToLook?: boolean;
}) {
  return (
    <Stack hasGutter>
      {fixture.loadingAlert ? (
        <StackItem>
          <Alert
            isInline
            customIcon={<Spinner size="md" aria-hidden="true" />}
            title={fixture.loadingAlert.title}
            variant="info"
          >
            {fixture.loadingAlert.body}
          </Alert>
        </StackItem>
      ) : null}
      {fixture.sourceAlert ? (
        <StackItem>
          <Alert isInline title={fixture.sourceAlert.title} variant="warning">
            {fixture.sourceAlert.body}
          </Alert>
        </StackItem>
      ) : null}
      <StackItem>
        <PipelineNowCard now={fixture.pipelineNow} />
      </StackItem>
      <StackItem>
        <ShippedCard />
      </StackItem>
      <StackItem>
        <ChangeStrip
          fixture={fixture}
          openWhereToLook={openWhereToLook}
          selectedSha={selectedSha}
        />
      </StackItem>
    </Stack>
  );
}

function Body({ state }: { state: DeliveryState }) {
  switch (state) {
    case "loading":
      return (
        <EmptyState
          headingLevel="h2"
          icon={Spinner}
          titleText="Loading delivery data"
        />
      );
    case "not-configured":
      return (
        <EmptyState
          headingLevel="h2"
          icon={CubesIcon}
          titleText="Delivery source not configured"
        >
          <EmptyStateBody>
            Set the bundle registry repository for this dashboard to follow
            changes from merge to every environment. The other tabs are not
            affected.
          </EmptyStateBody>
        </EmptyState>
      );
    case "error":
      return (
        <Alert isInline title="Unable to load delivery data" variant="danger">
          The delivery data could not be loaded. Try refreshing the page.
        </Alert>
      );
    case "change-selected":
      // FLT-DLV-14: the `change` parameter scrolls to and selects a panel.
      return (
        <LoadedBody
          fixture={FIXTURES["change-selected"]}
          selectedSha={SELECTED_SHA}
        />
      );
    case "promotion-retried":
      return (
        <LoadedBody
          fixture={FIXTURES["promotion-retried"]}
          selectedSha={RETRIED_SHA}
        />
      );
    case "release-overdue":
      // FLT-DLV-20: the Where to look popup is open on the Overdue Release.
      return <LoadedBody fixture={FIXTURES[state]} openWhereToLook />;
    default:
      return <LoadedBody fixture={FIXTURES[state]} />;
  }
}

export function DeliveryPipelinePageSurface({
  state,
}: {
  state: DeliveryState;
}) {
  const asOf =
    state === "loading" || state === "not-configured" || state === "error"
      ? undefined
      : FIXTURES[state].asOf;

  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <PageHeader
          asOf={asOf}
          refreshing={
            state !== "loading" &&
            state !== "not-configured" &&
            state !== "error" &&
            FIXTURES[state].refreshing === true
          }
        />
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <Body state={state} />
      </PageSection>
    </>
  );
}
