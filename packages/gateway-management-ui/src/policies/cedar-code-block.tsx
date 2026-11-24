import { CodeBlock, CodeBlockCode } from "@patternfly/react-core";
import { useMemo, type ReactNode } from "react";

import styles from "./cedar-code-block.module.css";
import { tokenizeCedar, type CedarTokenKind } from "./cedar-highlight";

const tokenClass: Record<CedarTokenKind, string | undefined> = {
  annotation: styles.annotation,
  comment: styles.comment,
  keyword: styles.keyword,
  number: styles.number,
  operator: undefined,
  plain: undefined,
  string: styles.string,
  type: styles.type,
  variable: styles.variable,
};

/** Syntax-highlighted Cedar source as React elements. */
export function CedarSource({ source }: { source: string }): ReactNode {
  const tokens = useMemo(() => tokenizeCedar(source), [source]);
  return tokens.map((token, index) => {
    const className = tokenClass[token.kind];
    return className ? (
      <span className={className} key={index}>
        {token.value}
      </span>
    ) : (
      token.value
    );
  });
}

/** A read-only PatternFly code block with syntax-highlighted Cedar. */
export function CedarCodeBlock({
  actions,
  id,
  source,
}: {
  actions?: ReactNode;
  id?: string;
  source: string;
}) {
  return (
    <CodeBlock actions={actions}>
      <CodeBlockCode id={id}>
        <CedarSource source={source} />
      </CodeBlockCode>
    </CodeBlock>
  );
}
