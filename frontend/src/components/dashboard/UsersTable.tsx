import React, { FC, useState, useMemo, useContext } from "react";
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
    TablePagination,
    InputAdornment,
    IconButton,
    Button,
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
} from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import ClearIcon from "@mui/icons-material/Clear";
import AddIcon from "@mui/icons-material/Add";
import LockResetIcon from "@mui/icons-material/LockReset";
import DeleteIcon from "@mui/icons-material/Delete";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import CardGiftcardIcon from "@mui/icons-material/CardGiftcard";
import CancelIcon from "@mui/icons-material/Cancel";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import { TypesUser } from "../../api/api";
import {
    useListUsers,
    useAdminApproveUser,
    useAdminRevokeTrial,
    UserListQuery,
} from "../../services/dashboardService";
import { AccountContext } from "../../contexts/account";
import useSnackbar from "../../hooks/useSnackbar";
import CreateUserDialog from "./CreateUserDialog";
import ResetPasswordDialog from "./ResetPasswordDialog";
import DeleteUserDialog from "./DeleteUserDialog";
import ActivateTrialDialog from "./ActivateTrialDialog";

// Helper function to format date for tooltip
const formatFullDate = (dateString: string | undefined): string => {
    if (!dateString) return "N/A";
    try {
        const date = new Date(dateString);
        const hours = date.getHours().toString().padStart(2, "0");
        const minutes = date.getMinutes().toString().padStart(2, "0");
        const day = date.getDate();
        const monthNames = [
            "Jan", "Feb", "Mar", "Apr", "May", "Jun",
            "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
        ];
        const month = monthNames[date.getMonth()];
        const year = date.getFullYear();
        let daySuffix = "th";
        if (day % 10 === 1 && day !== 11) daySuffix = "st";
        else if (day % 10 === 2 && day !== 12) daySuffix = "nd";
        else if (day % 10 === 3 && day !== 13) daySuffix = "rd";
        return `${hours}:${minutes} ${day}${daySuffix} ${month}, ${year}`;
    } catch (error) {
        return "Invalid Date";
    }
};

const formatShortDate = (dateString: string | undefined): string => {
    if (!dateString) return "N/A";
    try {
        const date = new Date(dateString);
        const year = date.getFullYear();
        const month = (date.getMonth() + 1).toString().padStart(2, "0");
        const day = date.getDate().toString().padStart(2, "0");
        const hours = date.getHours().toString().padStart(2, "0");
        const minutes = date.getMinutes().toString().padStart(2, "0");
        return `${year}-${month}-${day} ${hours}:${minutes}`;
    } catch (error) {
        return "Invalid Date";
    }
};

