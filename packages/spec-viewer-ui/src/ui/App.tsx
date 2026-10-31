import {
  Alert,
  Bullseye,
  Flex,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadLogo,
  MastheadMain,
  MastheadToggle,
  Button,
  Drawer,
  FlexItem,
  Tooltip,
  DrawerContent,
  DrawerContentBody,
  DrawerPanelContent,
  Page,
  Spinner,
  ToggleGroup,
  ToggleGroupItem,
} from "@patternfly/react-core";
import {
  BarsIcon,
  CodeIcon,
  CompressIcon,
  ExpandIcon,
  EyeIcon,
  EyeSlashIcon,
} from "@patternfly/react-icons";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useDeferredValue, useMemo, useState } from "react";
import { useIntl } from "react-intl";

import {
  buildSearchIndex,
  highlightTerms,
  searchSpecs,
} from "../domain/search";
import type { SpecCatalog } from "../domain/spec-document";
import { buildSpecTree } from "../domain/spec-tree";
import { messages } from "../messages";
import { ConformancePanel } from "./ConformancePanel";
import type { RefName } from "../application/ports";
import { PinnedPanel } from "./PinnedPanel";
import { usePorts } from "./ports-context";
import { RefPicker } from "./RefPicker";
import { RefProvider } from "./ref-context";
import { ReferencesPanel, type SourceLocation } from "./ReferencesPanel";
import { SearchPanel } from "./SearchPanel";
import { SourceView } from "./SourceView";
import { SpecDocument } from "./SpecDocument";
import { usePins } from "./use-pins";
import { useSelection } from "./use-selection";
import { usePanelWidth } from "./use-panel-width";
import { useSpecCatalog } from "./use-spec-catalog";

type View = "search" | "conformance" | "references";
const VIEW_KEY = "hypershell-spec-viewer.view";

function readView(): View {
  try {
    return window.localStorage.getItem(VIEW_KEY) === "conformance"
      ? "conformance"
      : "search";
  } catch {
    return "search";
  }
}

/**
 * Which panels show: all of them ("split"), the list without the document
 * ("list"), or the document alone ("document").
 */
type Layout = "split" | "list" | "document";
const LAYOUT_KEY = "hypershell-spec-viewer.layout";

function readLayout(): Layout {
  try {
    const stored = window.localStorage.getItem(LAYOUT_KEY);
    return stored === "list" || stored === "document" ? stored : "split";
  } catch {
    return "split";
  }
}

const REF_PARAM = "ref";
const REFS_KEY = ["refs"] as const;

/** The branch in the page URL (`?ref=`); "" is the server's default. */
function readRefFromUrl(): RefName {
  return new URLSearchParams(window.location.search).get(REF_PARAM) ?? "";
}

/** Keeps the branch in the URL, so a link opens the same branch. */
function writeRefToUrl(ref: RefName): void {
  const url = new URL(window.location.href);
  if (ref === "") url.searchParams.delete(REF_PARAM);
  else url.searchParams.set(REF_PARAM, ref);
  window.history.replaceState(window.history.state, "", url);
}

const EMPTY_INDEX = buildSearchIndex({
  files: [],
  nodesById: new Map(),
  filesByPath: new Map(),
});

/** Runs the search; the results trail typing so input stays responsive. */
function useSpecSearch(catalog: SpecCatalog | undefined, query: string) {
  const searchedQuery = useDeferredValue(query);
  const index = useMemo(
    () => (catalog ? buildSearchIndex(catalog) : EMPTY_INDEX),
    [catalog],
  );
  const hits = useMemo(
    () => searchSpecs(index, searchedQuery),
    [index, searchedQuery],
  );
  const tree = useMemo(
    () => (catalog ? buildSpecTree(catalog, hits).files : []),
    [catalog, hits],
  );
  const terms = useMemo(
    () => highlightTerms(searchedQuery, hits),
    [searchedQuery, hits],
  );
  return { searchedQuery, hits, tree, terms };
}

export function App() {
  const [ref, setRef] = useState<RefName>(readRefFromUrl);
  return (
    <RefProvider value={ref}>
      <SpecViewer
        onRefChange={(next) => {
          writeRefToUrl(next);
          setRef(next);
        }}
        specRef={ref}
      />
    </RefProvider>
  );
}

interface SpecViewerProps {
  specRef: RefName;
  onRefChange: (ref: RefName) => void;
}

