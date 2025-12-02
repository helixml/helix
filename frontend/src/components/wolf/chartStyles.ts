/**
 * Shared chart styles for dark theme visualizations
 * Used by GPU memory charts, disk usage charts, and other time-series visualizations
 */

import { SxProps, Theme } from "@mui/material";

/**
 * Common styling for LineChart components in dark theme
 */
export const darkChartStyles: SxProps<Theme> = {
    "& .MuiChartsGrid-line": {
        stroke: "rgba(255, 255, 255, 0.1)",
        strokeDasharray: "3 3",
    },
    "& .MuiChartsAxis-tick": {
        stroke: "rgba(255, 255, 255, 0.6)",
    },
    "& .MuiChartsAxis-line": {
        stroke: "rgba(255, 255, 255, 0.6)",
    },
    "& .MuiChartsLegend-label": {
        fill: "rgba(255, 255, 255, 0.8)",
    },
};

/**
 * Common container styling for chart boxes
 */
export const chartContainerStyles: SxProps<Theme> = {
    backgroundColor: "rgba(0, 0, 0, 0.2)",
    borderRadius: 2,
    border: "1px solid rgba(255, 255, 255, 0.08)",
    p: 1.5,
    boxShadow: "inset 0 1px 3px rgba(0, 0, 0, 0.3)",
};

/**
 * Common legend props for chart components
 */
export const chartLegendProps = {
    direction: "row" as const,
    position: { vertical: "top" as const, horizontal: "right" as const },
    padding: 0,
    itemMarkWidth: 10,
    itemMarkHeight: 10,
    markGap: 5,
    itemGap: 15,
    labelStyle: {
        fontSize: 12,
        fill: "rgba(255, 255, 255, 0.8)",
    },
};

/**
 * Common axis label styling
 */
export const axisLabelStyle = {
    fontSize: "10px",
    fill: "rgba(255, 255, 255, 0.6)",
};

/**
 * Empty state box styling for when no data is available
 */
export const emptyStateStyles: SxProps<Theme> = {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    color: "rgba(255, 255, 255, 0.5)",
    backgroundColor: "rgba(0, 0, 0, 0.2)",
    borderRadius: 2,
    border: "1px solid rgba(255, 255, 255, 0.08)",
};
