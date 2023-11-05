import { derived, writable, type Readable } from "svelte/store";
import {
  StateManagers,
  memoizeMetricsStore,
} from "@rilldata/web-common/features/dashboards/state-managers/state-managers";
import type { MetricsViewSpecMeasureV2 } from "@rilldata/web-common/runtime-client";
import { useTimeControlStore } from "@rilldata/web-common/features/dashboards/time-controls/time-control-store";
import { useTimeSeriesDataStore } from "@rilldata/web-common/features/dashboards/time-series/timeseries-data-store";
import { createSparkline } from "@rilldata/web-common/components/data-graphic/marks/sparkline";
import { safeFormatter, transposeArray } from "./util";
import {
  DEFAULT_TIME_RANGES,
  TIME_COMPARISON,
} from "@rilldata/web-common/lib/time/config";
import { useMetaQuery } from "@rilldata/web-common/features/dashboards/selectors/index";
import {
  getDimensionValueTimeSeries,
  type DimensionDataItem,
} from "@rilldata/web-common/features/dashboards/time-series/multiple-dimension-queries";
import { TimeRangePreset } from "@rilldata/web-common/lib/time/types";
import type {
  ChartInteractionColumns,
  HighlightedCell,
  TableData,
  TablePosition,
  TDDComparison,
} from "./types";
import { createMeasureValueFormatter } from "@rilldata/web-common/lib/number-formatting/format-measure-value";
import { numberPartsToString } from "@rilldata/web-common/lib/number-formatting/utils/number-parts-utils";
import { formatProperFractionAsPercent } from "@rilldata/web-common/lib/number-formatting/proper-fraction-formatter";
import { formatMeasurePercentageDifference } from "@rilldata/web-common/lib/number-formatting/percentage-formatter";

export type TimeDimensionDataState = {
  isFetching: boolean;
  comparing: TDDComparison;
  data?: TableData;
};

export type TimeSeriesDataStore = Readable<TimeDimensionDataState>;

/***
 * Add totals row from time series data
 * Add rest of dimension values from dimension table data
 * Transpose the data to columnar format
 */
function prepareDimensionData(
  totalsData,
  data: DimensionDataItem[],
  total: number,
  unfilteredTotal: number,
  measure: MetricsViewSpecMeasureV2,
  selectedValues: string[],
  isAllTime: boolean
): TableData {
  if (!data || !totalsData) return;

  const formatter = safeFormatter(createMeasureValueFormatter(measure));
  const measureName = measure?.name;
  const validPercentOfTotal = measure?.validPercentOfTotal;

  const totalsTableData = isAllTime
    ? totalsData?.slice(1)
    : totalsData?.slice(1, -1);
  const columnHeaderData = totalsTableData?.map((v) => [{ value: v.ts }]);

  const columnCount = columnHeaderData?.length;

  // Add totals row to count
  const rowCount = data?.length + 1;

  const totalsRow = [
    { value: "Total" },
    {
      value: formatter(total),
      spark: createSparkline(totalsTableData, (v) => v[measureName]),
    },
  ];

  let fixedColCount = 2;
  if (validPercentOfTotal) {
    fixedColCount = 3;
    const percOfTotal = total / unfilteredTotal;
    totalsRow.push({
      value: isNaN(percOfTotal)
        ? "...%"
        : numberPartsToString(formatProperFractionAsPercent(percOfTotal)),
    });
  }

  let rowHeaderData = [totalsRow];

  rowHeaderData = rowHeaderData.concat(
    data?.map((row) => {
      const rowData = isAllTime ? row?.data?.slice(1) : row?.data?.slice(1, -1);
      const dataRow = [
        { value: row?.value },
        {
          value: formatter(row?.total),
          spark: createSparkline(rowData, (v) => v[measureName]),
        },
      ];
      if (validPercentOfTotal) {
        const percOfTotal = row?.total / unfilteredTotal;
        dataRow.push({
          value: isNaN(percOfTotal)
            ? "...%"
            : numberPartsToString(formatProperFractionAsPercent(percOfTotal)),
        });
      }
      return dataRow;
    })
  );

  let body = [totalsTableData?.map((v) => formatter(v[measureName])) || []];

  body = body?.concat(
    data?.map((v) => {
      if (v.isFetching) return new Array(columnCount).fill(undefined);
      const dimData = isAllTime ? v?.data?.slice(1) : v?.data?.slice(1, -1);
      return dimData?.map((v) => formatter(v[measureName]));
    })
  );
  /* 
    Important: regular-table expects body data in columnar format,
    aka an array of arrays where outer array is the columns,
    inner array is the row values for a specific column
  */
  const columnarBody = transposeArray(body, rowCount, columnCount);

  return {
    rowCount,
    fixedColCount,
    rowHeaderData,
    columnCount,
    columnHeaderData,
    body: columnarBody,
    selectedValues: selectedValues,
  };
}

