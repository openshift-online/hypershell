import { Alert, Bullseye, Label, Spinner, Title } from "@patternfly/react-core";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef } from "react";
import { useIntl } from "react-intl";

import { sourceLanguage } from "../domain/references";
import { messages } from "../messages";
import { highlightHtml } from "./highlight-html";
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
  const html = useMemo(
    () => (source.data ? highlightHtml(source.data, id ? [id] : []) : ""),
    [source.data, id],
  );

  useEffect(() => {
    const container = scroller.current;
    if (!container || html === "") return;
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
  }, [html, location.line, revision]);

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
      <div className="sv-source" dangerouslySetInnerHTML={{ __html: html }} />
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
