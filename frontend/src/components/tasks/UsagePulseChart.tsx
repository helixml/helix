import React, { useMemo, useId } from "react";
import { Box, Tooltip } from "@mui/material";
import useTheme from "@mui/material/styles/useTheme";
import { BarChart } from "@mui/x-charts";
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

  const { chartData, chartLabels, totalTokens } = useMemo(() => {
    if (!usageData || usageData.length === 0)
      return { chartData: null, chartLabels: [], totalTokens: 0 };
    const sortedData = [...usageData].sort(
      (a, b) =>
        new Date(a.date || "").getTime() - new Date(b.date || "").getTime(),
    );
    const data = sortedData.map(
      (d: ServerBatchTaskUsageMetric) => d.total_tokens || 0,
    );
    const labels = sortedData.map((d: ServerBatchTaskUsageMetric) => {
      const date = new Date(d.date || "");
      const month = String(date.getMonth() + 1).padStart(2, "0");
      const day = String(date.getDate()).padStart(2, "0");
      return `${month}/${day}`;
    });
    const total = data.reduce((a, b) => a + b, 0);
    return { chartData: data, chartLabels: labels, totalTokens: total };
  }, [usageData]);

  if (!chartData || chartData.length === 0 || totalTokens === 0) return null;

  return (
    <Tooltip title={`${totalTokens.toLocaleString()} tokens used`}>
      <Box
        sx={{
          width: "100%",
          height: 40,
          mt: 0.5,
          mb: 0.5,
        }}
      >
        <BarChart
          xAxis={[
            {
              data: chartLabels,
              scaleType: "band",
              tickLabelStyle: { display: "none" },
            },
          ]}
          yAxis={[
            {
              tickLabelStyle: { display: "none" },
            },
          ]}
          series={[
            {
              data: chartData,
              color: accentColor,
            },
          ]}
          height={40}
          slotProps={{
            legend: { hidden: true },
          }}
          sx={{
            width: "100%",
            "& .MuiChartsAxis-line": { display: "none" },
            "& .MuiChartsAxis-tick": { display: "none" },
          }}
          grid={{ horizontal: false, vertical: false }}
          margin={{ top: 2, bottom: 2, left: 0, right: 0 }}
          borderRadius={2}
        />
      </Box>
    </Tooltip>
  );
};

export default React.memo(UsagePulseChart);