/***
 * Add totals row from time series data
 * Add Current, Previous, Percentage Change, Absolute Change rows for time comparison
 * Transpose the data to columnar format
 */
function prepareTimeData(
  data,
  total: number,
  comparisonTotal: number,
  currentLabel: string,
  comparisonLabel: string,
  measure: MetricsViewSpecMeasureV2,
  hasTimeComparison,
  isAllTime: boolean
): TableData {
  if (!data) return;

  const formatter = safeFormatter(createMeasureValueFormatter(measure));
  const measureName = measure?.name ?? "";

  /** Strip out data points out of chart view */
  const tableData = isAllTime ? data?.slice(1) : data?.slice(1, -1);
  const columnHeaderData = tableData?.map((v) => [{ value: v.ts }]);

  const columnCount = columnHeaderData?.length;

  let rowHeaderData: unknown[] = [];
  rowHeaderData.push([
    { value: "Total" },
    {
      value: formatter(total),
      spark: createSparkline(tableData, (v) => v[measureName]),
    },
  ]);

  const body: unknown[] = [];

  if (hasTimeComparison) {
    rowHeaderData = rowHeaderData.concat([
      [
        { value: currentLabel },
        {
          value: formatter(total),
          spark: createSparkline(tableData, (v) => v[measureName]),
        },
      ],
      [
        { value: comparisonLabel },
        {
          value: formatter(comparisonTotal),
          spark: createSparkline(
            tableData,
            (v) => v[`comparison.${measureName}`]
          ),
        },
      ],
      [{ value: "Percentage Change" }],
      [{ value: "Absolute Change" }],
    ]);

    // Push totals
    body.push(
      tableData?.map((v) => {
        if (v[measureName] === null && v[`comparison.${measureName}`] === null)
          return null;
        return formatter(v[measureName] + v[`comparison.${measureName}`]);
      })
    );

    // Push current range
    body.push(tableData?.map((v) => formatter(v[measureName])));

    body.push(tableData?.map((v) => formatter(v[`comparison.${measureName}`])));

    // Push percentage change
    body.push(
      tableData?.map((v) => {
        const comparisonValue = v[`comparison.${measureName}`];
        const currentValue = v[measureName];
        const comparisonPercChange =
          comparisonValue && currentValue !== undefined && currentValue !== null
            ? (currentValue - comparisonValue) / comparisonValue
            : null;
        if (comparisonPercChange === null) return null;
        return numberPartsToString(
          formatMeasurePercentageDifference(comparisonPercChange)
        );
      })
    );

    // Push absolute change
    body.push(
      tableData?.map((v) => {
        const comparisonValue = v[`comparison.${measureName}`];
        const currentValue = v[measureName];
        const change =
          comparisonValue && currentValue !== undefined && currentValue !== null
            ? currentValue - comparisonValue
            : null;

        if (change === null) return null;
        return formatter(change);
      })
    );
  } else {
    body.push(tableData?.map((v) => formatter(v[measureName])));
  }

  const rowCount = rowHeaderData.length;
  const columnarBody = transposeArray(body, rowCount, columnCount);

  return {
    rowCount,
    fixedColCount: 2,
    rowHeaderData,
    columnCount,
    columnHeaderData,
    body: columnarBody,
    selectedValues: [],
  };
}

