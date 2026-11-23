import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ResourceTable } from "./resource-table";

interface TestRow {
  id: string;
  name: string;
}

const rows: readonly TestRow[] = [{ id: "row-1", name: "Test row" }];
const actionLabel = "Open actions";
const selectionLabel = "Select row";

describe("ResourceTable", () => {
  it("fits selection and action columns to their controls", () => {
    render(
      <ResourceTable
        ariaLabel="Test resources"
        columns={[
          {
            id: "name",
            label: "Name",
            render: ({ name }) => name,
          },
        ]}
        getRowKey={({ id }) => id}
        id="test-resources"
        itemCount={rows.length}
        labels={{
          actions: "Actions",
          clearFilters: "Clear filters",
          emptyBody: "No resources",
          emptyTitle: "No resources",
          noResultsBody: "No matching resources",
          noResultsTitle: "No matching resources",
          resultsCountContext: "results",
          searchAriaLabel: "Filter resources",
          searchPlaceholder: "Filter resources",
        }}
        onStateChange={() => undefined}
        pageSizeOptions={[10]}
        renderRowAction={() => <button type="button">{actionLabel}</button>}
        renderRowSelection={() => (
          <input aria-label={selectionLabel} type="checkbox" />
        )}
        rows={rows}
        selectionColumnLabel="Row selection"
        state={{
          page: 1,
          pageSize: 10,
          query: "",
          sortColumnId: "name",
          sortDirection: "asc",
        }}
      />,
    );

    expect(
      screen
        .getByRole("columnheader", {
          hidden: true,
          name: "Row selection",
        })
        .classList.contains("pf-m-fit-content"),
    ).toBe(true);
    expect(
      screen
        .getByLabelText(selectionLabel)
        .closest("td")
        ?.classList.contains("pf-m-fit-content"),
    ).toBe(true);
    expect(
      screen
        .getByRole("button", { name: actionLabel })
        .closest("td")
        ?.classList.contains("pf-m-fit-content"),
    ).toBe(true);
  });
});
