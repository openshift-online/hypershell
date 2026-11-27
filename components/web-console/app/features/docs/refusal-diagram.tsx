import { useId } from "react";
import { useIntl } from "react-intl";

import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";

/**
 * Three-node diagram for the refusal page (WEB-UDOC-09): your request, the
 * platform rule check, and sandbox created or refused. It is one accessible
 * image with a title and description, and its outcomes differ by text, line
 * style, and a check or cross mark, not by color alone.
 */
export function RefusalDiagram() {
  const intl = useIntl();
  const id = useId();
  const titleId = `${id}-title`;
  const descId = `${id}-desc`;
  return (
    <svg
      aria-describedby={descId}
      aria-labelledby={titleId}
      className={styles.diagram}
      role="img"
      viewBox="0 0 560 200"
    >
      <title id={titleId}>
        {intl.formatMessage(docsMessages.refusalDiagramTitle)}
      </title>
      <desc id={descId}>
        {intl.formatMessage(docsMessages.refusalDiagramDescription)}
      </desc>
      <defs>
        <marker
          id={`${id}-arrow`}
          markerHeight="8"
          markerWidth="8"
          orient="auto"
          refX="7"
          refY="4"
        >
          <path className={styles.diagramArrowHead} d="M0,0 L8,4 L0,8 z" />
        </marker>
      </defs>
      <g aria-hidden="true">
        <rect
          className={styles.diagramNode}
          height="56"
          rx="8"
          width="140"
          x="8"
          y="72"
        />
        <text className={styles.diagramText} textAnchor="middle" x="78" y="105">
          {intl.formatMessage(docsMessages.refusalDiagramRequest)}
        </text>
        <path
          className={styles.diagramArrow}
          d="M148,100 L196,100"
          markerEnd={`url(#${id}-arrow)`}
        />
        <polygon
          className={styles.diagramNode}
          points="276,52 356,100 276,148 196,100"
        />
        <text
          className={styles.diagramText}
          textAnchor="middle"
          x="276"
          y="104"
        >
          {intl.formatMessage(docsMessages.refusalDiagramCheck)}
        </text>
        <path
          className={styles.diagramArrow}
          d="M356,100 L380,100 L380,44 L398,44"
          markerEnd={`url(#${id}-arrow)`}
        />
        <path
          className={styles.diagramArrow}
          d="M356,100 L380,100 L380,156 L398,156"
          markerEnd={`url(#${id}-arrow)`}
        />
        <rect
          className={styles.diagramAllowed}
          height="48"
          rx="8"
          width="152"
          x="400"
          y="20"
        />
        <path
          className={[styles.diagramMark, styles.diagramMarkAllowed].join(" ")}
          d="M408,44 L413,50 L422,38"
        />
        <text className={styles.diagramText} textAnchor="middle" x="486" y="49">
          {intl.formatMessage(docsMessages.refusalDiagramCreated)}
        </text>
        <rect
          className={styles.diagramRefused}
          height="48"
          rx="8"
          width="152"
          x="400"
          y="132"
        />
        <path
          className={[styles.diagramMark, styles.diagramMarkRefused].join(" ")}
          d="M409,151 L419,161 M419,151 L409,161"
        />
        <text
          className={styles.diagramText}
          textAnchor="middle"
          x="486"
          y="161"
        >
          {intl.formatMessage(docsMessages.refusalDiagramRefused)}
        </text>
      </g>
    </svg>
  );
}