function createDimensionTableData(
  ctx: StateManagers
): Readable<DimensionDataItem[]> {
  return derived(ctx.dashboardStore, (dashboardStore, set) => {
    const measureName = dashboardStore?.expandedMeasureName;
    return derived(
      getDimensionValueTimeSeries(ctx, [measureName], "table"),
      (data) => data
    ).subscribe(set);
  });
}

/**
 * Memoized version of the table data. Currently, memoized by metrics view name.
 */
export const useDimensionTableData = memoizeMetricsStore<
  Readable<DimensionDataItem[]>
>((ctx: StateManagers) => createDimensionTableData(ctx));

export function createTimeDimensionDataStore(ctx: StateManagers) {
  return derived(
    [
      ctx.dashboardStore,
      useMetaQuery(ctx),
      useTimeControlStore(ctx),
      useTimeSeriesDataStore(ctx),
      useDimensionTableData(ctx),
    ],
    ([
      dashboardStore,
      metricsView,
      timeControls,
      timeSeries,
      tableDimensionData,
    ]) => {
      if (
        !timeControls.ready ||
        timeControls?.isFetching ||
        timeSeries?.isFetching
      )
        return;

      const measureName = dashboardStore?.expandedMeasureName;
      const dimensionName = dashboardStore?.selectedComparisonDimension;
      const total = timeSeries?.total && timeSeries?.total[measureName];
      const unfilteredTotal =
        timeSeries?.unfilteredTotal && timeSeries?.unfilteredTotal[measureName];
      const comparisonTotal =
        timeSeries?.comparisonTotal && timeSeries?.comparisonTotal[measureName];
      const isAllTime =
        timeControls?.selectedTimeRange?.name === TimeRangePreset.ALL_TIME;

      const measure = metricsView?.data?.measures?.find(
        (m) => m.name === measureName
      );

      let comparing;
      let data: TableData;

      if (dimensionName) {
        comparing = "dimension";

        const excludeMode =
          dashboardStore?.dimensionFilterExcludeMode.get(dimensionName) ??
          false;
        const selectedValues =
          ((excludeMode
            ? dashboardStore?.filters.exclude.find(
                (d) => d.name === dimensionName
              )?.in
            : dashboardStore?.filters.include.find(
                (d) => d.name === dimensionName
              )?.in) as string[]) ?? [];

        data = prepareDimensionData(
          timeSeries?.timeSeriesData,
          tableDimensionData,
          total,
          unfilteredTotal,
          measure,
          selectedValues,
          isAllTime
        );
      } else {
        comparing = timeControls.showComparison ? "time" : "none";
        const currentRange = timeControls?.selectedTimeRange?.name;

        let currentLabel = "Custom Range";
        if (currentRange in DEFAULT_TIME_RANGES)
          currentLabel = DEFAULT_TIME_RANGES[currentRange].label;

        const comparisonRange = timeControls?.selectedComparisonTimeRange?.name;
        let comparisonLabel = "Custom Range";

        if (comparisonRange in TIME_COMPARISON)
          comparisonLabel = TIME_COMPARISON[comparisonRange].label;

        data = prepareTimeData(
          timeSeries?.timeSeriesData,
          total,
          comparisonTotal,
          currentLabel,
          comparisonLabel,
          measure,
          comparing === "time",
          isAllTime
        );
      }

      return { isFetching: false, comparing, data };
    }
  ) as TimeSeriesDataStore;
}

/**
 * Memoized version of the store. Currently, memoized by metrics view name.
 */
export const useTimeDimensionDataStore =
  memoizeMetricsStore<TimeSeriesDataStore>((ctx: StateManagers) =>
    createTimeDimensionDataStore(ctx)
  );

/**
 * Stores for handling interactions between chart and table
 * Two separate stores created to avoid looped updates and renders
 */
export const tableInteractionStore = writable<HighlightedCell>({
  dimensionValue: undefined,
  time: undefined,
});

export const chartInteractionColumn = writable<ChartInteractionColumns>({
  hover: undefined,
  scrubStart: undefined,
  scrubEnd: undefined,
});

export const lastKnownPosition = writable<TablePosition>(undefined);
