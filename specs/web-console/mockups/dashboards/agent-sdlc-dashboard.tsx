import {
  Button,
  Card,
  CardBody,
  CardTitle,
  Content,
  Label,
  PageSection,
  Title,
} from "@patternfly/react-core";
import ArrowRightIcon from "@patternfly/react-icons/dist/esm/icons/arrow-right-icon";
import CheckCircleIcon from "@patternfly/react-icons/dist/esm/icons/check-circle-icon";
import ClockIcon from "@patternfly/react-icons/dist/esm/icons/clock-icon";
import ExclamationCircleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-circle-icon";
import ExclamationTriangleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-triangle-icon";
import QuestionCircleIcon from "@patternfly/react-icons/dist/esm/icons/question-circle-icon";

import { MockupShell } from "../shell/mockup-template";

// Illustrative design mockup for the Agent SDLC visibility tab
// (agent-sdlc-visibility-dashboard.spec). This is a design spike, NOT a visual
// spec: the production page defers to PatternFly 6 and the fleet-dashboard-ui
// conventions. It communicates the information architecture - aging board, rework
// meter, escalation lane, and release-by-hub-cluster matrix.

const SUBTLE = "#6a6e73";
const BORDER = "#d2d2d2";
const INFO = "#0066cc";
const WARN = "#dca614";
const DANGER = "#b1380b";
const OK = "#3d7317";

type MockCard = {
  readonly ref: string;
  readonly title: string;
  readonly age: string;
  /** 0..1 position down the aging track. */
  readonly aging: number;
  readonly rework?: { readonly count: number; readonly threshold: number };
};

const COLUMNS: readonly {
  readonly key: string;
  readonly label: string;
  readonly sle: number;
  readonly cards: readonly MockCard[];
}[] = [
  {
    key: "intake",
    label: "Intake",
    sle: 0.4,
    cards: [
      { ref: "#612", title: "Trace sampling knob", age: "4h", aging: 0.15 },
    ],
  },
  {
    key: "workable",
    label: "Workable",
    sle: 0.5,
    cards: [
      { ref: "#618", title: "Gateway drain timeout", age: "9h", aging: 0.35 },
    ],
  },
  {
    key: "spec",
    label: "Spec",
    sle: 0.6,
    cards: [
      {
        ref: "#565",
        title: "Agent SDLC tab",
        age: "1d",
        aging: 0.3,
        rework: { count: 1, threshold: 3 },
      },
      {
        ref: "#559",
        title: "Funnel auth gate",
        age: "2d",
        aging: 0.55,
        rework: { count: 2, threshold: 3 },
      },
    ],
  },
  {
    key: "spec-approved",
    label: "Spec approved",
    sle: 0.4,
    cards: [
      { ref: "#525", title: "Declarative import", age: "6h", aging: 0.2 },
    ],
  },
  {
    key: "code",
    label: "Code",
    sle: 0.65,
    cards: [
      {
        ref: "#558",
        title: "Sync-retry count",
        age: "3d",
        aging: 0.5,
        rework: { count: 4, threshold: 10 },
      },
    ],
  },
  {
    key: "code-approved",
    label: "Code approved",
    sle: 0.35,
    cards: [
      { ref: "#555", title: "Gate pane visibility", age: "5h", aging: 0.18 },
    ],
  },
];

const HUBS = ["Prod hub", "Stage hub", "Int hub"] as const;

type MatrixRow = {
  readonly version: string;
  readonly unreleased?: boolean;
  readonly current: readonly boolean[];
  readonly issues: number;
};

const MATRIX: readonly MatrixRow[] = [
  {
    version: "Unreleased",
    unreleased: true,
    current: [false, false, false],
    issues: 3,
  },
  { version: "v20260915", current: [true, false, false], issues: 5 },
  { version: "v20260901", current: [false, true, false], issues: 4 },
  { version: "v20260815", current: [false, false, true], issues: 6 },
];

function ReworkBar({ count, threshold }: { count: number; threshold: number }) {
  const ratio = count / threshold;
  const tone = count >= threshold - 1 ? DANGER : ratio >= 2 / 3 ? WARN : INFO;
  return (
    <div style={{ marginTop: 4 }}>
      <div
        style={{
          height: 6,
          borderRadius: 3,
          background: "#f2f2f2",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            width: `${String(ratio * 100)}%`,
            height: "100%",
            background: tone,
          }}
        />
      </div>
      <Content component="small">
        <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
          {count >= threshold - 1 ? (
            <ExclamationTriangleIcon style={{ color: DANGER }} />
          ) : null}
          {`${String(count)} of ${String(threshold)} rejections`}
        </span>
      </Content>
    </div>
  );
}

