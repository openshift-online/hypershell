import {
  DataList,
  DataListCell,
  DataListItem,
  DataListItemCells,
  DataListItemRow,
} from "@patternfly/react-core";
import type { ReactNode } from "react";

export interface DashboardStatPanelRow {
  key: string;
  label: ReactNode;
  value: ReactNode;
}

function toRowDomId(key: string): string {
  return `dashboard-stat-${key.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
}

export function DashboardStatPanel({
  ariaLabel,
  rows,
}: Readonly<{
  ariaLabel: string;
  rows: readonly DashboardStatPanelRow[];
}>) {
  return (
    <DataList
      aria-label={ariaLabel}
      className="hypershell-dashboard-gateway-releases__stats"
      isCompact
      isPlain
    >
      {rows.map((row) => {
        const rowId = toRowDomId(row.key);

        return (
          <DataListItem key={row.key} id={rowId}>
            <DataListItemRow>
              <DataListItemCells
                dataListCells={[
                  <DataListCell key="label" width={2}>
                    {row.label}
                  </DataListCell>,
                  <DataListCell key="value" alignRight width={1}>
                    <strong>{row.value}</strong>
                  </DataListCell>,
                ]}
                rowid={`${rowId}-row`}
              />
            </DataListItemRow>
          </DataListItem>
        );
      })}
    </DataList>
  );
}
