import {
  Button,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  Pagination,
  SearchInput,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
} from "@patternfly/react-core";
import {
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  type ThProps,
} from "@patternfly/react-table";

export interface ResourceTableColumn<Row> {
  id: string;
  label: string;
  render: (row: Row) => React.ReactNode;
  sortable?: boolean;
  width?: ThProps["width"];
}

export interface ResourceTableState {
  page: number;
  pageSize: number;
  query: string;
  sortColumnId: string;
  sortDirection: "asc" | "desc";
}

export type ResourceTableStateChangeReason =
  "filter" | "page" | "page-size" | "sort";

interface ResourceTableLabels {
  actions: string;
  clearFilters: string;
  emptyBody: string;
  emptyTitle: string;
  noResultsBody: string;
  noResultsTitle: string;
  resultsCountContext: string;
  searchAriaLabel: string;
  searchPlaceholder: string;
}

interface ResourceTableProps<Row> {
  ariaLabel: string;
  // Content rendered between the toolbar and the table, e.g. a bulk-action
  // results banner. Rendered in every row state so it survives a refetch.
  banner?: React.ReactNode;
  // Bulk-action controls (e.g. Delete, Clear selection) placed in the primary
  // toolbar row after the filters.
  bulkActions?: React.ReactNode;
  // Header bulk-selection control placed at the start of the primary toolbar
  // row, before the search input.
  bulkSelect?: React.ReactNode;
  columns: readonly ResourceTableColumn<Row>[];
  /**
   * Controls rendered inline on the first toolbar row, alongside the search
   * input and the primary action (for example an additional list filter).
   */
  filterControls?: React.ReactNode;
  getRowKey: (row: Row) => React.Key;
  hasActiveFilters?: boolean;
  id: string;
  itemCount: number;
  labels: ResourceTableLabels;
  onStateChange: (
    state: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => void;
  onClearFilters?: () => void;
  pageSizeOptions: readonly number[];
  primaryAction?: React.ReactNode;
  renderRowAction?: (row: Row) => React.ReactNode;
  // When provided, a leading selection column is rendered and this returns the
  // per-row selection control. The caller owns selection state.
  renderRowSelection?: (row: Row) => React.ReactNode;
  rows: readonly Row[];
  /**
   * Controls rendered on a second toolbar row directly below the first row,
   * for a persistent list filter that reads as its own row (for example the
   * gateway ownership switch, per WEB-UIEX-07). The whole row is omitted when
   * this is not provided, so no empty toolbar row remains.
   */
  secondaryFilterControls?: React.ReactNode;
  // Screen-reader heading for the leading selection column.
  selectionColumnLabel?: string;
  /**
   * Whether to show pagination controls below the table. Defaults to true for
   * long lists (WEB-GLIS-02.03). Set to false for tables in space-constrained
   * layouts where only top pagination is desired.
   */
  showBottomPagination?: boolean;
  state: ResourceTableState;
}

export function ResourceTable<Row>({
  ariaLabel,
  banner,
  bulkActions,
  bulkSelect,
  columns,
  filterControls,
  getRowKey,
  hasActiveFilters,
  id,
  itemCount,
  labels,
  onStateChange,
  onClearFilters,
  pageSizeOptions,
  primaryAction,
  renderRowAction,
  renderRowSelection,
  rows,
  secondaryFilterControls,
  selectionColumnLabel,
  showBottomPagination = true,
  state,
}: ResourceTableProps<Row>) {
  const activeSortIndex = Math.max(
    0,
    columns.findIndex(({ id: columnId }) => columnId === state.sortColumnId),
  );
  const hasFilter = hasActiveFilters ?? state.query.trim().length > 0;

  const getSortParams = (columnIndex: number): ThProps["sort"] => ({
    columnIndex,
    onSort: (_event, index, direction) => {
      const column = columns[index];
      if (column) {
        onStateChange(
          {
            ...state,
            page: 1,
            sortColumnId: column.id,
            sortDirection: direction,
          },
          "sort",
        );
      }
    },
    sortBy: {
      direction: state.sortDirection,
      index: activeSortIndex,
    },
  });

  const clearFilter = () => {
    if (onClearFilters) {
      onClearFilters();
    } else {
      onStateChange({ ...state, page: 1, query: "" }, "filter");
    }
  };

  return (
    <>
      <Toolbar id={`${id}-toolbar`}>
        <ToolbarContent rowWrap={{ default: "wrap", md: "nowrap" }}>
          {bulkSelect ? <ToolbarItem>{bulkSelect}</ToolbarItem> : null}
          <ToolbarItem>
            <SearchInput
              aria-label={labels.searchAriaLabel}
              onChange={(_event, value) => {
                onStateChange({ ...state, page: 1, query: value }, "filter");
              }}
              onClear={clearFilter}
              placeholder={labels.searchPlaceholder}
              resultsCount={hasFilter ? itemCount : undefined}
              resultsCountContext={labels.resultsCountContext}
              value={state.query}
            />
          </ToolbarItem>
          {filterControls ? <ToolbarItem>{filterControls}</ToolbarItem> : null}
          {bulkActions ? <ToolbarGroup>{bulkActions}</ToolbarGroup> : null}
          {primaryAction ? <ToolbarItem>{primaryAction}</ToolbarItem> : null}
          <ToolbarItem
            align={{ default: "alignStart", md: "alignEnd" }}
            variant="pagination"
          >
            <Pagination
              isCompact
              itemCount={itemCount}
              onSetPage={(_event, nextPage) => {
                onStateChange({ ...state, page: nextPage }, "page");
              }}
              onPerPageSelect={(_event, nextPageSize) => {
                onStateChange(
                  { ...state, page: 1, pageSize: nextPageSize },
                  "page-size",
                );
              }}
              page={state.page}
              perPage={state.pageSize}
              perPageOptions={pageSizeOptions.map((value) => ({
                title: String(value),
                value,
              }))}
              widgetId={`${id}-pagination`}
            />
          </ToolbarItem>
        </ToolbarContent>
        {/* Secondary toolbar row rendered directly below the filtering
            controls. It uses PatternFly toolbar layout (a second
            ToolbarContent) so the row aligns with the first-row controls and
            the surrounding list content without manual offsets. The whole row
            is omitted when no secondary controls are provided, so no empty
            toolbar row remains (WEB-UIEX-07). */}
        {secondaryFilterControls ? (
          <ToolbarContent>
            <ToolbarItem>{secondaryFilterControls}</ToolbarItem>
          </ToolbarContent>
        ) : null}
      </Toolbar>

      {banner}

      {rows.length > 0 ? (
        <>
          <Table aria-label={ariaLabel} variant="compact">
            <Thead>
              <Tr>
                {renderRowSelection ? (
                  <Th screenReaderText={selectionColumnLabel} />
                ) : null}
                {columns.map((column, columnIndex) => (
                  <Th
                    key={column.id}
                    sort={
                      column.sortable === false
                        ? undefined
                        : getSortParams(columnIndex)
                    }
                    width={column.width}
                  >
                    {column.label}
                  </Th>
                ))}
                {renderRowAction ? (
                  <Th screenReaderText={labels.actions} />
                ) : null}
              </Tr>
            </Thead>
            <Tbody>
              {rows.map((row) => (
                <Tr key={getRowKey(row)}>
                  {renderRowSelection ? (
                    <Td dataLabel={selectionColumnLabel}>
                      {renderRowSelection(row)}
                    </Td>
                  ) : null}
                  {columns.map((column) => (
                    <Td dataLabel={column.label} key={column.id}>
                      {column.render(row)}
                    </Td>
                  ))}
                  {renderRowAction ? (
                    <Td isActionCell modifier="fitContent">
                      {renderRowAction(row)}
                    </Td>
                  ) : null}
                </Tr>
              ))}
            </Tbody>
          </Table>
          {/* Bottom pagination for long lists (WEB-GLIS-02.03) */}
          {showBottomPagination ? (
            <Toolbar id={`${id}-bottom-toolbar`}>
              <ToolbarContent>
                <ToolbarItem
                  align={{ default: "alignEnd" }}
                  variant="pagination"
                >
                  <Pagination
                    itemCount={itemCount}
                    onSetPage={(_event, nextPage) => {
                      onStateChange({ ...state, page: nextPage }, "page");
                    }}
                    onPerPageSelect={(_event, nextPageSize) => {
                      onStateChange(
                        { ...state, page: 1, pageSize: nextPageSize },
                        "page-size",
                      );
                    }}
                    page={state.page}
                    perPage={state.pageSize}
                    perPageOptions={pageSizeOptions.map((value) => ({
                      title: String(value),
                      value,
                    }))}
                    variant="bottom"
                    widgetId={`${id}-bottom-pagination`}
                  />
                </ToolbarItem>
              </ToolbarContent>
            </Toolbar>
          ) : null}
        </>
      ) : (
        <EmptyState
          headingLevel="h2"
          titleText={hasFilter ? labels.noResultsTitle : labels.emptyTitle}
          variant={EmptyStateVariant.sm}
        >
          <EmptyStateBody>
            {hasFilter ? labels.noResultsBody : labels.emptyBody}
          </EmptyStateBody>
          {hasFilter ? (
            <Button onClick={clearFilter} variant="link">
              {labels.clearFilters}
            </Button>
          ) : null}
        </EmptyState>
      )}
    </>
  );
}