function AgingColumn({ column }: { column: (typeof COLUMNS)[number] }) {
  return (
    <div style={{ flex: "1 0 180px", minWidth: 180 }}>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          fontWeight: 700,
          marginBottom: 4,
        }}
      >
        <span>{column.label}</span>
        <Label isCompact>{column.cards.length}</Label>
      </div>
      <div
        style={{
          position: "relative",
          minHeight: 260,
          border: `1px solid ${BORDER}`,
          borderRadius: 6,
          padding: 4,
        }}
      >
        <div
          style={{
            position: "absolute",
            left: 0,
            right: 0,
            top: `${String(column.sle * 90)}%`,
            borderTop: `2px dashed ${WARN}`,
          }}
        />
        <span
          style={{
            position: "absolute",
            right: 4,
            top: `${String(column.sle * 90)}%`,
            transform: "translateY(-50%)",
            fontSize: 10,
            color: SUBTLE,
          }}
        >
          SLE
        </span>
        {column.cards.map((card) => (
          <div
            key={card.ref}
            style={{
              position: "absolute",
              left: 4,
              right: 4,
              top: `${String(card.aging * 90)}%`,
              border: `1px solid ${BORDER}`,
              borderRadius: 6,
              padding: 6,
              background: "#ffffff",
            }}
          >
            <div style={{ fontWeight: 600, fontSize: 12 }}>
              {`${card.ref} ${card.title}`}
            </div>
            <Content component="small">
              <span
                style={{ display: "inline-flex", alignItems: "center", gap: 4 }}
              >
                <ClockIcon />
                {`In phase ${card.age}`}
              </span>
            </Content>
            {card.rework ? (
              <ReworkBar
                count={card.rework.count}
                threshold={card.rework.threshold}
              />
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}

export function AgentSdlcDashboardPageSurface() {
  return (
    <>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            Agent SDLC
          </Title>
          <p>
            Where every execution record sits in the label-driven funnel, how
            long it has waited, what is stalled, and which issue shipped in
            which bundle onto which hub cluster.
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <Card>
          <CardTitle>Aging work-in-progress</CardTitle>
          <CardBody>
            <div style={{ display: "flex", gap: 8, overflowX: "auto" }}>
              {COLUMNS.map((column) => (
                <AgingColumn key={column.key} column={column} />
              ))}
            </div>
          </CardBody>
        </Card>

        <Card style={{ marginTop: 12 }}>
          <CardTitle>Escalation lane</CardTitle>
          <CardBody>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
              <div
                style={{
                  flex: "1 1 260px",
                  border: `1px solid ${BORDER}`,
                  borderRadius: 6,
                  padding: 8,
                }}
              >
                <Label
                  isCompact
                  icon={<ExclamationCircleIcon />}
                  style={{ borderColor: DANGER }}
                >
                  Blocked
                </Label>
                <div style={{ fontWeight: 600, fontSize: 12, marginTop: 4 }}>
                  #540 Keycloak realm drift
                </div>
              </div>
              <div
                style={{
                  flex: "1 1 260px",
                  border: `1px solid ${BORDER}`,
                  borderRadius: 6,
                  padding: 8,
                }}
              >
                <Label
                  isCompact
                  icon={<QuestionCircleIcon />}
                  style={{ borderColor: INFO }}
                >
                  Needs input
                </Label>
                <div style={{ fontWeight: 600, fontSize: 12, marginTop: 4 }}>
                  #533 Unclear SLO target
                </div>
              </div>
              <div
                style={{
                  flex: "1 1 260px",
                  border: `1px solid ${BORDER}`,
                  borderRadius: 6,
                  padding: 8,
                }}
              >
                <Label
                  isCompact
                  icon={<ClockIcon />}
                  style={{ borderColor: WARN }}
                >
                  Past SLE
                </Label>
                <div style={{ fontWeight: 600, fontSize: 12, marginTop: 4 }}>
                  #501 Mesh handshake retry
                </div>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card style={{ marginTop: 12 }}>
          <CardTitle>Shipped - release by hub cluster</CardTitle>
          <CardBody>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: `minmax(120px, 1fr) repeat(${String(HUBS.length)}, minmax(90px, 1fr)) 80px`,
                gap: 4,
                alignItems: "center",
              }}
            >
              <div style={{ fontWeight: 700, color: SUBTLE }}>Bundle</div>
              {HUBS.map((hub) => (
                <div key={hub} style={{ fontWeight: 700, color: SUBTLE }}>
                  {hub}
                </div>
              ))}
              <div style={{ fontWeight: 700, color: SUBTLE }}>Issues</div>
              {MATRIX.map((row) => (
                <MatrixRowCells key={row.version} row={row} />
              ))}
            </div>
            <Content component="small">
              <span
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 4,
                  marginTop: 8,
                }}
              >
                <ArrowRightIcon />
                Selecting a bundle opens it on the Fleet Map (shared freight
                bar).
              </span>
            </Content>
          </CardBody>
        </Card>
      </PageSection>
    </>
  );
}

function MatrixRowCells({ row }: { row: MatrixRow }) {
  return (
    <>
      <div>
        {row.unreleased ? (
          <span style={{ color: SUBTLE }}>Unreleased</span>
        ) : (
          <Button
            variant="link"
            isInline
            icon={<ArrowRightIcon />}
            iconPosition="end"
          >
            {row.version}
          </Button>
        )}
      </div>
      {row.current.map((current, i) => (
        <div key={HUBS[i]}>
          {current ? (
            <span
              style={{ display: "inline-flex", alignItems: "center", gap: 4 }}
            >
              <CheckCircleIcon style={{ color: OK }} />
              current
            </span>
          ) : (
            <span style={{ color: BORDER }}>-</span>
          )}
        </div>
      ))}
      <div>
        <Label isCompact>{row.issues}</Label>
      </div>
    </>
  );
}

export function AgentSdlcDashboardMockup() {
  return (
    <MockupShell>
      <AgentSdlcDashboardPageSurface />
    </MockupShell>
  );
}
