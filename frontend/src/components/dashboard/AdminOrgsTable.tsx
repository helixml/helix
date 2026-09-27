import React, { FC, useEffect, useMemo, useState } from "react";
import {
    Paper,
    Typography,
    Box,
    TextField,
    InputAdornment,
    IconButton,
    Menu,
    MenuItem,
    Chip,
    Tooltip,
    TablePagination,
} from "@mui/material";
import { Search, X, EllipsisVertical, Gift, Coins, CircleSlash, BadgeDollarSign } from "lucide-react";
import { TypesOrgDetails } from "../../api/api";
import { useListAdminOrgs } from "../../services/dashboardService";
import SimpleTable from "../widgets/SimpleTable";
import AdminOrgBillingDialog, { OrgBillingAction } from "./AdminOrgBillingDialog";
import AdminOrgPlanDialog from "./AdminOrgPlanDialog";

// Dedicated subscription status chip so the wallet state reads at a glance.
const SubscriptionChip: FC<{ status?: string }> = ({ status }) => {
    if (!status) {
        return (
            <Typography variant="body2" color="text.secondary">
                None
            </Typography>
        );
    }
    const color = status === "active" ? "success" : status === "trialing" ? "warning" : "default";
    const label = status.charAt(0).toUpperCase() + status.slice(1);
    return <Chip label={label} size="small" color={color} variant={status === "active" ? "filled" : "outlined"} />;
};

// Plan cell: forced overrides get a chip + caption; the default state is
// spelled out as "Auto" so "no override" is explicit rather than a dash.
const PlanCell: FC<{ override?: string }> = ({ override }) => {
    if (!override) {
        return (
            <Box>
                <Typography variant="body2" color="text.secondary">
                    Auto
                </Typography>
                <Typography variant="caption" color="text.secondary">
                    from Stripe
                </Typography>
            </Box>
        );
    }
    return (
        <Box>
            <Chip
                label={override}
                size="small"
                color={override === "pro" ? "success" : "default"}
                variant="filled"
            />
            <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                forced
            </Typography>
        </Box>
    );
};

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

    const [actionAnchor, setActionAnchor] = useState<null | HTMLElement>(null);
    const [actionOrg, setActionOrg] = useState<TypesOrgDetails | null>(null);
    const [billingDialog, setBillingDialog] = useState<{ action: OrgBillingAction; org: TypesOrgDetails } | null>(null);
    const [planDialogOrg, setPlanDialogOrg] = useState<TypesOrgDetails | null>(null);

    const openActionMenu = (e: React.MouseEvent<HTMLElement>, org: TypesOrgDetails) => {
        e.stopPropagation();
        setActionAnchor(e.currentTarget);
        setActionOrg(org);
    };
    const closeActionMenu = () => {
        setActionAnchor(null);
        setActionOrg(null);
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

    const tableData = useMemo(() => {
        return (orgs ?? []).map((org: TypesOrgDetails) => {
            const name = org.organization?.display_name || org.organization?.name || "N/A";
            const projects = org.projects || [];
            const members = org.members || [];

            const memberLabel = (member: (typeof members)[number]) => {
                const user = member.user;
                const name = user?.email || user?.username || user?.id || member.user_id;
                const role = member.role ? member.role.charAt(0).toUpperCase() + member.role.slice(1) : "Member";
                return `${name} (${role})`;
            };

            return {
                id: org.organization?.id || "",
                _data: org,
                name: (
                    <Box>
                        <Typography variant="body2" sx={{ fontWeight: "bold" }}>
                            {name}
                        </Typography>
                        {org.organization?.name && org.organization?.display_name && org.organization.name !== org.organization.display_name && (
                            <Typography variant="caption" color="text.secondary">
                                {org.organization.name}
                            </Typography>
                        )}
                    </Box>
                ),
                projects: projects.length === 0 ? (
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
                ),
                members: members.length === 0 ? (
                    <Typography variant="body2" color="text.secondary">None</Typography>
                ) : (
                    <Tooltip
                        title={members.map(memberLabel).join(", ")}
                    >
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                            {members.map((m) => (
                                <Chip
                                    key={m.user_id}
                                    label={memberLabel(m)}
                                    size="small"
                                    variant="outlined"
                                />
                            ))}
                        </Box>
                    </Tooltip>
                ),
                subscription: <SubscriptionChip status={org.wallet?.subscription_status} />,
                plan: <PlanCell override={org.wallet?.plan_override} />,
                balance: (
                    <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                        {org.wallet?.balance !== undefined ? `$${org.wallet.balance.toFixed(2)}` : "N/A"}
                    </Typography>
                ),
            };
        });
    }, [orgs]);

    const getActions = (row: Record<string, any>) => (
        <IconButton
            size="small"
            aria-label="organization actions"
            onClick={(e) => openActionMenu(e, row._data as TypesOrgDetails)}
        >
            <EllipsisVertical size={18} />
        </IconButton>
    );

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
                    label="Search organizations or owner email"
                    size="small"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    sx={{ minWidth: 300 }}
                    InputProps={{
                        startAdornment: (
                            <InputAdornment position="start">
                                <Search size={18} />
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
                                    <X size={18} />
                                </IconButton>
                            </InputAdornment>
                        ),
                    }}
                />
                <Typography variant="body2" color="text.secondary">
                    {data?.totalCount ?? 0} org{data?.totalCount !== 1 ? "s" : ""}
                </Typography>
            </Box>

            <SimpleTable
                authenticated={true}
                loading={isLoading}
                fields={[
                    { name: "name", title: "Name" },
                    { name: "projects", title: "Projects" },
                    { name: "members", title: "Members" },
                    { name: "subscription", title: "Subscription" },
                    { name: "plan", title: "Plan" },
                    { name: "balance", title: "Wallet Credits", numeric: true },
                ]}
                data={tableData}
                getActions={getActions}
            />

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
                <MenuItem onClick={() => { setPlanDialogOrg(actionOrg); closeActionMenu(); }}>
                    <BadgeDollarSign size={16} style={{ marginRight: 8 }} />
                    Set plan…
                </MenuItem>
                {actionOrg?.wallet?.subscription_status !== "trialing" &&
                    actionOrg?.wallet?.subscription_status !== "active" && (
                        <MenuItem onClick={() => openBillingDialog("activate")}>
                            <Gift size={16} style={{ marginRight: 8 }} />
                            Activate trial…
                        </MenuItem>
                    )}
                {actionOrg?.wallet?.subscription_status === "trialing" && (
                    <MenuItem onClick={() => openBillingDialog("revoke")}>
                        <CircleSlash size={16} style={{ marginRight: 8 }} />
                        Revoke trial…
                    </MenuItem>
                )}
                <MenuItem onClick={() => openBillingDialog("credits")}>
                    <Coins size={16} style={{ marginRight: 8 }} />
                    Grant credits…
                </MenuItem>
            </Menu>

            <AdminOrgBillingDialog
                open={Boolean(billingDialog)}
                action={billingDialog?.action || "activate"}
                org={billingDialog?.org || null}
                onClose={() => setBillingDialog(null)}
            />
            <AdminOrgPlanDialog
                open={Boolean(planDialogOrg)}
                org={planDialogOrg}
                onClose={() => setPlanDialogOrg(null)}
            />
        </Paper>
    );
};

export default AdminOrgsTable;
