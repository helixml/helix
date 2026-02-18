import React, { FC, useState, useMemo } from "react";
import {
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Paper,
    Typography,
    Box,
    TextField,
    CircularProgress,
    Tooltip,
    Chip,
    InputAdornment,
    IconButton,
} from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import ClearIcon from "@mui/icons-material/Clear";
import { TypesOrgDetails } from "../../api/api";
import { useListAdminOrgs } from "../../services/dashboardService";

const AdminOrgsTable: FC = () => {
    const [searchQuery, setSearchQuery] = useState("");
    const { data: orgs, isLoading, error } = useListAdminOrgs();

    const filtered = useMemo(() => {
        if (!orgs) return [];
        if (!searchQuery.trim()) return orgs;
        const q = searchQuery.trim().toLowerCase();
        return orgs.filter((o: TypesOrgDetails) => {
            const name = (o.organization?.display_name || o.organization?.name || "").toLowerCase();
            return name.includes(q);
        });
    }, [orgs, searchQuery]);

    if (isLoading && !orgs) {
        return (
            <Paper
                sx={{
                    p: 2,
                    display: "flex",
                    justifyContent: "center",
                    alignItems: "center",
                    minHeight: 200,
                }}
            >
                <CircularProgress />
            </Paper>
        );
    }

    if (error) {
        return (
            <Paper sx={{ p: 2 }}>
                <Typography color="error">
                    Error loading organizations: {(error as Error).message}
                </Typography>
            </Paper>
        );
    }

    return (
        <Paper sx={{ width: "100%", overflow: "hidden" }}>
            <Box
                sx={{
                    p: 2,
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    gap: 2,
                }}
            >
                <TextField
                    label="Search organizations"
                    size="small"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    sx={{ minWidth: 300 }}
                    InputProps={{
                        startAdornment: (
                            <InputAdornment position="start">
                                <SearchIcon />
                            </InputAdornment>
                        ),
                        endAdornment: searchQuery && (
                            <InputAdornment position="end">
                                <IconButton
                                    aria-label="clear search"
                                    onClick={() => setSearchQuery("")}
                                    edge="end"
                                    size="small"
                                >
                                    <ClearIcon />
                                </IconButton>
                            </InputAdornment>
                        ),
                    }}
                />
                <Typography variant="body2" color="text.secondary">
                    {filtered.length} org{filtered.length !== 1 ? "s" : ""}
                </Typography>
            </Box>

            <TableContainer>
                <Table stickyHeader aria-label="organizations table">
                    <TableHead>
                        <TableRow>
                            <TableCell>Name</TableCell>
                            <TableCell>Projects</TableCell>
                            <TableCell>Members</TableCell>
                            <TableCell>Stripe Customer ID</TableCell>
                            <TableCell>Subscription</TableCell>
                            <TableCell align="right">Wallet Credits</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {filtered.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={6} align="center">
                                    <Typography variant="body2" color="text.secondary">
                                        {searchQuery ? "No organizations matching your search" : "No organizations found"}
                                    </Typography>
                                </TableCell>
                            </TableRow>
                        ) : (
                            filtered.map((org: TypesOrgDetails) => {
                                const name = org.organization?.display_name || org.organization?.name || "N/A";
                                const projects = org.projects || [];
                                const members = org.members || [];
                                const balance = org.wallet?.balance;

                                return (
                                    <TableRow key={org.organization?.id} hover>
                                        <TableCell>
                                            <Typography variant="body2" sx={{ fontWeight: "medium" }}>
                                                {name}
                                            </Typography>
                                            {org.organization?.name && org.organization?.display_name && org.organization.name !== org.organization.display_name && (
                                                <Typography variant="caption" color="text.secondary">
                                                    {org.organization.name}
                                                </Typography>
                                            )}
                                        </TableCell>
                                        <TableCell>
                                            {projects.length === 0 ? (
                                                <Typography variant="body2" color="text.secondary">None</Typography>
                                            ) : (
                                                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                                                    {projects.map((p) => (
                                                        <Chip
                                                            key={p.id}
                                                            label={p.name || p.id}
                                                            size="small"
                                                            variant="outlined"
                                                        />
                                                    ))}
                                                </Box>
                                            )}
                                        </TableCell>
                                        <TableCell>
                                            {members.length === 0 ? (
                                                <Typography variant="body2" color="text.secondary">None</Typography>
                                            ) : (
                                                <Tooltip
                                                    title={members.map((m) => m.email || m.username || m.id).join(", ")}
                                                >
                                                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                                                        {members.map((m) => (
                                                            <Chip
                                                                key={m.id}
                                                                label={m.email || m.username || m.id}
                                                                size="small"
                                                                variant="outlined"
                                                            />
                                                        ))}
                                                    </Box>
                                                </Tooltip>
                                            )}
                                        </TableCell>
                                        <TableCell>
                                            <Typography variant="body2" sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}>
                                                {org.wallet?.stripe_customer_id || "N/A"}
                                            </Typography>
                                        </TableCell>
                                        <TableCell>
                                            {org.wallet?.subscription_status ? (
                                                <Chip
                                                    label={org.wallet.subscription_status}
                                                    size="small"
                                                    color={org.wallet.subscription_status === "active" ? "success" : "default"}
                                                    variant={org.wallet.subscription_status === "active" ? "filled" : "outlined"}
                                                />
                                            ) : (
                                                <Typography variant="body2" color="text.secondary">None</Typography>
                                            )}
                                        </TableCell>
                                        <TableCell align="right">
                                            <Typography variant="body2">
                                                {balance !== undefined ? `$${balance.toFixed(2)}` : "N/A"}
                                            </Typography>
                                        </TableCell>
                                    </TableRow>
                                );
                            })
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
        </Paper>
    );
};

export default AdminOrgsTable;
