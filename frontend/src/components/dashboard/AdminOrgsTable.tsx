import React, { FC, useEffect, useMemo, useState } from "react";
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
    Menu,
    MenuItem,
    TablePagination,
} from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import ClearIcon from "@mui/icons-material/Clear";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import { TypesOrgDetails } from "../../api/api";
import { useListAdminOrgs, useAdminSetOrgPlan } from "../../services/dashboardService";
import useSnackbar from "../../hooks/useSnackbar";
import AdminOrgBillingDialog, { OrgBillingAction } from "./AdminOrgBillingDialog";

const AdminOrgsTable: FC = () => {
    const [searchQuery, setSearchQuery] = useState("");
    const [debouncedSearchQuery, setDebouncedSearchQuery] = useState("");
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(25);
    const query = useMemo(() => ({
        page: page + 1,
        per_page: rowsPerPage,
        ...(debouncedSearchQuery.trim() ? { query: debouncedSearchQuery.trim() } : {}),
    }), [page, rowsPerPage, debouncedSearchQuery]);
    const { data, isLoading, error } = useListAdminOrgs(query);
    const orgs = data?.organizations;
    const setOrgPlan = useAdminSetOrgPlan();
    const snackbar = useSnackbar();
    const [actionAnchor, setActionAnchor] = useState<null | HTMLElement>(null);
    const [actionOrg, setActionOrg] = useState<TypesOrgDetails | null>(null);
    const [billingDialog, setBillingDialog] = useState<{ action: OrgBillingAction; org: TypesOrgDetails } | null>(null);

    const openActionMenu = (e: React.MouseEvent<HTMLElement>, org: TypesOrgDetails) => {
        setActionAnchor(e.currentTarget);
        setActionOrg(org);
    };
    const closeActionMenu = () => {
        setActionAnchor(null);
        setActionOrg(null);
    };
    const applyPlan = (plan: string) => {
        const orgId = actionOrg?.organization?.id;
        if (orgId) {
            setOrgPlan.mutate({ orgId, plan }, {
                onSuccess: () => snackbar.success(plan ? `Plan set to ${plan} on ${actionOrg?.organization?.display_name || actionOrg?.organization?.name}` : `Plan override cleared on ${actionOrg?.organization?.display_name || actionOrg?.organization?.name}`),
                onError: (err: any) => snackbar.error(err?.response?.data?.error || err?.message || "Failed to set plan"),
            });
        }
        closeActionMenu();
    };
    const openBillingDialog = (action: OrgBillingAction) => {
        if (actionOrg) setBillingDialog({ action, org: actionOrg });
        closeActionMenu();
    };

    useEffect(() => {
        const timer = setTimeout(() => {
            setDebouncedSearchQuery(searchQuery);
            setPage(0);
        }, 300);
        return () => clearTimeout(timer);
    }, [searchQuery]);

    const filtered = orgs ?? [];

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
                    {data?.totalCount ?? 0} org{data?.totalCount !== 1 ? "s" : ""}
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
                            <TableCell>Plan</TableCell>
                            <TableCell align="right">Wallet Credits</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {filtered.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={7} align="center">
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
                                        <TableCell>
                                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                                                {org.wallet?.plan_override ? (
                                                    <Chip
                                                        label={org.wallet.plan_override}
                                                        size="small"
                                                        color={org.wallet.plan_override === "pro" ? "success" : "default"}
                                                    />
                                                ) : (
                                                    <Typography variant="body2" color="text.secondary">—</Typography>
                                                )}
                                                <IconButton
                                                    size="small"
                                                    aria-label="organization actions"
                                                    onClick={(e) => openActionMenu(e, org)}
                                                >
                                                    <MoreVertIcon fontSize="small" />
                                                </IconButton>
                                            </Box>
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

            {(data?.totalCount ?? 0) > 0 && (
                <TablePagination
                    rowsPerPageOptions={[10, 25, 50, 100]}
                    component="div"
                    count={data?.totalCount ?? 0}
                    rowsPerPage={rowsPerPage}
                    page={page}
                    onPageChange={(_event, newPage) => setPage(newPage)}
                    onRowsPerPageChange={(event) => {
                        setRowsPerPage(parseInt(event.target.value, 10));
                        setPage(0);
                    }}
                    labelRowsPerPage="Organizations per page:"
                />
            )}

            <Menu anchorEl={actionAnchor} open={Boolean(actionAnchor)} onClose={closeActionMenu}>
                <MenuItem onClick={() => applyPlan("pro")}>Set Pro (paid, no Stripe)</MenuItem>
                <MenuItem onClick={() => applyPlan("free")}>Set Free</MenuItem>
                <MenuItem onClick={() => applyPlan("")}>Clear override (use Stripe)</MenuItem>
                {actionOrg?.wallet?.subscription_status !== "trialing" &&
                    actionOrg?.wallet?.subscription_status !== "active" && (
                        <MenuItem onClick={() => openBillingDialog("activate")}>Activate trial…</MenuItem>
                    )}
                {actionOrg?.wallet?.subscription_status === "trialing" && (
                    <MenuItem onClick={() => openBillingDialog("revoke")}>Revoke trial…</MenuItem>
                )}
                <MenuItem onClick={() => openBillingDialog("credits")}>Grant credits…</MenuItem>
            </Menu>

            <AdminOrgBillingDialog
                open={Boolean(billingDialog)}
                action={billingDialog?.action || "activate"}
                org={billingDialog?.org || null}
                onClose={() => setBillingDialog(null)}
            />
        </Paper>
    );
};

export default AdminOrgsTable;
