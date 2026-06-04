import React, { useMemo, useId } from "react";
import { Box, Tooltip } from "@mui/material";
import useTheme from "@mui/material/styles/useTheme";
import { Area, AreaChart, ResponsiveContainer } from "recharts";
import { ServerBatchTaskUsageMetric } from "../../api/api";

interface UsagePulseChartProps {
  accentColor: string;
  /** Usage data from batch endpoint - required */
  usageData?: ServerBatchTaskUsageMetric[];
}

const UsagePulseChart: React.FC<UsagePulseChartProps> = ({
  accentColor,
  usageData,
}) => {
  const theme = useTheme();

  const { chartData, totalTokens } = useMemo(() => {
    if (!usageData || usageData.length === 0)
      return { chartData: null, totalTokens: 0 };
    const sortedData = [...usageData].sort(
      (a, b) =>
        new Date(a.date || "").getTime() - new Date(b.date || "").getTime(),
    );
    let points = sortedData.map((d: ServerBatchTaskUsageMetric) => ({
      label: (() => {
        const date = new Date(d.date || "");
        const month = String(date.getMonth() + 1).padStart(2, "0");
        const day = String(date.getDate()).padStart(2, "0");
        const hours = String(date.getHours()).padStart(2, "0");
        const minutes = String(date.getMinutes()).padStart(2, "0");
        return `${month}/${day} ${hours}:${minutes}`;
      })(),
      value: d.total_tokens || 0,
    }));
    // Pad with zeroes so the line chart can draw a visible spike
    // (a single point can't render a line)
    if (points.length < 3) {
      points = [{ label: "", value: 0 }, ...points, { label: "", value: 0 }];
    }
    const total = points.reduce((a, p) => a + p.value, 0);
    return { chartData: points, totalTokens: total };
  }, [usageData]);

  const uniqueId = useId();

  if (!chartData || chartData.length === 0 || totalTokens === 0) return null;

  const gradientId = `usageGradient-${uniqueId}`;

  return (
    <Tooltip title={`${totalTokens.toLocaleString()} tokens used`}>
      <Box
        sx={{
          width: "100%",
          height: 30,
        }}
      >
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={chartData} margin={{ top: 2, bottom: 2, left: 0, right: 0 }}>
            <defs>
              <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                <stop
                  offset="0%"
                  stopColor={theme.chartGradientStart}
                  stopOpacity={theme.chartGradientStartOpacity}
                />
                <stop
                  offset="100%"
                  stopColor={theme.chartGradientEnd}
                  stopOpacity={theme.chartGradientEndOpacity}
                />
              </linearGradient>
            </defs>
            <Area
              type="monotone"
              dataKey="value"
              stroke={accentColor}
              strokeWidth={2}
              fill={`url(#${gradientId})`}
              isAnimationActive={false}
              dot={false}
              activeDot={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </Box>
    </Tooltip>
  );
};

export default React.memo(UsagePulseChart);
