import {
  Alert,
  Bullseye,
  Flex,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadLogo,
  MastheadMain,
  Button,
  Drawer,
  FlexItem,
  Tooltip,
  DrawerContent,
  DrawerContentBody,
  DrawerPanelContent,
  EmptyState,
  EmptyStateBody,
  Label,
  Page,
  SearchInput,
  Spinner,
} from "@patternfly/react-core";
import {
  CheckCircleIcon,
  CompressIcon,
  ExpandIcon,
  EyeIcon,
  EyeSlashIcon,
  MoonIcon,
  OutlinedStarIcon,
  StarIcon,
  SunIcon,
} from "@patternfly/react-icons";
import { useQuery } from "@tanstack/react-query";
import {
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useIntl } from "react-intl";

import {
  buildSearchIndex,
  highlightTerms,
  searchSpecs,
} from "../domain/search";
import { diffCatalogs, isEmptyDiff } from "../domain/spec-diff";
import type { SpecCatalog } from "../domain/spec-document";
import { messages } from "../messages";
import { ConformancePanel } from "./ConformancePanel";
import { DiffPanel } from "./DiffPanel";
import { DiffView } from "./DiffView";
import { nodeTitle } from "./NodeText";
import type { RefName } from "../application/ports";
import { usePorts } from "./ports-context";
import { refLabel, RefPicker } from "./RefPicker";
import { RefProvider } from "./ref-context";
import { ReferencesPanel, type SourceLocation } from "./ReferencesPanel";
import { SourceView } from "./SourceView";
import { SpecDocument } from "./SpecDocument";
import { usePins } from "./use-pins";
import { useSelection } from "./use-selection";
import { usePanelWidth } from "./use-panel-width";
import { useCatalogOf, useSpecCatalog } from "./use-spec-catalog";
import { useTheme, type Theme } from "./use-theme";

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
const COMPARE_PARAM = "compare";
const REFS_KEY = ["refs"] as const;

/** The branch in the page URL (`?ref=`); "" is the server's default. */
function readRefFromUrl(): RefName {
  return new URLSearchParams(window.location.search).get(REF_PARAM) ?? "";
}

/** The branch compared against (`?compare=`); null outside diff mode. */
function readCompareFromUrl(): RefName | null {
  const value = new URLSearchParams(window.location.search).get(COMPARE_PARAM);
  return value === null || value === "" ? null : value;
}

/** Keeps a branch in the URL, so a link opens the same branch or diff. */
function writeParamToUrl(name: string, value: RefName | null): void {
  const url = new URL(window.location.href);
  if (value === null || value === "") url.searchParams.delete(name);
  else url.searchParams.set(name, value);
  window.history.replaceState(window.history.state, "", url);
}

/** Search terms to mark; none in diff mode, where search is off. */
const NO_TERMS: string[] = [];

const EMPTY_INDEX = buildSearchIndex({
  files: [],
  nodesById: new Map(),
  filesByPath: new Map(),
});

/** Every hit filters the table, so the search is not capped. */
const ALL_HITS = Number.MAX_SAFE_INTEGER;

/** Runs the search; the results trail typing so input stays responsive. */
function useSpecSearch(catalog: SpecCatalog | undefined, query: string) {
  const searchedQuery = useDeferredValue(query);
  const index = useMemo(
    () => (catalog ? buildSearchIndex(catalog) : EMPTY_INDEX),
    [catalog],
  );
  const hits = useMemo(
    () => searchSpecs(index, searchedQuery, ALL_HITS),
    [index, searchedQuery],
  );
  const terms = useMemo(
    () => highlightTerms(searchedQuery, hits),
    [searchedQuery, hits],
  );
  // Undefined while nothing is searched, so the table is not filtered.
  const hitsById = useMemo(
    () =>
      searchedQuery.trim() === ""
        ? undefined
        : new Map(hits.map((hit) => [hit.nodeId, hit])),
    [searchedQuery, hits],
  );
  const fileCount = useMemo(() => {
    if (!catalog) return 0;
    const paths = new Set<string>();
    for (const hit of hits) {
      const path = catalog.nodesById.get(hit.nodeId)?.path;
      if (path) paths.add(path);
    }
    return paths.size;
  }, [catalog, hits]);
  return { searchedQuery, hits, hitsById, terms, fileCount };
}

