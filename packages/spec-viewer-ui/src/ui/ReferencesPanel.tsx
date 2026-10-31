import {
  Alert,
  Bullseye,
  Content,
  EmptyState,
  EmptyStateBody,
  Spinner,
  Title,
  TreeView,
  type TreeViewDataItem,
} from "@patternfly/react-core";
import { CodeIcon, FileAltIcon, FlaskIcon } from "@patternfly/react-icons";
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";

import type { FileReferences } from "../domain/references";
import type { SpecNode } from "../domain/spec-document";
import { messages } from "../messages";
import { NodeLabels } from "./NodeLabels";
import { usePorts } from "./ports-context";
import { useSpecRef } from "./ref-context";
import { nodeTitle } from "./SpecTreeView";

export interface SourceLocation {
  path: string;
  line: number;
}

interface ReferencesPanelProps {
  /** Requirement or scenario whose ID is traced; undefined when none is selected. */
  node: SpecNode | undefined;
  selected: SourceLocation | undefined;
  onOpen: (location: SourceLocation) => void;
}

function locationId(location: SourceLocation): string {
  return `${location.path}:${String(location.line)}`;
}

function fileItems(
  files: FileReferences[],
  selectedPath: string | undefined,
): TreeViewDataItem[] {
  return files.map((file) => ({
    id: locationId({ path: file.path, line: file.matches[0]?.line ?? 1 }),
    icon: file.isTest ? <FlaskIcon /> : <FileAltIcon />,
    name: <span className="sv-reference__path">{file.path}</span>,
    defaultExpanded: files.length <= 10 || file.path === selectedPath,
    children: file.matches.map((match) => ({
      id: locationId({ path: file.path, line: match.line }),
      name: (
        <span className="sv-reference__line">
          <span className="sv-reference__number">{match.line}</span>
          <code>{match.text}</code>
        </span>
      ),
    })),
  }));
}

function parseLocation(id: string): SourceLocation | undefined {
  const at = id.lastIndexOf(":");
  const line = Number(id.slice(at + 1));
  return at === -1 || !Number.isInteger(line)
    ? undefined
    : { path: id.slice(0, at), line };
}

export function ReferencesPanel({
  node,
  selected,
  onOpen,
}: ReferencesPanelProps) {
  const intl = useIntl();
  const { repository } = usePorts();
  const ref = useSpecRef();
  const id = node?.specId;
  const references = useQuery({
    queryKey: ["references", ref, id],
    queryFn: () => repository.findReferences(ref, id ?? ""),
    enabled: id !== undefined,
  });

  let body: React.ReactNode;
  if (!node || !id) {
    body = (
      <EmptyState
        headingLevel="h2"
        icon={CodeIcon}
        titleText={intl.formatMessage(messages.referencesNoIdTitle)}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.referencesNoIdBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  } else if (references.isPending) {
    body = (
      <Bullseye>
        <Spinner aria-label={intl.formatMessage(messages.referencesLoading)} />
      </Bullseye>
    );
  } else if (references.isError) {
    body = (
      <Alert
        isInline
        title={intl.formatMessage(messages.referencesErrorTitle)}
        variant="danger"
      >
        {references.error.message}
      </Alert>
    );
  } else {
    const files = references.data;
    const tests = files.filter((file) => file.isTest);
    const others = files.filter((file) => !file.isTest);
    const section = (title: string, list: FileReferences[]) =>
      list.length === 0 ? null : (
        <section className="sv-reference__group">
          <Title headingLevel="h3" size="md">
            {title}
          </Title>
          <TreeView
            activeItems={
              selected ? [{ id: locationId(selected), name: "" }] : []
            }
            aria-label={title}
            className="sv-tree"
            data={fileItems(list, selected?.path)}
            hasGuides
            hasSelectableNodes
            onSelect={(_event, item) => {
              const location = item.id ? parseLocation(item.id) : undefined;
              if (location) onOpen(location);
            }}
          />
        </section>
      );
    body = (
      <>
        {tests.length === 0 ? (
          <Alert
            isInline
            isPlain
            title={intl.formatMessage(messages.referencesNoTests, { id })}
            variant="warning"
          />
        ) : null}
        {section(
          intl.formatMessage(messages.referencesTests, {
            count: tests.length,
          }),
          tests,
        )}
        {section(
          intl.formatMessage(messages.referencesOtherFiles, {
            count: others.length,
          }),
          others,
        )}
      </>
    );
  }

  return (
    <div className="sv-results sv-references">
      <div className="sv-references__header">
        {node && id ? (
          <Content className="sv-node__title" component="p">
            <NodeLabels node={node} />
            {nodeTitle(intl, node)}
          </Content>
        ) : null}
      </div>
      <div className="sv-results__body">{body}</div>
    </div>
  );
}
