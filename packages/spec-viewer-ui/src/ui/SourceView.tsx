import { Alert, Bullseye, Label, Spinner, Title } from "@patternfly/react-core";
import { useQuery } from "@tanstack/react-query";
import {
  Fragment,
  useEffect,
  useMemo,
  useRef,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useIntl } from "react-intl";

import { sourceLanguage } from "../domain/references";
import type { SourceToken } from "../domain/source-highlight";
import { messages } from "../messages";
import { MATCH_CLASS } from "./highlight-hast";
import { usePorts } from "./ports-context";
import { useSpecRef } from "./ref-context";
import type { SourceLocation } from "./ReferencesPanel";
import { FLASH_CLASS } from "./SpecDocument";

interface SourceViewProps {
  location: SourceLocation;
  /** Changes on every open, so reopening the same line flashes it again. */
  revision: number;
  /** ID to mark wherever it appears in the file. */
  id: string | undefined;
}

function tokenStyle(token: SourceToken): CSSProperties {
  return {
    color: token.lightColor,
    fontWeight: token.bold ? 700 : undefined,
    fontStyle: token.italic ? "italic" : undefined,
    textDecoration:
      token.underline || token.strikethrough
        ? [
            token.underline ? "underline" : null,
            token.strikethrough ? "line-through" : null,
          ]
            .filter(Boolean)
            .join(" ")
        : undefined,
    "--shiki-dark": token.darkColor,
  } as CSSProperties;
}

function rootStyle(highlight: {
  foreground?: string;
  darkForeground?: string;
  background?: string;
  darkBackground?: string;
}): CSSProperties {
  return {
    color: highlight.foreground,
    backgroundColor: highlight.background,
    "--shiki-dark": highlight.darkForeground,
    "--shiki-dark-bg": highlight.darkBackground,
  } as CSSProperties;
}

function tokenChildren(
  token: SourceToken,
  pattern: RegExp | undefined,
): ReactNode {
  if (!pattern) return token.text;
  const parts: ReactNode[] = [];
  let last = 0;
  for (const match of token.text.matchAll(pattern)) {
    parts.push(token.text.slice(last, match.index));
    parts.push(
      <mark className={MATCH_CLASS} key={parts.length}>
        {match[0]}
      </mark>,
    );
    last = match.index + match[0].length;
  }
  if (parts.length === 0) return token.text;
  parts.push(token.text.slice(last));
  return parts;
}

/** A repository file with syntax coloring, scrolled to the selected line. */
export function SourceView({ location, revision, id }: SourceViewProps) {
  const intl = useIntl();
  const { repository, highlighter } = usePorts();
  const ref = useSpecRef();
  const scroller = useRef<HTMLDivElement>(null);
  const language = sourceLanguage(location.path);

  const source = useQuery({
    queryKey: ["source", ref, location.path],
    queryFn: async () =>
      highlighter.highlight(
        await repository.readFile(ref, location.path),
        language,
      ),
  });
  const pattern = useMemo(
    () =>
      id
        ? new RegExp(id.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&"), "giu")
        : undefined,
    [id],
  );

  useEffect(() => {
    const container = scroller.current;
    if (!container || source.isPending) return;
    const line =
      container.querySelectorAll<HTMLElement>(".line")[location.line - 1];
    if (!line) return;
    line.scrollIntoView({ block: "center" });
    line.classList.remove(FLASH_CLASS);
    void line.offsetWidth;
    line.classList.add(FLASH_CLASS);
    const clear = () => {
      line.classList.remove(FLASH_CLASS);
    };
    line.addEventListener("animationend", clear, { once: true });
    return () => {
      line.removeEventListener("animationend", clear);
    };
  }, [source.isPending, location.line, revision]);

  let body: React.ReactNode;
  if (source.isPending) {
    body = (
      <Bullseye>
        <Spinner aria-label={intl.formatMessage(messages.sourceLoading)} />
      </Bullseye>
    );
  } else if (source.isError) {
    body = (
      <Alert
        isInline
        title={intl.formatMessage(messages.sourceErrorTitle)}
        variant="danger"
      >
        {source.error.message}
      </Alert>
    );
  } else {
    body = (
      <div className="sv-source">
        <pre className="shiki" style={rootStyle(source.data)}>
          <code>
            {source.data.lines.map((line, index) => (
              <Fragment key={index}>
                <span className="line">
                  {line.tokens.map((item, tokenIndex) => (
                    <span key={tokenIndex} style={tokenStyle(item)}>
                      {tokenChildren(item, pattern)}
                    </span>
                  ))}
                </span>
                <br />
              </Fragment>
            ))}
          </code>
        </pre>
      </div>
    );
  }

  return (
    <div className="sv-document">
      <div className="sv-document__header sv-source__header">
        <Title headingLevel="h2" size="md">
          {location.path}
        </Title>
        <Label isCompact>{language}</Label>
      </div>
      <div
        aria-label={intl.formatMessage(messages.sourceLabel)}
        className="sv-document__body"
        ref={scroller}
        role="region"
        tabIndex={-1}
      >
        {body}
      </div>
    </div>
  );
}
