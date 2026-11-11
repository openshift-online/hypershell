import "@patternfly/react-core/dist/styles/base.css";

import {
  ActionList,
  ActionListItem,
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  DataList,
  DataListCell,
  DataListItem,
  DataListItemCells,
  DataListItemRow,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Drawer,
  DrawerActions,
  DrawerCloseButton,
  DrawerContent,
  DrawerContentBody,
  DrawerHead,
  DrawerPanelBody,
  DrawerPanelContent,
  EmptyState,
  EmptyStateBody,
  Flex,
  FlexItem,
  Icon,
  PageSection,
  ProgressStep,
  ProgressStepper,
  Spinner,
  Stack,
  StackItem,
  Title,
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
import {
  InnerScrollContainer,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@patternfly/react-table";

import { HealthStatus } from "../../common/health-status";
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
  type PromotionAttempt,
  type PromotionOutcome,
  type Severity,
  type StageCell,
  type StageState,
} from "./delivery-pipeline-fixtures";

// Glass Delivery tab page surface (fleet-dashboard-delivery-pipeline.spec,
// FLT-DLV-01..16). Page-owned: header region, Now and next summary, change
// table, details drawer, and every reachable state. The Glass tab bar and
// masthead are shell chrome and live in the design-review story only.

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
          {secondary}
        </StackItem>
      ) : null}
      {cell.slow ? (
        <StackItem className="pf-v6-u-font-size-sm">
          <HealthStatus appearance="warning" label="Slower than usual" />
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
          {a.bundle} ({a.gitopsPr}), promoted {a.promoted}
          {a.detail ? (
            <span className="pf-v6-u-text-color-subtle"> - {a.detail}</span>
          ) : null}
        </StackItem>
      ))}
    </Stack>
  );
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

// FLT-DLV-10: one row per undelivered change, newest first, at most 3.
function NowAndNext({ fixture }: { fixture: DeliveryFixture }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            spaceItems={{ default: "spaceItemsSm" }}
          >
            <FlexItem>
              <SummaryIcon severity={fixture.summarySeverity} />
            </FlexItem>
            <FlexItem>
              <Title headingLevel="h2" size="lg">
                Now and next
              </Title>
            </FlexItem>
          </Flex>
        </CardTitle>
      </CardHeader>
      <CardBody>
        {fixture.deliveredSummary ? (
          <Content component="p">{fixture.deliveredSummary}</Content>
        ) : (
          <DataList aria-label="Changes in flight" isCompact>
            {fixture.summary.map((row) => (
              <DataListItem
                aria-labelledby={`now-next-${row.shortSha}`}
                key={row.shortSha}
              >
                <DataListItemRow>
                  <DataListItemCells
                    dataListCells={[
                      <DataListCell key="change" width={2}>
                        <Content
                          className="pf-v6-u-font-family-monospace"
                          component="p"
                          id={`now-next-${row.shortSha}`}
                        >
                          {changeName(row.shortSha, row.pr)}
                        </Content>
                        <Content
                          className="pf-v6-u-text-color-subtle"
                          component="small"
                        >
                          <Truncate content={row.title} />
                        </Content>
                      </DataListCell>,
                      <DataListCell key="now" width={3}>
                        <Content component="p">
                          <strong>Now</strong>
                        </Content>
                        <Flex spaceItems={{ default: "spaceItemsSm" }}>
                          <FlexItem>
                            <SummaryIcon severity={row.severity} />
                          </FlexItem>
                          <FlexItem flex={{ default: "flex_1" }}>
                            {row.now}
                          </FlexItem>
                        </Flex>
                      </DataListCell>,
                      <DataListCell key="next" width={3}>
                        <Content component="p">
                          <strong>Next</strong>
                        </Content>
                        {row.next}
                      </DataListCell>,
                    ]}
                  />
                </DataListItemRow>
              </DataListItem>
            ))}
          </DataList>
        )}
      </CardBody>
    </Card>
  );
}

