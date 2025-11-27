import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import CircularProgress from "@mui/material/CircularProgress";
import { FC, useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { LineChart } from "@mui/x-charts";
import useApi from "../../hooks/useApi";
import { formatMB } from "../../utils/format";
import {
    darkChartStyles,
    chartContainerStyles,
    chartLegendProps,
    axisLabelStyle,
    emptyStateStyles,
} from "./chartStyles";

interface DiskUsageChartProps {
    sandboxInstanceId: string;
    height?: number;
}

const DiskUsageChart: FC<DiskUsageChartProps> = ({
    sandboxInstanceId,
    height = 300,
}) => {
    const api = useApi();
    const apiClient = api.getApiClient();
    const [hours, setHours] = useState(24);
    const [selectedMount, setSelectedMount] = useState<string>("all");

    const { data, isLoading, error } = useQuery({
        queryKey: ["disk-usage-history", sandboxInstanceId, hours],
        queryFn: async () => {
            const response = await apiClient.v1WolfInstancesDiskHistoryDetail(
                sandboxInstanceId,
                { hours }
            );
            return response.data;
        },
        enabled: !!sandboxInstanceId,
        refetchInterval: 30000, // Refresh every 30 seconds
    });

    // Get unique mount points for the dropdown
    const mountPoints = useMemo(() => {
        if (!data?.history?.length) return [];
        const mounts = new Set<string>();
        data.history.forEach((point) => {
            if (point.mount_point) mounts.add(point.mount_point);
        });
        return Array.from(mounts).sort();
    }, [data?.history]);

    // Filter data by selected mount point
    const filteredData = useMemo(() => {
        if (!data?.history?.length) return [];
        const filtered =
            selectedMount === "all"
                ? data.history
                : data.history.filter(
                      (point) => point.mount_point === selectedMount
                  );
        return filtered.sort(
            (a, b) =>
                new Date(a.timestamp || "").getTime() -
                new Date(b.timestamp || "").getTime()
        );
    }, [data?.history, selectedMount]);

    // Prepare chart data
    const chartData = useMemo(() => {
        return filteredData.map((point) => ({
            timestamp: new Date(point.timestamp || ""),
            used: point.used_mb || 0,
            total: point.total_mb || 0,
        }));
    }, [filteredData]);

    const xAxisData = chartData.map((point) => point.timestamp);
    const usedData = chartData.map((point) => point.used);
    const totalData = chartData.map((point) => point.total);

    const formatTimestamp = (value: Date) => {
        if (hours > 24) {
            return value.toLocaleDateString(undefined, {
                month: "short",
                day: "numeric",
                hour: "2-digit",
                minute: "2-digit",
            });
        }
        return value.toLocaleTimeString();
    };

    if (!sandboxInstanceId) {
        return (
            <Box sx={{ height, ...emptyStateStyles }}>
                <Typography>Select a sandbox to view disk usage history</Typography>
            </Box>
        );
    }

    if (isLoading) {
        return (
            <Box sx={{ height, display: "flex", alignItems: "center", justifyContent: "center" }}>
                <CircularProgress size={24} />
            </Box>
        );
    }

    if (error) {
        return (
            <Box sx={{ height, display: "flex", alignItems: "center", justifyContent: "center", color: "error.main" }}>
                <Typography>Failed to load disk usage history</Typography>
            </Box>
        );
    }

    if (!chartData.length) {
        return (
            <Box sx={{ height, ...emptyStateStyles }}>
                <Typography>No disk usage data available yet</Typography>
            </Box>
        );
    }

    const maxTotal = Math.max(...chartData.map((d) => d.total));

    return (
        <Box>
            {/* Controls */}
            <Box sx={{ display: "flex", gap: 2, mb: 2 }}>
                <FormControl size="small" sx={{ minWidth: 120 }}>
                    <InputLabel>Time Range</InputLabel>
                    <Select
                        value={hours}
                        label="Time Range"
                        onChange={(e) => setHours(e.target.value as number)}
                    >
                        <MenuItem value={1}>Last hour</MenuItem>
                        <MenuItem value={6}>Last 6 hours</MenuItem>
                        <MenuItem value={24}>Last 24 hours</MenuItem>
                        <MenuItem value={72}>Last 3 days</MenuItem>
                        <MenuItem value={168}>Last 7 days</MenuItem>
                    </Select>
                </FormControl>
                {mountPoints.length > 1 && (
                    <FormControl size="small" sx={{ minWidth: 120 }}>
                        <InputLabel>Mount Point</InputLabel>
                        <Select
                            value={selectedMount}
                            label="Mount Point"
                            onChange={(e) => setSelectedMount(e.target.value)}
                        >
                            <MenuItem value="all">All</MenuItem>
                            {mountPoints.map((mount) => (
                                <MenuItem key={mount} value={mount}>
                                    {mount}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                )}
            </Box>

            {/* Chart */}
            <Box sx={{ height, ...chartContainerStyles }}>
                <LineChart
                    xAxis={[
                        {
                            data: xAxisData,
                            scaleType: "time",
                            valueFormatter: formatTimestamp,
                            labelStyle: axisLabelStyle,
                        },
                    ]}
                    yAxis={[
                        {
                            valueFormatter: formatMB,
                            labelStyle: axisLabelStyle,
                            min: 0,
                            max: maxTotal * 1.1,
                        },
                    ]}
                    series={[
                        {
                            data: totalData,
                            label: "Total",
                            color: "#555",
                            showMark: false,
                            curve: "linear",
                        },
                        {
                            data: usedData,
                            label: "Used",
                            color: "#00c8ff",
                            showMark: false,
                            curve: "linear",
                            area: true,
                        },
                    ]}
                    height={height - 30}
                    margin={{ left: 70, right: 20, top: 40, bottom: 30 }}
                    grid={{ horizontal: true, vertical: false }}
                    sx={darkChartStyles}
                    slotProps={{ legend: chartLegendProps }}
                />
            </Box>

            {/* Container breakdown */}
            {data?.containers && data.containers.length > 0 && (
                <Box sx={{ mt: 3 }}>
                    <Typography variant="subtitle2" sx={{ mb: 1, color: "text.secondary" }}>
                        Container Disk Usage (latest)
                    </Typography>
                    <Table size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell>Container</TableCell>
                                <TableCell align="right">Size</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {data.containers
                                .sort((a, b) => (b.latest_size_mb || 0) - (a.latest_size_mb || 0))
                                .map((container) => (
                                    <TableRow key={container.container_name}>
                                        <TableCell sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}>
                                            {container.container_name}
                                        </TableCell>
                                        <TableCell align="right">
                                            {formatMB(container.latest_size_mb || 0)}
                                        </TableCell>
                                    </TableRow>
                                ))}
                        </TableBody>
                    </Table>
                </Box>
            )}
        </Box>
    );
};

export default DiskUsageChart;
