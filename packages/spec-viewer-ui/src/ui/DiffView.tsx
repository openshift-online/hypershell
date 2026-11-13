import {
  Flex,
  Title,
  ToggleGroup,
  ToggleGroupItem,
} from "@patternfly/react-core";
import { useMemo, useState } from "react";
import { useIntl } from "react-intl";

import {
  diffBlocks,
  diffLines,
  diffWords,
  type BlockRow,
  type DiffLine,
  type DiffRow,
  type DiffSegment,
} from "../domain/text-diff";
import { messages } from "../messages";
import { MarkdownDiffView } from "./MarkdownDiffView";
import { usePorts } from "./ports-context";

interface DiffViewProps {
  /** File path of the changed item. */
  path: string;
  /** Title of the changed item. */
  title: string;
  /** Markdown in the branch being compared against. */
  before: string;
  /** Markdown in the current branch. */
  after: string;
  /** Names of the two branches, for the column headers. */
  beforeLabel: string;
  afterLabel: string;
}

interface CellProps {
  line: DiffLine | null;
  /** Which side this cell is on, and so which color a change gets. */
  side: "before" | "after";
  /** True when the line differs from the other side. */
  changed: boolean;
}

/** The mark in the gutter of a removed or added line. */
const SIGN = { before: "\u2212", after: "+" } as const;

function Cell({ line, side, changed }: CellProps) {
  const intl = useIntl();
  const kind = changed ? (side === "before" ? "removed" : "added") : "plain";
  const empty = line === null;
  return (
    <>
      <td
        aria-hidden="true"
        className={`sv-diff__sign sv-diff__cell--${empty ? "empty" : kind}`}
      >
        {changed && line ? SIGN[side] : ""}
      </td>
      <td className={`sv-diff__text sv-diff__cell--${empty ? "empty" : kind}`}>
        {line ? (
          <>
            {changed ? (
              <span className="pf-v6-u-screen-reader">
                {intl.formatMessage(
                  side === "before"
                    ? messages.diffRemovedPrefix
                    : messages.diffAddedPrefix,
                )}
              </span>
            ) : null}
            {line.segments.map((segment, index) =>
              segment.changed ? (
                <span className="sv-diff__word" key={index}>
                  {segment.text}
                </span>
              ) : (
                segment.text
              ),
            )}
          </>
        ) : null}
      </td>
    </>
  );
}

function Row({ row }: { row: DiffRow }) {
  return (
    <tr className={`sv-diff__row sv-diff__row--${row.kind}`}>
      <Cell changed={row.kind !== "same"} line={row.before} side="before" />
      <Cell changed={row.kind !== "same"} line={row.after} side="after" />
    </tr>
  );
}

/**
 * Split view of one changed item: the branch compared against on the left,
 * the current branch on the right. Changed lines sit side by side; removed
 * text is red, added text green, with the changed words marked inside them.
 */
type ViewMode = "raw" | "rendered";

interface RenderedSegments {
  before: DiffSegment[];
  after: DiffSegment[];
}

/** One row of the rendered view: the markdown of a block on each side. */
function RenderedRow({ row }: { row: BlockRow }) {
  const { markdown } = usePorts();
  const segments = useMemo<RenderedSegments>(() => {
    const beforeText = row.before === null ? "" : markdown.text(row.before);
    const afterText = row.after === null ? "" : markdown.text(row.after);
    if (row.kind !== "changed") {
      return {
        before: beforeText ? [{ text: beforeText, changed: false }] : [],
        after: afterText ? [{ text: afterText, changed: false }] : [],
      };
    }
    const [before, after] = diffWords(beforeText, afterText);
    return { before, after };
  }, [markdown, row]);
  const changed = row.kind !== "same";
  const side = (source: string | null, which: "before" | "after") => {
    const kind =
      source === null
        ? "empty"
        : changed
          ? which === "before"
            ? "removed"
            : "added"
          : "plain";
    return (
      <>
        <td
          aria-hidden="true"
          className={`sv-diff__sign sv-diff__cell--${kind}`}
        >
          {changed && source !== null ? SIGN[which] : ""}
        </td>
        <td className={`sv-diff__rendered sv-diff__cell--${kind}`}>
          {source === null ? null : (
            <MarkdownDiffView segments={segments[which]} source={source} />
          )}
        </td>
      </>
    );
  };
  return (
    <tr className={`sv-diff__row sv-diff__row--${row.kind}`}>
      {side(row.before, "before")}
      {side(row.after, "after")}
    </tr>
  );
}

/**
 * Split view of one changed item: the branch compared against on the left,
 * the current branch on the right. Raw mode puts changed lines side by side
 * with the changed words marked; rendered mode shows each markdown block as
 * it reads, with changed blocks in red and green.
 */
export function DiffView({
  path,
  title,
  before,
  after,
  beforeLabel,
  afterLabel,
}: DiffViewProps) {
  const intl = useIntl();
  const [mode, setMode] = useState<ViewMode>("rendered");
  const rows = useMemo(() => diffLines(before, after), [before, after]);
  const blocks = useMemo(() => diffBlocks(before, after), [before, after]);
  return (
    <div className="sv-document">
      <div className="sv-document__header">
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <Title headingLevel="h2" size="md">
            {path}
          </Title>
          <ToggleGroup
            aria-label={intl.formatMessage(messages.diffViewMode)}
            isCompact
          >
            <ToggleGroupItem
              buttonId="sv-diff-raw"
              isSelected={mode === "raw"}
              onChange={() => {
                setMode("raw");
              }}
              text={intl.formatMessage(messages.diffViewRaw)}
            />
            <ToggleGroupItem
              buttonId="sv-diff-rendered"
              isSelected={mode === "rendered"}
              onChange={() => {
                setMode("rendered");
              }}
              text={intl.formatMessage(messages.diffViewRendered)}
            />
          </ToggleGroup>
        </Flex>
        <span className="sv-diff__title">{title}</span>
      </div>
      <div
        aria-label={intl.formatMessage(messages.diffLabel)}
        className="sv-document__body sv-diff"
        role="region"
        tabIndex={-1}
      >
        <table
          className={`sv-diff__table${mode === "rendered" ? " sv-diff__table--rendered" : ""}`}
        >
          <colgroup>
            <col className="sv-diff__col-sign" />
            <col className="sv-diff__col-text" />
            <col className="sv-diff__col-sign" />
            <col className="sv-diff__col-text" />
          </colgroup>
          <thead>
            <tr>
              <th colSpan={2} scope="col">
                {beforeLabel}
              </th>
              <th colSpan={2} scope="col">
                {afterLabel}
              </th>
            </tr>
          </thead>
          <tbody>
            {mode === "raw"
              ? rows.map((row, index) => <Row key={index} row={row} />)
              : blocks.map((row, index) => (
                  <RenderedRow key={index} row={row} />
                ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