// FLT-DLV-03: one row per change, one column per stage, then environment
// groups. The first column stays visible while the stages scroll sideways.
function ChangeTable({
  changes,
  selectedSha,
}: {
  changes: readonly Change[];
  selectedSha?: string;
}) {
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
        <InnerScrollContainer>
          <Table aria-label="Changes on main and their delivery stages">
            <Thead>
              <Tr>
                <Th hasRightBorder isStickyColumn stickyMinWidth="18rem">
                  Change
                </Th>
                {STAGES.map((stage) => (
                  <Th key={stage.key} modifier="nowrap">
                    {stage.label}
                  </Th>
                ))}
              </Tr>
            </Thead>
            <Tbody>
              {changes.map((change) => (
                <Tr
                  isClickable
                  isRowSelected={change.sha === selectedSha}
                  key={change.sha}
                  onRowClick={() => undefined}
                >
                  <Td
                    dataLabel="Change"
                    hasRightBorder
                    isStickyColumn
                    stickyMinWidth="18rem"
                  >
                    <Button
                      className="pf-v6-u-font-family-monospace"
                      isInline
                      variant="link"
                    >
                      {changeName(change.shortSha, change.pr)}
                    </Button>
                    <Content
                      className="pf-v6-u-text-color-subtle"
                      component="small"
                    >
                      <Truncate content={change.title} />
                    </Content>
                    <Content
                      className="pf-v6-u-text-color-subtle"
                      component="small"
                    >
                      by {change.author}
                    </Content>
                  </Td>
                  {STAGES.map((stage) => (
                    <Td dataLabel={stage.label} key={stage.key}>
                      <StageStatus
                        cell={change.stages[stage.key] ?? { state: "unknown" }}
                      />
                    </Td>
                  ))}
                </Tr>
              ))}
            </Tbody>
          </Table>
        </InnerScrollContainer>
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

// FLT-DLV-11: the selected change's stages, checks, bundle, and environments.
function ChangeDetails({ detail }: { detail: ChangeDetail }) {
  const groups = Array.from(new Set(detail.environments.map((e) => e.group)));

  return (
    <DrawerPanelContent widths={{ default: "width_50", xl: "width_33" }}>
      <DrawerHead>
        <Title headingLevel="h2" size="xl">
          <span className="pf-v6-u-font-family-monospace">
            {changeName(detail.shortSha, detail.pr)}
          </span>
        </Title>
        <Content component="p">{detail.title}</Content>
        <Content className="pf-v6-u-text-color-subtle" component="small">
          by {detail.author}
        </Content>
        <DrawerActions>
          <DrawerCloseButton onClick={() => undefined} />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h3" size="md">
              Stages
            </Title>
            <ProgressStepper aria-label="Delivery stages" isVertical>
              {detail.timeline.map((step) => (
                <ProgressStep
                  description={step.description}
                  id={`step-${step.label}`}
                  isCurrent={step.state === "running"}
                  key={step.label}
                  titleId={`step-${step.label}-title`}
                  variant={STEP_VARIANT[step.state]}
                >
                  {step.label}
                </ProgressStep>
              ))}
            </ProgressStepper>
          </StackItem>
          {detail.checks.length > 0 ? (
            <StackItem>
              <Title headingLevel="h3" size="md">
                Builds and policy checks
              </Title>
              <DescriptionList isCompact isHorizontal>
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
            <Title headingLevel="h3" size="md">
              Bundle
            </Title>
            <DescriptionList isCompact isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Alias</DescriptionListTerm>
                <DescriptionListDescription>
                  {detail.bundle.alias}
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
                <DescriptionListTerm>Published</DescriptionListTerm>
                <DescriptionListDescription>
                  {detail.bundle.published}.{" "}
                  <ExternalLink>{detail.bundle.downloads}</ExternalLink>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>GitOps update</DescriptionListTerm>
                <DescriptionListDescription>
                  <ExternalLink>{detail.gitopsPr}</ExternalLink>
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </StackItem>
          <StackItem>
            <Title headingLevel="h3" size="md">
              Environments
            </Title>
            <Stack hasGutter>
              {groups.map((group) => (
                <StackItem key={group}>
                  <Title headingLevel="h4" size="md">
                    {group}
                  </Title>
                  <DescriptionList isCompact isHorizontal>
                    {detail.environments
                      .filter((env) => env.group === group)
                      .map((env) => (
                        <DescriptionListGroup key={env.instance}>
                          <DescriptionListTerm>
                            <Button
                              aria-label={`Open ${env.instance} on Fleet Map`}
                              component="a"
                              href="#"
                              isInline
                              variant="link"
                            >
                              {env.instance}
                            </Button>
                          </DescriptionListTerm>
                          <DescriptionListDescription>
                            <StageStatus cell={env.cell} />
                            {env.history ? (
                              <PromotionHistory attempts={env.history} />
                            ) : null}
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                      ))}
                  </DescriptionList>
                </StackItem>
              ))}
            </Stack>
          </StackItem>
          <StackItem>
            {/* FLT-DLV-11: the first bundle on the Fleet Map, and its Shipped
                row in Agent SDLC (shown because this bundle is locked). */}
            <ActionList isVertical>
              <ActionListItem>
                <Button component="a" href="#" variant="secondary">
                  Show bundle on Fleet Map
                </Button>
              </ActionListItem>
              <ActionListItem>
                <Button component="a" href="#" variant="secondary">
                  Show bundle in Agent SDLC
                </Button>
              </ActionListItem>
            </ActionList>
          </StackItem>
        </Stack>
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

function PageHeader({ asOf }: { asOf?: string }) {
  return (
    <Flex
      alignItems={{ default: "alignItemsFlexStart" }}
      justifyContent={{ default: "justifyContentSpaceBetween" }}
    >
      <FlexItem>
        <Content>
          <Title headingLevel="h1" size="2xl">
            Delivery
          </Title>
          <Content component="p">
            Follow each change on main from merge, through build and release, to
            every environment.
          </Content>
          {asOf ? (
            <Content className="pf-v6-u-text-color-subtle" component="small">
              Data as of {asOf}
            </Content>
          ) : null}
        </Content>
      </FlexItem>
      <FlexItem>
        <Button aria-label="Refresh" icon={<SyncAltIcon />} variant="plain" />
      </FlexItem>
    </Flex>
  );
}

function LoadedBody({
  fixture,
  selectedSha,
}: {
  fixture: DeliveryFixture;
  selectedSha?: string;
}) {
  return (
    <Stack hasGutter>
      {fixture.sourceAlert ? (
        <StackItem>
          <Alert isInline title={fixture.sourceAlert.title} variant="warning">
            {fixture.sourceAlert.body}
          </Alert>
        </StackItem>
      ) : null}
      <StackItem>
        <NowAndNext fixture={fixture} />
      </StackItem>
      <StackItem>
        <ChangeTable changes={fixture.changes} selectedSha={selectedSha} />
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
      return (
        <Drawer isExpanded isInline>
          <DrawerContent
            panelContent={<ChangeDetails detail={SELECTED_DETAIL} />}
          >
            <DrawerContentBody>
              <LoadedBody
                fixture={FIXTURES["change-selected"]}
                selectedSha={SELECTED_SHA}
              />
            </DrawerContentBody>
          </DrawerContent>
        </Drawer>
      );
    case "promotion-retried":
      return (
        <Drawer isExpanded isInline>
          <DrawerContent
            panelContent={<ChangeDetails detail={RETRIED_DETAIL} />}
          >
            <DrawerContentBody>
              <LoadedBody
                fixture={FIXTURES["promotion-retried"]}
                selectedSha={RETRIED_SHA}
              />
            </DrawerContentBody>
          </DrawerContent>
        </Drawer>
      );
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
        <PageHeader asOf={asOf} />
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
