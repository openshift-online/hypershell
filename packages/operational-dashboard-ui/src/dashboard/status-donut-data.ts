export interface StatusDonutDatum {
  x: string;
  y: number;
}

export interface StatusDonutLegendDatum {
  name: string;
}

export interface StatusDonutSeries {
  colorScale: string[];
  data: StatusDonutDatum[];
  legendData: StatusDonutLegendDatum[];
}

interface StatusDonutEntry {
  color: string;
  count: number;
  label: string;
  legendName: string;
}

export function buildStatusDonutData(
  entries: readonly StatusDonutEntry[],
): StatusDonutSeries {
  const visibleEntries = entries.filter((entry) => entry.count > 0);

  return {
    colorScale: visibleEntries.map((entry) => entry.color),
    data: visibleEntries.map((entry) => ({ x: entry.label, y: entry.count })),
    legendData: visibleEntries.map((entry) => ({ name: entry.legendName })),
  };
}
