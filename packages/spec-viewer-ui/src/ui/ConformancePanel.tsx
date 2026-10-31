import {
  Alert,
  Button,
  Content,
  Flex,
  FlexItem,
  Label,
  LabelGroup,
  Progress,
  ProgressMeasureLocation,
  ProgressSize,
  ProgressVariant,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  FileAltIcon,
  FolderIcon,
  SyncAltIcon,
} from "@patternfly/react-icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  TreeRowWrapper,
  type TdProps,
} from "@patternfly/react-table";
import { useMemo, useState } from "react";
import { useIntl, type IntlShape } from "react-intl";

import {
  buildConformanceReport,
  conformancePercent,
  type ConformanceCounts,
  type FileReport,
  type FolderReport,
  type RequirementIssue,
  type RequirementReport,
  type ScenarioIssue,
  type ScenarioReport,
} from "../domain/conformance";
import { fileEntryId, type SpecCatalog } from "../domain/spec-document";
import { messages } from "../messages";
import { buildTraceability, type TestCitation } from "../domain/traceability";
import { NodeLabels } from "./NodeLabels";
import { usePorts } from "./ports-context";
import type { SourceLocation } from "./ReferencesPanel";
import { useSpecRef } from "./ref-context";
import { TRACEABILITY_KEY } from "./use-spec-catalog";
import { nodeTitle } from "./SpecTreeView";

interface ConformancePanelProps {
  catalog: SpecCatalog;
  selectedNodeId: string | undefined;
  onSelect: (nodeId: string) => void;
  /** Opens a test file at the line of its `Verifies:` marker. */
  onOpenSource: (location: SourceLocation, id: string) => void;
}

/** At most this many test labels per scenario; the rest are counted. */
const MAX_CITATION_LABELS = 3;

const REQUIREMENT_ISSUES = {
  "no-scenarios": messages.issueNoScenarios,
  "malformed-scenarios": messages.issueMalformedScenarios,
  "no-normative": messages.issueNoNormative,
  "missing-id": messages.issueMissingId,
  "scenarios-missing-id": messages.issueScenariosMissingId,
  "untested-scenarios": messages.issueUntestedScenarios,
} satisfies Record<RequirementIssue, unknown>;

const SCENARIO_ISSUES = {
  "missing-when": messages.issueMissingWhen,
  "missing-then": messages.issueMissingThen,
  "then-before-when": messages.issueThenBeforeWhen,
  "missing-id": messages.issueMissingId,
  untested: messages.issueUntested,
} satisfies Record<ScenarioIssue, unknown>;

/** Count columns as [short header, full name shown in its tooltip]. */
const COUNT_COLUMNS = [
  [messages.columnRequirementsShort, messages.columnRequirements],
  [messages.columnScenariosShort, messages.columnScenarios],
  [messages.columnWellFormedShort, messages.columnWellFormed],
  [messages.columnMissingIdShort, messages.columnMissingId],
  [messages.columnScenarioMissingIdShort, messages.columnScenarioMissingId],
  [messages.columnTestedShort, messages.columnTested],
] as const;

/** Bar color: green from 80%, amber from 50%, red below. */
function progressVariant(percent: number): ProgressVariant {
  if (percent >= 80) return ProgressVariant.success;
  if (percent >= 50) return ProgressVariant.warning;
  return ProgressVariant.danger;
}

interface Row {
  key: string;
  name: React.ReactNode;
  /** Node to open when the row is selected. */
  nodeId?: string;
  cells: [
    React.ReactNode,
    React.ReactNode,
    React.ReactNode,
    React.ReactNode,
    React.ReactNode,
    React.ReactNode,
  ];
  conformance: React.ReactNode;
  children: Row[];
}