function SpecViewer({ specRef, onRefChange }: SpecViewerProps) {
  const intl = useIntl();
  const { repository } = usePorts();
  const refs = useQuery({
    queryKey: REFS_KEY,
    queryFn: () => repository.listRefs(),
    staleTime: 60_000,
  });
  const catalogQuery = useSpecCatalog();
  const pins = usePins();
  const [selection, select] = useSelection();
  const catalog = catalogQuery.data;
  const [query, setQuery] = useState("");
  const [pinnedOpen, setPinnedOpen] = useState(true);
  const [pinnedWidth, setPinnedWidth] = usePanelWidth("pinned");
  const [narrowWidth, setNarrowWidth] = usePanelWidth("list");
  // Conformance needs a wider table, so it keeps its own width.
  const [wideWidth, setWideWidth] = usePanelWidth("conformance");
  const [view, setViewState] = useState<View>(readView);
  // A repository file opened from the references or conformance view
  // replaces the spec document until a spec item is selected or the view
  // changes. `id` is the requirement or scenario ID marked in the file.
  const [source, setSource] = useState<{
    location: SourceLocation;
    id: string | undefined;
    revision: number;
  } | null>(null);
  const setView = (next: View) => {
    setViewState(next);
    setSource(null);
    // The references view follows the selection, so it is not remembered.
    if (next === "references") return;
    try {
      window.localStorage.setItem(VIEW_KEY, next);
    } catch {
      // The view choice is a convenience; blocked storage keeps the default.
    }
  };
  const [layout, setLayoutState] = useState<Layout>(readLayout);
  const setLayout = (next: Layout) => {
    setLayoutState(next);
    try {
      window.localStorage.setItem(LAYOUT_KEY, next);
    } catch {
      // The layout is a convenience; blocked storage keeps the default.
    }
  };
  // Opening something while the document is hidden shows it again.
  const showDocument = useCallback(() => {
    setLayoutState((current) => (current === "list" ? "split" : current));
  }, []);
  const selectNode = (nodeId: string) => {
    setSource(null);
    select(nodeId);
    showDocument();
  };
  // Stable, so the conformance table does not rebuild its rows each render.
  const openSource = useCallback(
    (location: SourceLocation, id?: string) => {
      showDocument();
      setSource((current) => ({
        location,
        id,
        revision: (current?.revision ?? 0) + 1,
      }));
    },
    [showDocument],
  );
  const search = useSpecSearch(catalog, query);

  const node = selection ? catalog?.nodesById.get(selection.nodeId) : undefined;
  const file = node ? catalog?.filesByPath.get(node.path) : undefined;

  let summary = "";
  if (catalog) {
    let specs = 0;
    let scenarios = 0;
    for (const entry of catalog.nodesById.values()) {
      if (entry.kind === "spec") specs += 1;
      if (entry.kind === "scenario") scenarios += 1;
    }
    summary = intl.formatMessage(messages.catalogSummary, {
      files: intl.formatMessage(messages.countFiles, {
        count: catalog.files.length,
      }),
      specs: intl.formatMessage(messages.countSpecs, { count: specs }),
      scenarios: intl.formatMessage(messages.countScenarios, {
        count: scenarios,
      }),
    });
  }

  const masthead = (
    <Masthead>
      <MastheadMain>
        <MastheadToggle>
          <Button
            aria-expanded={pinnedOpen}
            aria-label={intl.formatMessage(messages.pinnedTitle)}
            icon={<BarsIcon />}
            onClick={() => {
              setPinnedOpen(!pinnedOpen);
            }}
            variant="plain"
          />
        </MastheadToggle>
        <MastheadBrand>
          <MastheadLogo component="span">
            {intl.formatMessage(messages.productName)}
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          gap={{ default: "gapLg" }}
        >
          <RefPicker
            onChange={(next) => {
              // An opened file belongs to the old branch.
              setSource(null);
              onRefChange(next);
            }}
            refs={refs.data}
            value={specRef}
            {...(refs.isError ? { error: refs.error.message } : {})}
          />
          <ToggleGroup
            aria-label={intl.formatMessage(messages.viewLabel)}
            isCompact
          >
            <ToggleGroupItem
              isSelected={view === "search"}
              onChange={() => {
                setView("search");
              }}
              text={intl.formatMessage(messages.viewSearch)}
            />
            <ToggleGroupItem
              isSelected={view === "conformance"}
              onChange={() => {
                setView("conformance");
              }}
              text={intl.formatMessage(messages.viewConformance)}
            />
            {node?.specId ? (
              <ToggleGroupItem
                aria-label={intl.formatMessage(messages.viewReferences, {
                  id: node.specId,
                })}
                icon={<CodeIcon />}
                isSelected={view === "references"}
                onChange={() => {
                  setView("references");
                }}
                text={node.specId}
              />
            ) : null}
          </ToggleGroup>
          <span className="sv-summary">{summary}</span>
          <FlexItem align={{ default: "alignRight" }}>
            <LayoutButtons layout={layout} onChange={setLayout} />
          </FlexItem>
        </Flex>
      </MastheadContent>
    </Masthead>
  );

  let content: React.ReactNode;
  if (catalogQuery.isPending) {
    content = (
      <Bullseye>
        <Spinner aria-label={intl.formatMessage(messages.loading)} />
      </Bullseye>
    );
  } else if (catalogQuery.isError) {
    content = (
      <Alert
        isInline
        title={intl.formatMessage(messages.loadErrorTitle)}
        variant="danger"
      >
        {catalogQuery.error.message}
      </Alert>
    );
  } else {
    const wide = view === "conformance";
    const listWidth = wide ? wideWidth : narrowWidth;
    const setListWidth = wide ? setWideWidth : setNarrowWidth;
    const listContent =
      view === "conformance" ? (
        <ConformancePanel
          catalog={catalogQuery.data}
          onOpenSource={openSource}
          onSelect={selectNode}
          selectedNodeId={selection?.nodeId}
        />
      ) : view === "references" ? (
        <ReferencesPanel
          node={node}
          onOpen={(location) => {
            openSource(location, node?.specId);
          }}
          selected={source?.location}
        />
      ) : (
        <SearchPanel
          hits={search.hits}
          onQueryChange={setQuery}
          onSelect={selectNode}
          pins={pins}
          query={query}
          searchedQuery={search.searchedQuery}
          selectedNodeId={selection?.nodeId}
          tree={search.tree}
        />
      );
    const listPanel = (
      <DrawerPanelContent
        className="sv-panel"
        defaultSize={
          listWidth === null ? (wide ? "67%" : "36%") : `${String(listWidth)}px`
        }
        increment={16}
        isResizable
        key={wide ? "wide" : "list"}
        // Leave the document room, whatever width was saved.
        maxSize="calc(100% - 240px)"
        minSize={wide ? "480px" : "320px"}
        onResize={(_event, width) => {
          setListWidth(width);
        }}
        resizeAriaLabel={intl.formatMessage(messages.resizeList)}
      >
        {listContent}
      </DrawerPanelContent>
    );
    content =
      layout === "list" ? (
        <div className="sv-list-only">{listContent}</div>
      ) : (
        <Drawer
          className="sv-drawer"
          isExpanded={layout === "split"}
          isInline
          position="start"
        >
          <DrawerContent panelContent={listPanel}>
            <DrawerContentBody className="sv-drawer__body">
              {source ? (
                <SourceView
                  id={source.id}
                  location={source.location}
                  revision={source.revision}
                />
              ) : (
                <SpecDocument
                  catalog={catalogQuery.data}
                  file={file}
                  highlightTerms={search.terms}
                  node={node}
                  revision={selection?.revision ?? 0}
                />
              )}
            </DrawerContentBody>
          </DrawerContent>
        </Drawer>
      );
  }

  const pinnedPanel = (
    <DrawerPanelContent
      className="sv-panel sv-panel--pinned"
      defaultSize={pinnedWidth === null ? "24rem" : `${String(pinnedWidth)}px`}
      increment={16}
      isResizable
      minSize="200px"
      onResize={(_event, width) => {
        setPinnedWidth(width);
      }}
      resizeAriaLabel={intl.formatMessage(messages.resizePinned)}
    >
      {catalog ? (
        <PinnedPanel
          catalog={catalog}
          onSelect={selectNode}
          pins={pins}
          selectedNodeId={selection?.nodeId}
        />
      ) : null}
    </DrawerPanelContent>
  );

  return (
    <Page className="sv-page" isContentFilled masthead={masthead}>
      <Drawer
        className="sv-drawer"
        isExpanded={pinnedOpen && layout !== "document"}
        isInline
        position="start"
      >
        <DrawerContent panelContent={pinnedPanel}>
          <DrawerContentBody className="sv-drawer__body">
            {content}
          </DrawerContentBody>
        </DrawerContent>
      </Drawer>
    </Page>
  );
}

