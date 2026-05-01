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
    TablePagination,
    InputAdornment,
    IconButton,
    Button,
} from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import ClearIcon from "@mui/icons-material/Clear";
import AddIcon from "@mui/icons-material/Add";
import LockResetIcon from "@mui/icons-material/LockReset";
import DeleteIcon from "@mui/icons-material/Delete";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import { TypesUser } from "../../api/api";
import { useListUsers, useAdminApproveUser, UserListQuery } from "../../services/dashboardService";
import useSnackbar from "../../hooks/useSnackbar";
import CreateUserDialog from "./CreateUserDialog";
import ResetPasswordDialog from "./ResetPasswordDialog";
import DeleteUserDialog from "./DeleteUserDialog";

// Helper function to format date for tooltip
const formatFullDate = (dateString: string | undefined): string => {
    if (!dateString) return "N/A";
    try {
        const date = new Date(dateString);
        const hours = date.getHours().toString().padStart(2, "0");
        const minutes = date.getMinutes().toString().padStart(2, "0");
        const day = date.getDate();
        const monthNames = [
            "Jan",
            "Feb",
            "Mar",
            "Apr",
            "May",
            "Jun",
            "Jul",
            "Aug",
            "Sep",
            "Oct",
            "Nov",
            "Dec",
        ];
        const month = monthNames[date.getMonth()];
        const year = date.getFullYear();

        let daySuffix = "th";
        if (day % 10 === 1 && day !== 11) {
            daySuffix = "st";
        } else if (day % 10 === 2 && day !== 12) {
            daySuffix = "nd";
        } else if (day % 10 === 3 && day !== 13) {
            daySuffix = "rd";
        }

        return `${hours}:${minutes} ${day}${daySuffix} ${month}, ${year}`;
    } catch (error) {
        console.error("Error formatting date:", dateString, error);
        return "Invalid Date";
    }
};

// Helper function to format date shortly for the column
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
        console.error("Error formatting short date:", dateString, error);
        return "Invalid Date";
    }
};

// Relative time for "last active" column - e.g. "5m ago", "3d ago", "never".
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

interface UsersTableProps {
    onSelectUser?: (userId: string) => void;
}