function percentCell(
  intl: IntlShape,
  label: string,
  counts: ConformanceCounts,
): React.ReactNode {
  const percent = conformancePercent(counts);
  if (percent === null) return null;
  return (
    <span className="sv-conformance__percent">
      <Progress
        aria-label={intl.formatMessage(messages.conformanceOf, { name: label })}
        className="sv-conformance__bar"
        measureLocation={ProgressMeasureLocation.none}
        size={ProgressSize.sm}
        value={percent}
        variant={progressVariant(percent)}
      />
      <span className="sv-conformance__value">
        {intl.formatNumber(percent / 100, { style: "percent" })}
      </span>
    </span>
  );
}

function countCells(counts: ConformanceCounts): Row["cells"] {
  return [
    counts.requirements,
    counts.scenarios,
    counts.wellFormedScenarios,
    counts.missingId,
    counts.scenariosMissingId,
    counts.testedScenarios,
  ];
}

type OpenSource = ConformancePanelProps["onOpenSource"];

function citationName(citation: TestCitation): string {
  return (
    citation.test ||
    `${citation.path.split("/").pop() ?? citation.path}:${String(citation.line)}`
  );
}

/** Tests citing a scenario, each opening the test at its marker. */
function citationLabels(
  intl: IntlShape,
  scenario: ScenarioReport,
  openSource: OpenSource,
): React.ReactNode {
  const id = scenario.node.specId;
  if (!id || scenario.citations.length === 0) return null;
  return (
    <LabelGroup
      collapsedText={intl.formatMessage(messages.citationsMore, {
        remaining: "${remaining}",
      })}
      isCompact
      numLabels={MAX_CITATION_LABELS}
    >
      {scenario.citations.map((citation) => {
        const location = `${citation.path}:${String(citation.line)}`;
        return (
          <Label
            color="green"
            icon={<CheckCircleIcon />}
            isCompact
            key={location}
            onClick={() => {
              openSource({ path: citation.path, line: citation.line }, id);
            }}
            title={location}
            variant="outline"
          >
            {citationName(citation)}
          </Label>
        );
      })}
    </LabelGroup>
  );
}

function issueLabels(
  intl: IntlShape,
  issues: string[],
  names: Record<string, (typeof messages)[keyof typeof messages]>,
): React.ReactNode {
  if (issues.length === 0) {
    return (
      <Label color="green" isCompact>
        {intl.formatMessage(messages.issueNone)}
      </Label>
    );
  }
  return issues.map((issue) => {
    const message = names[issue];
    return (
      <Label color="orange" isCompact key={issue}>
        {message ? intl.formatMessage(message) : issue}
      </Label>
    );
  });
}

function requirementRow(
  intl: IntlShape,
  report: RequirementReport,
  openSource: OpenSource,
): Row {
  const { node, scenarios, issues, nonTestMethod } = report;
  const wellFormed = scenarios.filter((s) =>
    s.issues.every((issue) => issue === "missing-id" || issue === "untested"),
  ).length;
  return {
    key: node.id,
    nodeId: node.id,
    name: (
      <span className="sv-conformance__name">
        <NodeLabels node={node} />
        <span>{nodeTitle(intl, node)}</span>
      </span>
    ),
    cells: [
      null,
      scenarios.length,
      wellFormed,
      node.specId ? 0 : 1,
      scenarios.filter((s) => !s.node.specId).length,
      scenarios.filter((s) => s.citations.length > 0).length,
    ],
    conformance: (
      <span className="sv-conformance__issues">
        {nonTestMethod ? (
          <Label color="grey" isCompact>
            {intl.formatMessage(messages.verifiedBy, {
              method: nonTestMethod,
            })}
          </Label>
        ) : null}
        {issueLabels(intl, issues, REQUIREMENT_ISSUES)}
      </span>
    ),
    children: scenarios.map((scenario) => ({
      key: scenario.node.id,
      nodeId: scenario.node.id,
      name: (
        <span className="sv-conformance__name">
          <NodeLabels node={scenario.node} />
          <span>{scenario.node.title}</span>
        </span>
      ),
      cells: [null, null, null, null, null, null],
      conformance: (
        <span className="sv-conformance__issues">
          {issueLabels(intl, scenario.issues, SCENARIO_ISSUES)}
          {citationLabels(intl, scenario, openSource)}
        </span>
      ),
      children: [],
    })),
  };
}