export function App() {
  const [ref, setRef] = useState<RefName>(readRefFromUrl);
  const [compare, setCompare] = useState<RefName | null>(readCompareFromUrl);
  return (
    <RefProvider value={ref}>
      <SpecViewer
        compareRef={compare}
        onCompareChange={(next) => {
          writeParamToUrl(COMPARE_PARAM, next);
          setCompare(next);
        }}
        onRefChange={(next) => {
          writeParamToUrl(REF_PARAM, next);
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
  /** The branch compared against; null when diff mode is off. */
  compareRef: RefName | null;
  onCompareChange: (ref: RefName | null) => void;
}

function SpecViewer({
  specRef,
  onRefChange,
  compareRef,
  onCompareChange,
}: SpecViewerProps) {
  const intl = useIntl();
  const { repository } = usePorts();
  const refs = useQuery({
    queryKey: REFS_KEY,
    queryFn: () => repository.listRefs(),
    staleTime: 60_000,
  });
  const catalogQuery = useSpecCatalog();
  const diffMode = compareRef !== null;
  const compareQuery = useCatalogOf(compareRef ?? "", diffMode);
  const pins = usePins();
  const [selection, select] = useSelection();
  const catalog = catalogQuery.data;
  const compareCatalog = diffMode ? compareQuery.data : undefined;
  const diff = useMemo(
    () =>
      catalog && compareCatalog
        ? diffCatalogs(compareCatalog, catalog)
        : undefined,
    [catalog, compareCatalog],
  );
  // The open entry of the diff tree; null shows the normal selection.
  const [diffSelection, setDiffSelection] = useState<{
    key: string;
    revision: number;
  } | null>(null);
  const diffEntry = diffSelection
    ? diff?.byKey.get(diffSelection.key)
    : undefined;
  const [query, setQuery] = useState("");
  // The conformance table is wide, so its width is stored on its own.
  const [listWidth, setListWidth] = usePanelWidth("conformance");
  const [starredOnly, setStarredOnly] = useState(false);
  // The requirement or scenario whose ID references are listed, in place of
  // the conformance table; null shows the table.
  const [referencesOf, setReferencesOf] = useState<string | null>(null);
  const searchInput = useRef<HTMLInputElement>(null);
  // Searching is the first thing to do, so start with the input focused.
  useEffect(() => {
    searchInput.current?.focus();
  }, []);
  // A repository file opened from the references or conformance view
  // replaces the spec document until a spec item is selected or the view
  // changes. `id` is the requirement or scenario ID marked in the file.
  const [source, setSource] = useState<{
    location: SourceLocation;
    id: string | undefined;
    revision: number;
  } | null>(null);
  const theme = useTheme();
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
    setDiffSelection(null);
    select(nodeId);
    showDocument();
  };
  const selectDiffEntry = (key: string) => {
    setSource(null);
    setDiffSelection((current) => ({
      key,
      revision: (current?.revision ?? 0) + 1,
    }));
    showDocument();
  };
  const changeCompare = (next: RefName | null) => {
    setDiffSelection(null);
    setSource(null);
    setReferencesOf(null);
    onCompareChange(next);
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
  // Stable, so the conformance table does not rebuild its rows each render.
  // The list shows the references, so a maximized document makes room.
  const findReferences = useCallback((nodeId: string) => {
    setReferencesOf(nodeId);
    setLayoutState((current) => (current === "document" ? "split" : current));
  }, []);
  const referencesNode =
    referencesOf === null
      ? undefined
      : (catalog?.nodesById.get(referencesOf) ??
        compareCatalog?.nodesById.get(referencesOf));

  const node = selection ? catalog?.nodesById.get(selection.nodeId) : undefined;
  const file = node ? catalog?.filesByPath.get(node.path) : undefined;

  const currentName = specRef === "" ? refs.data?.default : specRef;
  const currentLabel =
    currentName === undefined
      ? intl.formatMessage(messages.refDefault)
      : refLabel(intl, currentName);
  const compareLabel = compareRef === null ? "" : refLabel(intl, compareRef);

  let summary = "";
  if (diff) {
    summary = intl.formatMessage(messages.diffSummary, {
      added: diff.counts.added,
      removed: diff.counts.removed,
      changed: diff.counts.changed,
    });
  } else if (diffMode) {
    summary = "";
  } else if (catalog && search.hitsById) {
    summary = intl.formatMessage(messages.resultCount, {
      matches: intl.formatMessage(messages.countMatches, {
        count: search.hits.length,
      }),
      files: intl.formatMessage(messages.countFiles, {
        count: search.fileCount,
      }),
    });
  } else if (catalog) {
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
        <MastheadBrand>
          <MastheadLogo component="span">
            {intl.formatMessage(messages.productName)}
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          className="sv-masthead__bar"
          gap={{ default: "gapLg" }}
        >
          <RefPicker
            comparing={compareRef}
            onChange={(next) => {
              // An opened file belongs to the old branch.
              setSource(null);
              setDiffSelection(null);
              onRefChange(next);
            }}
            onCompare={changeCompare}
            refs={refs.data}
            value={specRef}
            {...(refs.isError ? { error: refs.error.message } : {})}
          />
          {compareRef === null ? null : (
            <Label
              closeBtnAriaLabel={intl.formatMessage(messages.diffExit)}
              color="blue"
              onClose={() => {
                changeCompare(null);
              }}
            >
              {intl.formatMessage(messages.diffMode, {
                branch: compareLabel,
              })}
            </Label>
          )}
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            className="sv-masthead__search"
            flexWrap={{ default: "nowrap" }}
            gap={{ default: "gapXs" }}
          >
            <SearchInput
              aria-label={intl.formatMessage(messages.searchLabel)}
              className="sv-search"
              isDisabled={diffMode}
              onChange={(_event, value) => {
                setQuery(value);
                // A search lands on the table it filters.
                setReferencesOf(null);
              }}
              onClear={() => {
                setQuery("");
              }}
              placeholder={intl.formatMessage(messages.searchLabel)}
              ref={searchInput}
              value={query}
            />
            <Tooltip content={intl.formatMessage(messages.starredOnly)}>
              <Button
                aria-label={intl.formatMessage(messages.starredOnly)}
                aria-pressed={starredOnly}
                className={starredOnly ? "sv-star sv-star--on" : "sv-star"}
                icon={starredOnly ? <StarIcon /> : <OutlinedStarIcon />}
                isDisabled={diffMode}
                onClick={() => {
                  setStarredOnly(!starredOnly);
                  setReferencesOf(null);
                }}
                variant="plain"
              />
            </Tooltip>
          </Flex>
          <span className="sv-summary">{summary}</span>
          <FlexItem align={{ default: "alignRight" }}>
            <Flex gap={{ default: "gapXs" }}>
              <LayoutButtons layout={layout} onChange={setLayout} />
              <ThemeButton onToggle={theme.toggle} theme={theme.theme} />
            </Flex>
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
  } else if (diffMode && compareQuery.isPending) {
    content = (
      <Bullseye>
        <Spinner aria-label={intl.formatMessage(messages.loading)} />
      </Bullseye>
    );
  } else if (diffMode && compareQuery.isError) {
    content = (
      <Alert
        isInline
        title={intl.formatMessage(messages.diffErrorTitle)}
        variant="danger"
      >
        {compareQuery.error.message}
      </Alert>
    );
  } else {
    // The table stays mounted under the references list, so Back returns
    // to it as it was: same filter, expanded rows, and scroll position.
    const listContent = (
      <>
        {referencesNode ? (
          <ReferencesPanel
            node={referencesNode}
            onBack={() => {
              setReferencesOf(null);
            }}
            onOpen={(location) => {
              openSource(location, referencesNode.specId);
            }}
            selected={source?.location}
          />
        ) : null}
        {diff ? (
          <div
            className="sv-list-view"
            hidden={referencesNode !== undefined || !diffMode}
          >
            {isEmptyDiff(diff) ? (
              <EmptyState
                headingLevel="h2"
                icon={CheckCircleIcon}
                titleText={intl.formatMessage(messages.diffEmptyTitle)}
              >
                <EmptyStateBody>
                  {intl.formatMessage(messages.diffEmptyBody, {
                    current: currentLabel,
                    compare: compareLabel,
                  })}
                </EmptyStateBody>
              </EmptyState>
            ) : (
              <DiffPanel
                diff={diff}
                onSelect={selectDiffEntry}
                selectedKey={diffEntry?.key}
              />
            )}
          </div>
        ) : null}
        <div
          className="sv-list-view"
          hidden={referencesNode !== undefined || diffMode}
        >
          <ConformancePanel
            catalog={catalogQuery.data}
            hits={search.hitsById}
            // A new filter starts with the rows above its matches expanded.
            key={`${search.searchedQuery}\u0000${String(starredOnly)}`}
            onFindReferences={findReferences}
            onOpenSource={openSource}
            onSelect={selectNode}
            selectedNodeId={selection?.nodeId}
            starredOnly={starredOnly}
            stars={pins}
          />
        </div>
      </>
    );
    const listPanel = (
      <DrawerPanelContent
        className="sv-panel"
        hidden={layout === "document"}
        defaultSize={listWidth === null ? "67%" : `${String(listWidth)}px`}
        increment={16}
        isResizable
        // Leave the document room, whatever width was saved.
        maxSize="calc(100% - 240px)"
        minSize="480px"
        onResize={(_event, width) => {
          setListWidth(width);
        }}
        resizeAriaLabel={intl.formatMessage(messages.resizeList)}
      >
        {listContent}
      </DrawerPanelContent>
    );
    // One open drawer for every layout. PatternFly unmounts a closed
    // panel's contents, and a different tree would remount the table, so
    // either would lose its expanded rows. The layouts only hide parts.
    content = (
      <Drawer
        className={`sv-drawer sv-drawer--${layout}`}
        isExpanded
        isInline
        position="start"
      >
        <DrawerContent panelContent={listPanel}>
          <DrawerContentBody
            className="sv-drawer__body"
            hidden={layout === "list"}
          >
            {source ? (
              <SourceView
                id={source.id}
                location={source.location}
                revision={source.revision}
              />
            ) : diffEntry && diffSelection ? (
              diffEntry.kind === "item" &&
              diffEntry.status === "changed" &&
              diffEntry.node &&
              diffEntry.before !== undefined &&
              diffEntry.after !== undefined ? (
                <DiffView
                  after={diffEntry.after}
                  afterLabel={currentLabel}
                  before={diffEntry.before}
                  beforeLabel={compareLabel}
                  path={diffEntry.path}
                  title={nodeTitle(intl, diffEntry.node)}
                />
              ) : (
                // New and removed items are shown as they are, from the
                // branch that has them; nothing is compared.
                <SpecDocument
                  catalog={
                    diffEntry.side === "compare"
                      ? (compareCatalog ?? catalogQuery.data)
                      : catalogQuery.data
                  }
                  file={diffEntry.file}
                  highlightTerms={NO_TERMS}
                  node={diffEntry.node ?? diffEntry.file?.nodes[0]}
                  onFindReferences={findReferences}
                  revision={diffSelection.revision}
                />
              )
            ) : (
              <SpecDocument
                catalog={catalogQuery.data}
                file={file}
                highlightTerms={diffMode ? NO_TERMS : search.terms}
                onFindReferences={findReferences}
                node={node}
                revision={selection?.revision ?? 0}
              />
            )}
          </DrawerContentBody>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Page className="sv-page" isContentFilled masthead={masthead}>
      {content}
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
    <>
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
    </>
  );
}

interface ThemeButtonProps {
  theme: Theme;
  onToggle: () => void;
}

/** Switches between the light and dark themes. */
function ThemeButton({ theme, onToggle }: ThemeButtonProps) {
  const intl = useIntl();
  const label = intl.formatMessage(
    theme === "dark" ? messages.themeLight : messages.themeDark,
  );
  return (
    <Tooltip content={label}>
      <Button
        aria-label={label}
        icon={theme === "dark" ? <SunIcon /> : <MoonIcon />}
        onClick={onToggle}
        variant="plain"
      />
    </Tooltip>
  );
}
