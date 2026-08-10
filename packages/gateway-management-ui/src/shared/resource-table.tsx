import {
  Button,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  Pagination,
  SearchInput,
  Toolbar,
  ToolbarContent,
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
  columns: readonly ResourceTableColumn<Row>[];
  getRowKey: (row: Row) => React.Key;
  id: string;
  itemCount: number;
  labels: ResourceTableLabels;
  onStateChange: (
    state: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => void;
  pageSizeOptions: readonly number[];
  primaryAction?: React.ReactNode;
  renderRowAction?: (row: Row) => React.ReactNode;
  rows: readonly Row[];
  state: ResourceTableState;
}

export function ResourceTable<Row>({
  ariaLabel,
  columns,
  getRowKey,
  id,
  itemCount,
  labels,
  onStateChange,
  pageSizeOptions,
  primaryAction,
  renderRowAction,
  rows,
  state,
}: ResourceTableProps<Row>) {
  const activeSortIndex = Math.max(
    0,
    columns.findIndex(({ id: columnId }) => columnId === state.sortColumnId),
  );
  const hasFilter = state.query.trim().length > 0;

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
    onStateChange({ ...state, page: 1, query: "" }, "filter");
  };

  return (
    <>
      <Toolbar id={`${id}-toolbar`}>
        <ToolbarContent rowWrap={{ default: "wrap", md: "nowrap" }}>
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
      </Toolbar>

      {rows.length > 0 ? (
        <Table aria-label={ariaLabel} variant="compact">
          <Thead>
            <Tr>
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