function fileRow(
  intl: IntlShape,
  report: FileReport,
  openSource: OpenSource,
): Row {
  return {
    key: `file:${report.file.path}`,
    nodeId: fileEntryId(report.file),
    name: (
      <span className="sv-conformance__name">
        <FileAltIcon />
        <span>
          {report.file.title}
          <span className="sv-node__path">{report.file.path}</span>
        </span>
      </span>
    ),
    cells: countCells(report.counts),
    conformance: percentCell(intl, report.file.title, report.counts),
    children: report.requirements.map((r) =>
      requirementRow(intl, r, openSource),
    ),
  };
}

function folderRows(
  intl: IntlShape,
  folder: FolderReport,
  openSource: OpenSource,
): Row[] {
  return [
    ...folder.folders.map((child): Row => ({
      key: `folder:${child.path}`,
      name: (
        <span className="sv-conformance__name">
          <FolderIcon />
          <span>{child.path}</span>
        </span>
      ),
      cells: countCells(child.counts),
      conformance: percentCell(intl, child.path, child.counts),
      children: folderRows(intl, child, openSource),
    })),
    ...folder.files.map((file) => fileRow(intl, file, openSource)),
  ];
}

export function ConformancePanel({
  catalog,
  selectedNodeId,
  onSelect,
  onOpenSource,
}: ConformancePanelProps) {
  const intl = useIntl();
  const { repository } = usePorts();
  const queryClient = useQueryClient();
  const ref = useSpecRef();
  // The marker scan reads the whole repository; until it answers, or if it
  // fails, the test check is left out rather than guessed.
  const traceability = useQuery({
    queryKey: [...TRACEABILITY_KEY, ref],
    queryFn: () => repository.loadMarkers(ref),
    staleTime: Infinity,
  });
  const report = useMemo(
    () =>
      buildConformanceReport(
        catalog,
        traceability.data
          ? buildTraceability(catalog.files, traceability.data)
          : undefined,
      ),
    [catalog, traceability.data],
  );
  const rows = useMemo(
    () => folderRows(intl, report.root, onOpenSource),
    [intl, report, onOpenSource],
  );
  // Folders start expanded so every file is visible; files and
  // requirements start collapsed.
  const [expanded, setExpanded] = useState<Set<string>>(
    () =>
      new Set(
        rows.flatMap(function folderKeys(row): string[] {
          return row.key.startsWith("folder:")
            ? [row.key, ...row.children.flatMap(folderKeys)]
            : [];
        }),
      ),
  );

  const toggle = (key: string) => {
    setExpanded((current) => {
      const next = new Set(current);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  };

  // Row position among the rendered rows; PatternFly names the expand
  // toggles after it.
  let rowIndex = 0;
  const renderRows = (list: Row[], level: number): React.ReactElement[] =>
    list.flatMap((row, index) => {
      const isExpanded = expanded.has(row.key);
      rowIndex += 1;
      const treeRow: TdProps["treeRow"] = {
        rowIndex,
        onCollapse: () => {
          toggle(row.key);
        },
        props: {
          isExpanded,
          "aria-level": level,
          "aria-posinset": index + 1,
          "aria-setsize": row.children.length,
        },
      };
      const { nodeId } = row;
      const element = (
        <TreeRowWrapper
          className={
            nodeId !== undefined && nodeId === selectedNodeId
              ? "sv-conformance__row--selected"
              : undefined
          }
          key={row.key}
          row={{ props: treeRow.props as Record<string, unknown> }}
        >
          <Td
            dataLabel={intl.formatMessage(messages.columnName)}
            treeRow={treeRow}
          >
            {nodeId ? (
              <Button
                className="sv-conformance__open"
                isInline
                onClick={() => {
                  onSelect(nodeId);
                }}
                variant="link"
              >
                {row.name}
              </Button>
            ) : (
              row.name
            )}
          </Td>
          {row.cells.map((cell, cellIndex) => (
            <Td className="sv-conformance__count" key={cellIndex}>
              {cell}
            </Td>
          ))}
          <Td dataLabel={intl.formatMessage(messages.columnConformance)}>
            {row.conformance}
          </Td>
        </TreeRowWrapper>
      );
      return isExpanded
        ? [element, ...renderRows(row.children, level + 1)]
        : [element];
    });

  const total = report.root.counts;
  const metrics: [string, React.ReactNode][] = [
    [
      intl.formatMessage(messages.columnConformance),
      intl.formatNumber((conformancePercent(total) ?? 0) / 100, {
        style: "percent",
      }),
    ],
    [intl.formatMessage(messages.columnRequirements), total.requirements],
    [intl.formatMessage(messages.metricWithScenarios), total.withScenarios],
    [
      intl.formatMessage(messages.columnWellFormed),
      `${String(total.wellFormedScenarios)} / ${String(total.scenarios)}`,
    ],
    [intl.formatMessage(messages.columnMissingId), total.missingId],
    [
      intl.formatMessage(messages.columnScenarioMissingId),
      total.scenariosMissingId,
    ],
    [
      intl.formatMessage(messages.metricTested),
      traceability.data
        ? `${String(total.testedScenarios)} / ${String(total.scenarios)}`
        : "",
    ],
  ];

  return (
    <div className="sv-results sv-conformance">
      <Flex className="sv-conformance__metrics" gap={{ default: "gapSm" }}>
        {metrics.map(([label, value]) => (
          <FlexItem className="sv-metric" key={label}>
            <span className="sv-metric__label">{label}</span>
            <span className="sv-metric__value">{value}</span>
          </FlexItem>
        ))}
        <FlexItem align={{ default: "alignRight" }}>
          <Button
            icon={<SyncAltIcon />}
            isDisabled={traceability.isFetching}
            isInline
            onClick={() => {
              void queryClient.invalidateQueries({
                queryKey: [...TRACEABILITY_KEY, ref],
              });
            }}
            variant="link"
          >
            {intl.formatMessage(
              traceability.isFetching
                ? messages.testMarkersLoading
                : messages.testMarkersReload,
            )}
          </Button>
        </FlexItem>
      </Flex>
      {traceability.isError ? (
        <Alert
          isInline
          isPlain
          title={intl.formatMessage(messages.testMarkersError)}
          variant="warning"
        >
          {traceability.error.message}
        </Alert>
      ) : null}
      <div className="sv-results__body">
        <Table
          aria-label={intl.formatMessage(messages.conformanceLabel)}
          isTreeTable
          variant="compact"
        >
          <Thead>
            <Tr>
              <Th>{intl.formatMessage(messages.columnName)}</Th>
              {COUNT_COLUMNS.map(([short, full]) => (
                <Th
                  className="sv-conformance__count"
                  key={full.id}
                  modifier="fitContent"
                  tooltip={intl.formatMessage(full)}
                >
                  {intl.formatMessage(short)}
                </Th>
              ))}
              <Th modifier="fitContent">
                {intl.formatMessage(messages.columnConformance)}
              </Th>
            </Tr>
          </Thead>
          <Tbody>{renderRows(rows, 1)}</Tbody>
        </Table>
        {report.filesWithoutRequirements.length > 0 ? (
          <Content className="sv-conformance__none" component="p">
            {intl.formatMessage(messages.filesWithoutRequirements, {
              count: report.filesWithoutRequirements.length,
              files: report.filesWithoutRequirements
                .map((file) => file.path)
                .join(", "),
            })}
          </Content>
        ) : null}
      </div>
    </div>
  );
}
