import {
  Button,
  EmptyState,
  EmptyStateBody,
  SearchInput,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
import { SearchIcon, ThumbtackIcon } from "@patternfly/react-icons";
import { useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { SearchHit } from "../domain/search";
import type { SpecTreeFile } from "../domain/spec-tree";
import { messages } from "../messages";
import { SpecTreeView } from "./SpecTreeView";
import type { Pins } from "./use-pins";

interface SearchPanelProps {
  /** What the user typed; updates on every keystroke. */
  query: string;
  onQueryChange: (query: string) => void;
  /** Query the results were computed for; may lag behind `query`. */
  searchedQuery: string;
  hits: SearchHit[];
  tree: SpecTreeFile[];
  pins: Pins;
  selectedNodeId: string | undefined;
  onSelect: (nodeId: string) => void;
}

export function SearchPanel({
  query,
  onQueryChange,
  searchedQuery,
  hits,
  tree,
  pins,
  selectedNodeId,
  onSelect,
}: SearchPanelProps) {
  const intl = useIntl();
  const searchInput = useRef<HTMLInputElement>(null);
  // Searching is the first thing to do here, so start with the input focused.
  useEffect(() => {
    searchInput.current?.focus();
  }, []);
  const hasQuery = searchedQuery.trim() !== "";
  // Collapsing remounts the tree with every item collapsed (or expanded), so
  // the toggle works again after items were opened by hand. A new search
  // starts expanded.
  const [layout, setLayout] = useState({
    query: searchedQuery,
    collapsed: false,
    revision: 0,
  });
  const collapsed = layout.query === searchedQuery && layout.collapsed;
  const toggleCollapsed = () => {
    setLayout((current) => ({
      query: searchedQuery,
      collapsed: !collapsed,
      revision: current.revision + 1,
    }));
  };

  const togglePins = (nodeIds: string[]) => {
    if (nodeIds.every(pins.isPinned)) {
      pins.unpin(nodeIds);
    } else {
      pins.pin(nodeIds);
    }
  };

  let body: React.ReactNode;
  if (!hasQuery) {
    body = (
      <EmptyState
        headingLevel="h2"
        icon={SearchIcon}
        titleText={intl.formatMessage(messages.searchHintTitle)}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.searchHintBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  } else if (hits.length === 0) {
    body = (
      <EmptyState
        headingLevel="h2"
        icon={SearchIcon}
        titleText={intl.formatMessage(messages.noResultsTitle)}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.noResultsBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  } else {
    body = (
      <SpecTreeView
        action="pin"
        defaultExpanded={!collapsed}
        key={layout.revision}
        files={tree}
        idPrefix="result"
        isPinned={pins.isPinned}
        label={intl.formatMessage(messages.resultsLabel)}
        onAction={togglePins}
        onSelect={onSelect}
        selectedNodeId={selectedNodeId}
      />
    );
  }

  return (
    <div className="sv-results">
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem className="sv-results__search">
            <SearchInput
              aria-label={intl.formatMessage(messages.searchLabel)}
              ref={searchInput}
              onChange={(_event, value) => {
                onQueryChange(value);
              }}
              onClear={() => {
                onQueryChange("");
              }}
              placeholder={intl.formatMessage(messages.searchLabel)}
              value={query}
            />
          </ToolbarItem>
          <ToolbarItem>
            <Button
              icon={<ThumbtackIcon />}
              isDisabled={hits.length === 0}
              onClick={() => {
                pins.pin(hits.map((hit) => hit.nodeId));
              }}
              variant="secondary"
            >
              {intl.formatMessage(messages.pinResults)}
            </Button>
          </ToolbarItem>
          {hasQuery ? (
            <ToolbarItem className="sv-results__count">
              {intl.formatMessage(messages.resultCount, {
                matches: intl.formatMessage(messages.countMatches, {
                  count: hits.length,
                }),
                files: intl.formatMessage(messages.countFiles, {
                  count: tree.length,
                }),
              })}
              {hits.length > 0 ? (
                <Button isInline onClick={toggleCollapsed} variant="link">
                  {intl.formatMessage(
                    collapsed ? messages.expandAll : messages.collapseAll,
                  )}
                </Button>
              ) : null}
            </ToolbarItem>
          ) : null}
        </ToolbarContent>
      </Toolbar>
      <div className="sv-results__body">{body}</div>
    </div>
  );
}
