import React, { FC } from "react";
import {
    Box,
    Paper,
    Typography,
    Button,
    CircularProgress,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Chip,
    Stack,
    Tooltip,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { useUserStats } from "../../services/dashboardService";

interface UserDetailPanelProps {
    userId: string;
    onBack: () => void;
}

const formatFullDate = (dateString?: string): string => {
    if (!dateString) return "Never";
    const date = new Date(dateString);
    const ts = date.getTime();
    if (!isFinite(ts) || ts <= 0) return "Never";
    return date.toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
    });
};

const formatRelative = (dateString?: string): string => {
    if (!dateString) return "Never";
    const ts = new Date(dateString).getTime();
    if (!isFinite(ts) || ts <= 0) return "Never";
    const diffMs = Date.now() - ts;
    if (diffMs < 0) return "just now";
    const minutes = Math.floor(diffMs / 60_000);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d ago`;
    const months = Math.floor(days / 30);
    if (months < 12) return `${months}mo ago`;
    const years = Math.floor(days / 365);
    return `${years}y ago`;
};

const formatNumber = (value?: number): string =>
    typeof value === "number" ? value.toLocaleString() : "0";

const formatCost = (value?: number): string => {
    const v = value ?? 0;
    if (v === 0) return "$0.00";
    if (v < 0.01) return `$${v.toFixed(4)}`;
    return `$${v.toFixed(2)}`;
};

const StatCard: FC<{ label: string; value: React.ReactNode; hint?: string }> = ({
    label,
    value,
    hint,
}) => (
    <Paper
        variant="outlined"
        sx={{ p: 2, flex: 1, minWidth: 180, display: "flex", flexDirection: "column", gap: 0.5 }}
    >
        <Typography variant="caption" color="text.secondary" sx={{ textTransform: "uppercase" }}>
            {label}
        </Typography>
        <Typography variant="h5" sx={{ fontVariantNumeric: "tabular-nums" }}>
            {value}
        </Typography>
        {hint && (
            <Typography variant="caption" color="text.secondary">
                {hint}
            </Typography>
        )}
    </Paper>
);

const UserDetailPanel: FC<UserDetailPanelProps> = ({ userId, onBack }) => {
    const { data, isLoading, error } = useUserStats(userId);

    if (isLoading) {
        return (
            <Box sx={{ p: 2, display: "flex", justifyContent: "center" }}>
                <CircularProgress />
            </Box>
        );
    }

    if (error) {
        return (
            <Paper sx={{ p: 2 }}>
                <Button startIcon={<ArrowBackIcon />} onClick={onBack} sx={{ mb: 2 }}>
                    Back to users
                </Button>
                <Typography color="error">
                    Error loading user stats: {(error as Error).message}
                </Typography>
            </Paper>
        );
    }

    if (!data) {
        return (
            <Paper sx={{ p: 2 }}>
                <Button startIcon={<ArrowBackIcon />} onClick={onBack} sx={{ mb: 2 }}>
                    Back to users
                </Button>
                <Typography>User not found.</Typography>
            </Paper>
        );
    }

    const user = data.user;
    const models = data.models ?? [];
    const totalRequests = models.reduce((acc, m) => acc + (m.total_requests ?? 0), 0);
    const totalTokens = models.reduce((acc, m) => acc + (m.total_tokens ?? 0), 0);
    const totalCost = models.reduce((acc, m) => acc + (m.total_cost ?? 0), 0);

    return (
        <Box>
            <Box sx={{ mb: 2, display: "flex", alignItems: "center", gap: 2 }}>
                <Button startIcon={<ArrowBackIcon />} onClick={onBack}>
                    Back to users
                </Button>
            </Box>

            <Paper sx={{ p: 3, mb: 2 }}>
                <Stack direction={{ xs: "column", md: "row" }} spacing={2} alignItems="flex-start">
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="h5" sx={{ fontWeight: 500 }}>
                            {user?.full_name || user?.username || user?.email || user?.id}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            {user?.email}
                        </Typography>
                        <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: "wrap" }}>
                            {user?.admin && <Chip label="Admin" size="small" color="error" />}
                            {user?.waitlisted && <Chip label="Waitlisted" size="small" color="warning" />}
                            {user?.deactivated && <Chip label="Deactivated" size="small" color="default" />}
                            {user?.auth_provider && (
                                <Chip label={String(user.auth_provider)} size="small" variant="outlined" />
                            )}
                        </Stack>
                        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
                            ID: {user?.id}
                        </Typography>
                    </Box>

                    <Stack spacing={0.5} sx={{ minWidth: 220 }}>
                        <Typography variant="caption" color="text.secondary" sx={{ textTransform: "uppercase" }}>
                            Last Active
                        </Typography>
                        <Tooltip title={formatFullDate(data.last_active_at)}>
                            <Typography variant="body1" sx={{ fontWeight: 500 }}>
                                {formatRelative(data.last_active_at)}
                            </Typography>
                        </Tooltip>
                        <Typography variant="caption" color="text.secondary">
                            Auth: {formatRelative(user?.last_seen_at)} · Usage: {models.length > 0 ? "tracked" : "none"}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                            Joined: {formatFullDate(user?.created_at)}
                        </Typography>
                    </Stack>
                </Stack>
            </Paper>

            <Stack direction={{ xs: "column", md: "row" }} spacing={2} sx={{ mb: 2 }}>
                <StatCard label="Projects" value={formatNumber(data.projects_count)} />
                <StatCard label="Spec Tasks" value={formatNumber(data.spec_tasks_count)} />
                <StatCard label="Total Requests" value={formatNumber(totalRequests)} hint="Across all models" />
                <StatCard label="Total Tokens" value={formatNumber(totalTokens)} />
                <StatCard label="Total Cost" value={formatCost(totalCost)} />
            </Stack>

            <Paper sx={{ width: "100%", overflow: "hidden" }}>
                <Box sx={{ p: 2 }}>
                    <Typography variant="h6">Model Usage</Typography>
                    <Typography variant="body2" color="text.secondary">
                        All inference this user has driven, grouped by provider and model.
                    </Typography>
                </Box>
                <TableContainer>
                    <Table size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell sx={{ fontWeight: 600 }}>Provider</TableCell>
                                <TableCell sx={{ fontWeight: 600 }}>Model</TableCell>
                                <TableCell sx={{ fontWeight: 600 }} align="right">Requests</TableCell>
                                <TableCell sx={{ fontWeight: 600 }} align="right">Input</TableCell>
                                <TableCell sx={{ fontWeight: 600 }} align="right">Output</TableCell>
                                <TableCell sx={{ fontWeight: 600 }} align="right">Cache R/W</TableCell>
                                <TableCell sx={{ fontWeight: 600 }} align="right">Total Tokens</TableCell>
                                <TableCell sx={{ fontWeight: 600 }} align="right">Cost</TableCell>
                                <TableCell sx={{ fontWeight: 600 }}>Last Used</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {models.length === 0 ? (
                                <TableRow>
                                    <TableCell colSpan={9} align="center" sx={{ py: 4 }}>
                                        <Typography variant="body2" color="text.secondary">
                                            No inference usage recorded for this user.
                                        </Typography>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                models.map((model) => (
                                    <TableRow key={`${model.provider}-${model.model}`} hover>
                                        <TableCell>{model.provider || "—"}</TableCell>
                                        <TableCell>
                                            <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                                                {model.model || "—"}
                                            </Typography>
                                        </TableCell>
                                        <TableCell align="right" sx={{ fontVariantNumeric: "tabular-nums" }}>
                                            {formatNumber(model.total_requests)}
                                        </TableCell>
                                        <TableCell align="right" sx={{ fontVariantNumeric: "tabular-nums" }}>
                                            {formatNumber(model.prompt_tokens)}
                                        </TableCell>
                                        <TableCell align="right" sx={{ fontVariantNumeric: "tabular-nums" }}>
                                            {formatNumber(model.completion_tokens)}
                                        </TableCell>
                                        <TableCell align="right" sx={{ fontVariantNumeric: "tabular-nums" }}>
                                            <Tooltip
                                                title={`Read: ${formatNumber(model.cache_read_tokens)} · Write: ${formatNumber(model.cache_write_tokens)}`}
                                            >
                                                <span>
                                                    {formatNumber((model.cache_read_tokens ?? 0) + (model.cache_write_tokens ?? 0))}
                                                </span>
                                            </Tooltip>
                                        </TableCell>
                                        <TableCell align="right" sx={{ fontVariantNumeric: "tabular-nums", fontWeight: 600 }}>
                                            {formatNumber(model.total_tokens)}
                                        </TableCell>
                                        <TableCell align="right" sx={{ fontVariantNumeric: "tabular-nums" }}>
                                            {formatCost(model.total_cost)}
                                        </TableCell>
                                        <TableCell>
                                            <Tooltip title={formatFullDate(model.last_used)}>
                                                <Typography variant="body2" color="text.secondary">
                                                    {formatRelative(model.last_used)}
                                                </Typography>
                                            </Tooltip>
                                        </TableCell>
                                    </TableRow>
                                ))
                            )}
                        </TableBody>
                    </Table>
                </TableContainer>
            </Paper>
        </Box>
    );
};

export default UserDetailPanel;