const formatRelative = (dateString: string | undefined): string => {
    if (!dateString) return "Never";
    const date = new Date(dateString);
    const ts = date.getTime();
    if (!isFinite(ts) || ts <= 0) return "Never";
    const diffMs = Date.now() - ts;
    if (diffMs < 0) return "just now";
    const seconds = Math.floor(diffMs / 1000);
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
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

type WaitlistFilter = "all" | "waitlisted" | "active";

const TrialChip: FC<{ user: TypesUser }> = ({ user }) => {
    const status = user.trial_status;
    if (status === "stashed") {
        const days = user.trial_days_on_first_org;
        return (
            <Tooltip title="Trial granted but the user has not yet created an organization.">
                <Chip
                    label={days ? `Pending (${days}d)` : "Pending"}
                    size="small"
                    color="warning"
                    variant="outlined"
                />
            </Tooltip>
        );
    }
    if (status === "active") {
        const endsAt = user.trial_ends_at;
        let label = "Active";
        let tooltip = "Trial subscription is currently active.";
        if (endsAt) {
            const remainingDays = Math.max(0, Math.ceil((endsAt * 1000 - Date.now()) / 86400000));
            label = `${remainingDays}d remaining`;
            tooltip = `Trial ends ${formatFullDate(new Date(endsAt * 1000).toISOString())}`;
        }
        return (
            <Tooltip title={tooltip}>
                <Chip label={label} size="small" color="success" variant="outlined" />
            </Tooltip>
        );
    }
    return (
        <Typography variant="body2" color="text.secondary">
            —
        </Typography>
    );
};

interface UsersTableProps {
    onSelectUser?: (userId: string) => void;
}

const UsersTable: FC<UsersTableProps> = ({ onSelectUser }) => {
    const account = useContext(AccountContext);
    const isCloud = account.serverConfig?.edition === "cloud";

    const [searchQuery, setSearchQuery] = useState("");
    const [waitlistFilter, setWaitlistFilter] = useState<WaitlistFilter>("all");
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(25);
    const [createDialogOpen, setCreateDialogOpen] = useState(false);
    const [resetPasswordDialogOpen, setResetPasswordDialogOpen] = useState(false);
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [activateTrialDialogOpen, setActivateTrialDialogOpen] = useState(false);
    const [selectedUser, setSelectedUser] = useState<TypesUser | null>(null);
    const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);
    const [menuUser, setMenuUser] = useState<TypesUser | null>(null);

    const approveUser = useAdminApproveUser();
    const revokeTrial = useAdminRevokeTrial();
    const snackbar = useSnackbar();

    const openMenu = (e: React.MouseEvent<HTMLElement>, user: TypesUser) => {
        e.stopPropagation();
        setMenuAnchor(e.currentTarget);
        setMenuUser(user);
    };
    const closeMenu = () => {
        setMenuAnchor(null);
        setMenuUser(null);
    };

    const handleApprove = async () => {
        if (!menuUser?.id) return;
        const u = menuUser;
        closeMenu();
        try {
            await approveUser.mutateAsync(u.id!);
            snackbar.success(`User ${u.email || u.username} approved`);
        } catch (err: any) {
            snackbar.error(err?.message || "Failed to approve user");
        }
    };

    const handleActivateTrial = () => {
        setSelectedUser(menuUser);
        closeMenu();
        setActivateTrialDialogOpen(true);
    };

    const handleRevokeTrial = async () => {
        if (!menuUser?.id) return;
        const u = menuUser;
        closeMenu();
        try {
            await revokeTrial.mutateAsync(u.id!);
            snackbar.success(`Trial revoked for ${u.email || u.username}`);
        } catch (err: any) {
            snackbar.error(err?.response?.data?.error || err?.message || "Failed to revoke trial");
        }
    };

    const handleResetPassword = () => {
        setSelectedUser(menuUser);
        closeMenu();
        setResetPasswordDialogOpen(true);
    };

    const handleDeleteUser = () => {
        setSelectedUser(menuUser);
        closeMenu();
        setDeleteDialogOpen(true);
    };

    const handleRowClick = (user: TypesUser) => {
        if (user.id && onSelectUser) onSelectUser(user.id);
    };

    const [debouncedSearchQuery, setDebouncedSearchQuery] = useState("");
    React.useEffect(() => {
        const timer = setTimeout(() => {
            setDebouncedSearchQuery(searchQuery);
            setPage(0);
        }, 300);
        return () => clearTimeout(timer);
    }, [searchQuery]);

    const query: UserListQuery = useMemo(() => {
        const params: UserListQuery = {
            page: page + 1,
            per_page: rowsPerPage,
        };
        if (debouncedSearchQuery.trim()) params.query = debouncedSearchQuery.trim();
        if (waitlistFilter === "waitlisted") params.waitlisted = true;
        if (waitlistFilter === "active") params.waitlisted = false;
        if (isCloud) params.include = "trial";
        return params;
    }, [page, rowsPerPage, debouncedSearchQuery, waitlistFilter, isCloud]);

    const { data, isLoading, error } = useListUsers(query);

    const handleChangePage = (_e: unknown, newPage: number) => setPage(newPage);
    const handleChangeRowsPerPage = (e: React.ChangeEvent<HTMLInputElement>) => {
        setRowsPerPage(parseInt(e.target.value, 10));
        setPage(0);
    };
    const handleClearSearch = () => {
        setSearchQuery("");
        setDebouncedSearchQuery("");
    };

    if (isLoading && !data) {
        return (
            <Paper sx={{ p: 2, display: "flex", justifyContent: "center", alignItems: "center", minHeight: 200 }}>
                <CircularProgress />
            </Paper>
        );
    }

    if (error) {
        return (
            <Paper sx={{ p: 2 }}>
                <Typography color="error">Error loading users: {(error as Error).message}</Typography>
            </Paper>
        );
    }

    const users = data?.users || [];
    const totalCount = data?.totalCount || 0;

    const filterChips: { value: WaitlistFilter; label: string }[] = [
        { value: "all", label: "All" },
        { value: "waitlisted", label: "Waitlisted" },
        { value: "active", label: "Active" },
    ];

    const trialActiveOrStashed = (u: TypesUser | null) =>
        Boolean(u && (u.trial_status === "active" || u.trial_status === "stashed"));

    return (
        <>
            <Box sx={{ mb: 2, display: "flex", justifyContent: "flex-end" }}>
                <Button variant="contained" color="secondary" startIcon={<AddIcon />} onClick={() => setCreateDialogOpen(true)}>
                    Create User
                </Button>
            </Box>
            <Paper sx={{ width: "100%", overflow: "hidden" }}>
                <Box sx={{ p: 2, display: "flex", justifyContent: "space-between", alignItems: "center", gap: 2 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 2, flex: 1, flexWrap: "wrap" }}>
                        <TextField
                            label="Search users"
                            placeholder="Email, username, or full name"
                            size="small"
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            sx={{ minWidth: 320 }}
                            InputProps={{
                                startAdornment: (
                                    <InputAdornment position="start">
                                        <SearchIcon />
                                    </InputAdornment>
                                ),
                                endAdornment: searchQuery && (
                                    <InputAdornment position="end">
                                        <IconButton aria-label="clear search" onClick={handleClearSearch} edge="end" size="small">
                                            <ClearIcon />
                                        </IconButton>
                                    </InputAdornment>
                                ),
                            }}
                        />
                        {isCloud && (
                            <Box sx={{ display: "flex", gap: 1 }}>
                                {filterChips.map((f) => (
                                    <Chip
                                        key={f.value}
                                        label={f.label}
                                        size="small"
                                        color={waitlistFilter === f.value ? "primary" : "default"}
                                        variant={waitlistFilter === f.value ? "filled" : "outlined"}
                                        onClick={() => {
                                            setWaitlistFilter(f.value);
                                            setPage(0);
                                        }}
                                    />
                                ))}
                            </Box>
                        )}
                    </Box>
                    <Typography variant="body2" color="text.secondary">
                        {totalCount} user{totalCount !== 1 ? "s" : ""} total
                    </Typography>
                </Box>

                <TableContainer>
                    <Table stickyHeader aria-label="users table">
                        <TableHead>
                            <TableRow>
                                <TableCell>Username</TableCell>
                                <TableCell>Email</TableCell>
                                <TableCell>Full Name</TableCell>
                                <TableCell>Status</TableCell>
                                {isCloud && <TableCell>Trial</TableCell>}
                                <TableCell>Admin</TableCell>
                                <TableCell>Last Active</TableCell>
                                <TableCell>Created At</TableCell>
                                <TableCell align="right">Actions</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {users.length === 0 ? (
                                <TableRow>
                                    <TableCell colSpan={isCloud ? 9 : 8} align="center">
                                        <Typography variant="body2" color="text.secondary">
                                            {debouncedSearchQuery ? "No users found matching your search" : "No users found"}
                                        </Typography>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                users.map((user: TypesUser) => (
                                    <TableRow
                                        key={user.id}
                                        hover
                                        onClick={() => handleRowClick(user)}
                                        sx={onSelectUser ? { cursor: "pointer" } : undefined}
                                    >
                                        <TableCell>
                                            <Typography variant="body2" sx={{ fontWeight: "medium" }}>
                                                {user.username || "N/A"}
                                            </Typography>
                                        </TableCell>
                                        <TableCell>
                                            <Typography variant="body2">{user.email || "N/A"}</Typography>
                                        </TableCell>
                                        <TableCell>
                                            <Typography variant="body2">{user.full_name || "N/A"}</Typography>
                                        </TableCell>
                                        <TableCell>
                                            {user.waitlisted ? (
                                                <Chip label="Waitlisted" size="small" color="warning" variant="outlined" />
                                            ) : (
                                                <Chip label="Active" size="small" color="success" variant="outlined" />
                                            )}
                                        </TableCell>
                                        {isCloud && (
                                            <TableCell>
                                                <TrialChip user={user} />
                                            </TableCell>
                                        )}
                                        <TableCell>
                                            <Chip
                                                label={user.admin ? "Yes" : "No"}
                                                size="small"
                                                color={user.admin ? "error" : "default"}
                                                variant={user.admin ? "filled" : "outlined"}
                                            />
                                        </TableCell>
                                        <TableCell>
                                            <Tooltip title={user.last_seen_at ? formatFullDate(user.last_seen_at) : "Never signed in"}>
                                                <Typography variant="body2" color={user.last_seen_at ? "text.primary" : "text.secondary"}>
                                                    {formatRelative(user.last_seen_at)}
                                                </Typography>
                                            </Tooltip>
                                        </TableCell>
                                        <TableCell>
                                            <Tooltip title={formatFullDate(user.created_at)}>
                                                <Typography variant="body2">{formatShortDate(user.created_at)}</Typography>
                                            </Tooltip>
                                        </TableCell>
                                        <TableCell align="right" onClick={(e) => e.stopPropagation()}>
                                            <IconButton size="small" onClick={(e) => openMenu(e, user)}>
                                                <MoreVertIcon fontSize="small" />
                                            </IconButton>
                                        </TableCell>
                                    </TableRow>
                                ))
                            )}
                        </TableBody>
                    </Table>
                </TableContainer>

                {totalCount > 0 && (
                    <TablePagination
                        rowsPerPageOptions={[10, 25, 50, 100]}
                        component="div"
                        count={totalCount}
                        rowsPerPage={rowsPerPage}
                        page={page}
                        onPageChange={handleChangePage}
                        onRowsPerPageChange={handleChangeRowsPerPage}
                        labelRowsPerPage="Users per page:"
                        labelDisplayedRows={({ from, to, count }) =>
                            `${from}-${to} of ${count !== -1 ? count : `more than ${to}`}`
                        }
                    />
                )}
            </Paper>

            <Menu anchorEl={menuAnchor} open={Boolean(menuAnchor)} onClose={closeMenu}>
                {isCloud && menuUser?.waitlisted && (
                    <MenuItem onClick={handleApprove}>
                        <ListItemIcon><CheckCircleIcon fontSize="small" sx={{ color: "success.main" }} /></ListItemIcon>
                        <ListItemText>Approve</ListItemText>
                    </MenuItem>
                )}
                {isCloud && !trialActiveOrStashed(menuUser) && (
                    <MenuItem onClick={handleActivateTrial}>
                        <ListItemIcon><CardGiftcardIcon fontSize="small" /></ListItemIcon>
                        <ListItemText>Activate trial</ListItemText>
                    </MenuItem>
                )}
                {isCloud && trialActiveOrStashed(menuUser) && (
                    <MenuItem onClick={handleRevokeTrial}>
                        <ListItemIcon><CancelIcon fontSize="small" sx={{ color: "warning.main" }} /></ListItemIcon>
                        <ListItemText>Revoke trial</ListItemText>
                    </MenuItem>
                )}
                <MenuItem onClick={handleResetPassword}>
                    <ListItemIcon><LockResetIcon fontSize="small" /></ListItemIcon>
                    <ListItemText>Reset password</ListItemText>
                </MenuItem>
                <MenuItem onClick={handleDeleteUser}>
                    <ListItemIcon><DeleteIcon fontSize="small" sx={{ color: "error.main" }} /></ListItemIcon>
                    <ListItemText>Delete user</ListItemText>
                </MenuItem>
            </Menu>

            <CreateUserDialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} />
            <ResetPasswordDialog
                open={resetPasswordDialogOpen}
                onClose={() => {
                    setResetPasswordDialogOpen(false);
                    setSelectedUser(null);
                }}
                user={selectedUser}
            />
            <DeleteUserDialog
                open={deleteDialogOpen}
                onClose={() => {
                    setDeleteDialogOpen(false);
                    setSelectedUser(null);
                }}
                user={selectedUser}
            />
            <ActivateTrialDialog
                open={activateTrialDialogOpen}
                onClose={() => {
                    setActivateTrialDialogOpen(false);
                    setSelectedUser(null);
                }}
                user={selectedUser}
            />
        </>
    );
};

export default UsersTable;
