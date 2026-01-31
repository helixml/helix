import React, { FC, useState, useEffect } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    TextField,
    Box,
    Alert,
    CircularProgress,
    IconButton,
    Typography,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import { TypesUser } from '../../api/api';
import { useAdminDeleteUser } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface DeleteUserDialogProps {
    open: boolean;
    onClose: () => void;
    user: TypesUser | null;
}

const DeleteUserDialog: FC<DeleteUserDialogProps> = ({ open, onClose, user }) => {
    const [confirmText, setConfirmText] = useState('');
    const [error, setError] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);
    const deleteUser = useAdminDeleteUser();
    const snackbar = useSnackbar();

    const expectedConfirmation = 'delete';

    useEffect(() => {
        if (open) {
            setConfirmText('');
            setError('');
        }
    }, [open]);

    const handleSubmit = async () => {
        if (!user?.id) return;

        if (confirmText.toLowerCase() !== expectedConfirmation) {
            setError(`Please type "${expectedConfirmation}" to confirm deletion`);
            return;
        }

        setError('');
        setIsSubmitting(true);

        try {
            await deleteUser.mutateAsync(user.id);
            snackbar.success(`User ${user.email || user.username} has been deleted`);
            onClose();
        } catch (err) {
            console.error('Failed to delete user:', err);
            const errorMessage = err instanceof Error ? err.message : 'Failed to delete user';
            setError(errorMessage);
            snackbar.error(errorMessage);
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleClose = () => {
        if (!isSubmitting) {
            setError('');
            onClose();
        }
    };

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
            <DialogTitle
                sx={{
                    m: 0,
                    p: 2,
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    backgroundColor: 'error.dark',
                }}
            >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <WarningAmberIcon />
                    <Typography variant="h6" component="div">
                        Delete User
                    </Typography>
                </Box>
                <IconButton
                    aria-label="close"
                    onClick={handleClose}
                    disabled={isSubmitting}
                    sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
                >
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    <Alert severity="error" sx={{ mb: 2 }}>
                        <Typography variant="body2" sx={{ fontWeight: 'bold', mb: 1 }}>
                            This action is permanent and cannot be undone!
                        </Typography>
                        <Typography variant="body2">
                            Deleting this user will also remove all their associated data including API keys and user settings.
                        </Typography>
                    </Alert>

                    {user && (
                        <Box sx={{ mb: 2, p: 2, backgroundColor: 'action.hover', borderRadius: 1 }}>
                            <Typography variant="body2" color="text.secondary">
                                User to be deleted:
                            </Typography>
                            <Typography variant="body1" sx={{ fontWeight: 'medium' }}>
                                {user.email || user.username}
                            </Typography>
                            {user.full_name && (
                                <Typography variant="body2" color="text.secondary">
                                    {user.full_name}
                                </Typography>
                            )}
                            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                                ID: {user.id}
                            </Typography>
                        </Box>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    <Typography variant="body2" sx={{ mb: 1 }}>
                        To confirm deletion, type <strong>{expectedConfirmation}</strong> below:
                    </Typography>
                    <TextField
                        fullWidth
                        placeholder={expectedConfirmation}
                        value={confirmText}
                        onChange={(e) => setConfirmText(e.target.value)}
                        disabled={isSubmitting}
                        autoComplete="off"
                        size="small"
                    />
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button
                    onClick={handleClose}
                    disabled={isSubmitting}
                    variant="outlined"
                >
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color="error"
                    variant="contained"
                    disabled={isSubmitting || confirmText.toLowerCase() !== expectedConfirmation}
                    startIcon={isSubmitting ? <CircularProgress size={20} /> : null}
                >
                    {isSubmitting ? 'Deleting...' : 'Delete User'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default DeleteUserDialog;