interface LayoutButtonsProps {
  layout: Layout;
  onChange: (layout: Layout) => void;
}

/** Hides or shows the document, and maximizes or restores it. */
function LayoutButtons({ layout, onChange }: LayoutButtonsProps) {
  const intl = useIntl();
  const hidden = layout === "list";
  const maximized = layout === "document";
  const hideLabel = intl.formatMessage(
    hidden ? messages.documentShow : messages.documentHide,
  );
  const maximizeLabel = intl.formatMessage(
    maximized ? messages.documentRestore : messages.documentMaximize,
  );
  return (
    <Flex gap={{ default: "gapXs" }}>
      <Tooltip content={hideLabel}>
        <Button
          aria-label={hideLabel}
          aria-pressed={hidden}
          icon={hidden ? <EyeIcon /> : <EyeSlashIcon />}
          onClick={() => {
            onChange(hidden ? "split" : "list");
          }}
          variant="plain"
        />
      </Tooltip>
      <Tooltip content={maximizeLabel}>
        <Button
          aria-label={maximizeLabel}
          aria-pressed={maximized}
          icon={maximized ? <CompressIcon /> : <ExpandIcon />}
          onClick={() => {
            onChange(maximized ? "split" : "document");
          }}
          variant="plain"
        />
      </Tooltip>
    </Flex>
  );
}
