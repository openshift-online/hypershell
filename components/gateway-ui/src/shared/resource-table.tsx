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
import { useState } from "react";

const rowsPerPage = 20;

export interface ResourceTableColumn<Row> {
  getSortValue: (row: Row) => string;
  id: string;
  label: string;
  render: (row: Row) => React.ReactNode;
  searchable?: boolean;
  sortable?: boolean;
  width?: ThProps["width"];
}

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
  defaultSortColumn?: number;
  getRowKey: (row: Row) => React.Key;
  id: string;
  labels: ResourceTableLabels;
  primaryAction?: React.ReactNode;
  renderRowAction?: (row: Row) => React.ReactNode;
  rows: readonly Row[];
}

export function ResourceTable<Row>({
  ariaLabel,
  columns,
  defaultSortColumn = 0,
  getRowKey,
  id,
  labels,
  primaryAction,
  renderRowAction,
  rows,
}: ResourceTableProps<Row>) {
  const [query, setQuery] = useState("");
  const [activeSortIndex, setActiveSortIndex] =
    useState<number>(defaultSortColumn);
  const [activeSortDirection, setActiveSortDirection] = useState<
    "asc" | "desc"
  >("asc");
  const [page, setPage] = useState(1);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filteredRows = rows.filter((row) =>
    columns
      .filter(({ searchable = true }) => searchable)
      .some(({ getSortValue }) =>
        getSortValue(row).toLocaleLowerCase().includes(normalizedQuery),
      ),
  );
  const sortedRows = [...filteredRows].sort((left, right) => {
    const activeColumn = columns[activeSortIndex] ?? columns[0];
    if (!activeColumn) {
      return 0;
    }

    const leftValue = activeColumn.getSortValue(left);
    const rightValue = activeColumn.getSortValue(right);
    const comparison = leftValue.localeCompare(rightValue, undefined, {
      numeric: true,
      sensitivity: "base",
    });

    return activeSortDirection === "asc" ? comparison : -comparison;
  });
  const pageCount = Math.max(1, Math.ceil(sortedRows.length / rowsPerPage));
  const currentPage = Math.min(page, pageCount);
  const visibleRows = sortedRows.slice(
    (currentPage - 1) * rowsPerPage,
    currentPage * rowsPerPage,
  );
  const hasFilter = normalizedQuery.length > 0;

  const getSortParams = (columnIndex: number): ThProps["sort"] => ({
    columnIndex,
    onSort: (_event, index, direction) => {
      setActiveSortIndex(index);
      setActiveSortDirection(direction);
      setPage(1);
    },
    sortBy: {
      direction: activeSortDirection,
      index: activeSortIndex,
    },
  });

  const clearFilter = () => {
    setQuery("");
    setPage(1);
  };

  return (
    <>
      <Toolbar id={`${id}-toolbar`}>
        <ToolbarContent rowWrap={{ default: "wrap", md: "nowrap" }}>
          <ToolbarItem>
            <SearchInput
              aria-label={labels.searchAriaLabel}
              onChange={(_event, value) => {
                setQuery(value);
                setPage(1);
              }}
              onClear={clearFilter}
              placeholder={labels.searchPlaceholder}
              resultsCount={hasFilter ? filteredRows.length : undefined}
              resultsCountContext={labels.resultsCountContext}
              value={query}
            />
          </ToolbarItem>
          {primaryAction ? <ToolbarItem>{primaryAction}</ToolbarItem> : null}
          <ToolbarItem
            align={{ default: "alignStart", md: "alignEnd" }}
            variant="pagination"
          >
            <Pagination
              isCompact
              itemCount={filteredRows.length}
              onSetPage={(_event, nextPage) => {
                setPage(nextPage);
              }}
              page={currentPage}
              perPage={rowsPerPage}
              widgetId={`${id}-pagination`}
            />
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>

      {visibleRows.length > 0 ? (
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
            {visibleRows.map((row) => (
              <Tr key={getRowKey(row)}>
                {columns.map((column) => (
                  <Td dataLabel={column.label} key={column.id}>
                    {column.render(row)}
                  </Td>
                ))}
                {renderRowAction ? (
                  <Td hasAction modifier="fitContent">
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