const UsersTable: FC<UsersTableProps> = ({ onSelectUser }) => {
    const [searchQuery, setSearchQuery] = useState("");
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(25);
    const [createDialogOpen, setCreateDialogOpen] = useState(false);
    const [resetPasswordDialogOpen, setResetPasswordDialogOpen] = useState(false);
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [selectedUser, setSelectedUser] = useState<TypesUser | null>(null);
    const approveUser = useAdminApproveUser();
    const snackbar = useSnackbar();

    const handleResetPassword = (user: TypesUser) => {
        setSelectedUser(user);
        setResetPasswordDialogOpen(true);
    };

    const handleCloseResetPasswordDialog = () => {
        setResetPasswordDialogOpen(false);
        setSelectedUser(null);
    };

    const handleApproveUser = async (user: TypesUser) => {
        if (!user.id) return;
        try {
            await approveUser.mutateAsync(user.id);
            snackbar.success(`User ${user.email || user.username} approved`);
        } catch (err: any) {
            snackbar.error(err?.message || "Failed to approve user");
        }
    };

    const handleDeleteUser = (user: TypesUser) => {
        setSelectedUser(user);
        setDeleteDialogOpen(true);
    };

    const handleCloseDeleteDialog = () => {
        setDeleteDialogOpen(false);
        setSelectedUser(null);
    };

    const handleRowClick = (user: TypesUser) => {
        if (user.id && onSelectUser) {
            onSelectUser(user.id);
        }
    };

    // Debounced search query
    const [debouncedSearchQuery, setDebouncedSearchQuery] = useState("");

    // Debounce search query
    React.useEffect(() => {
        const timer = setTimeout(() => {
            setDebouncedSearchQuery(searchQuery);
            setPage(0); // Reset to first page when searching
        }, 300);

        return () => clearTimeout(timer);
    }, [searchQuery]);

    // Build query parameters
    const query: UserListQuery = useMemo(() => {
        const params: UserListQuery = {
            page: page + 1, // API uses 1-based pagination
            per_page: rowsPerPage,
        };

        if (debouncedSearchQuery.trim()) {
            params.query = debouncedSearchQuery.trim();
        }

        return params;
    }, [page, rowsPerPage, debouncedSearchQuery]);

    const { data, isLoading, error } = useListUsers(query);

    const handleChangePage = (event: unknown, newPage: number) => {
        setPage(newPage);
    };

    const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
        setRowsPerPage(parseInt(event.target.value, 10));
        setPage(0);
    };

    const handleClearSearch = () => {
        setSearchQuery("");
        setDebouncedSearchQuery("");
    };

    if (isLoading && !data) {
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
                    Error loading users: {error.message}
                </Typography>
            </Paper>
        );
    }

    const users = data?.users || [];
    const totalCount = data?.totalCount || 0;

    return (
        <>
            <Box sx={{ mb: 2, display: "flex", justifyContent: "flex-end" }}>
                <Button
                    variant="contained"
                    color="secondary"
                    startIcon={<AddIcon />}
                    onClick={() => setCreateDialogOpen(true)}
                >
                    Create User
                </Button>
            </Box>
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
                    <Box sx={{ display: "flex", alignItems: "center", gap: 2, flex: 1 }}>
                        <TextField
                            label="Search users"
                            placeholder="Email, username, or full name"
                            size="small"
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            sx={{ minWidth: 360 }}
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
                                            onClick={handleClearSearch}
                                            edge="end"
                                            size="small"
                                        >
                                            <ClearIcon />
                                        </IconButton>
                                    </InputAdornment>
                                ),
                            }}
                        />
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
                            <TableCell>Admin</TableCell>
                            <TableCell>Last Active</TableCell>
                            <TableCell>Created At</TableCell>
                            <TableCell align="right">Actions</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {users.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={7} align="center">
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
                                        <Typography variant="body2">
                                            {user.email || "N/A"}
                                        </Typography>
                                    </TableCell>
                                    <TableCell>
                                        <Typography variant="body2">
                                            {user.full_name || "N/A"}
                                        </Typography>
                                    </TableCell>
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
                                            <Typography
                                                variant="body2"
                                                color={user.last_seen_at ? "text.primary" : "text.secondary"}
                                            >
                                                {formatRelative(user.last_seen_at)}
                                            </Typography>
                                        </Tooltip>
                                    </TableCell>
                                    <TableCell>
                                        <Tooltip title={formatFullDate(user.created_at)}>
                                            <Typography variant="body2">
                                                {formatShortDate(user.created_at)}
                                            </Typography>
                                        </Tooltip>
                                    </TableCell>
                                    <TableCell
                                        align="right"
                                        onClick={(e) => e.stopPropagation()}
                                    >
                                        {user.waitlisted && (
                                            <Tooltip title="Approve User">
                                                <IconButton
                                                    size="small"
                                                    onClick={() => handleApproveUser(user)}
                                                    disabled={approveUser.isPending}
                                                    sx={{ color: 'success.main' }}
                                                >
                                                    <CheckCircleIcon fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        <Tooltip title="Reset Password">
                                            <IconButton
                                                size="small"
                                                onClick={() => handleResetPassword(user)}
                                                sx={{ color: 'text.secondary' }}
                                            >
                                                <LockResetIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title="Delete User">
                                            <IconButton
                                                size="small"
                                                onClick={() => handleDeleteUser(user)}
                                                sx={{ color: 'error.main' }}
                                            >
                                                <DeleteIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
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
        <CreateUserDialog
            open={createDialogOpen}
            onClose={() => setCreateDialogOpen(false)}
        />
        <ResetPasswordDialog
            open={resetPasswordDialogOpen}
            onClose={handleCloseResetPasswordDialog}
            user={selectedUser}
        />
        <DeleteUserDialog
            open={deleteDialogOpen}
            onClose={handleCloseDeleteDialog}
            user={selectedUser}
        />
        </>
    );
};

export default UsersTable;
