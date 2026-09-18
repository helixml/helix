import React, { FC, useEffect, useState } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    Box,
    Alert,
    CircularProgress,
    IconButton,
    Typography,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesUser } from '../../api/api';
import { useAdminRevokeTrial, useAdminUserOwnedOrgs } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface RevokeTrialDialogProps {
    open: boolean;
    onClose: () => void;
    user: TypesUser | null;
}

const RevokeTrialDialog: FC<RevokeTrialDialogProps> = ({ open, onClose, user }) => {
    const [error, setError] = useState('');
    const revokeTrial = useAdminRevokeTrial();
    const snackbar = useSnackbar();
    const { data: ownedOrgs, isLoading: isLoadingOrgs } = useAdminUserOwnedOrgs(user?.id, open);

    useEffect(() => {
        if (open) {
            setError('');
        }
    }, [open]);

    const hasOrgs = (ownedOrgs?.length ?? 0) > 0;
    const willClearStash = !isLoadingOrgs && !hasOrgs;
    const submitDisabled = revokeTrial.isPending || isLoadingOrgs || hasOrgs;

    const handleSubmit = async () => {
        if (!user?.id) return;
        if (hasOrgs) return; // org-owning users are handled on the org screen
        try {
            await revokeTrial.mutateAsync({
                userId: user.id,
                orgId: undefined,
            });
            snackbar.success(`Trial revoked for ${user.email || user.username}`);
            onClose();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err?.message || 'Failed to revoke trial';
            setError(msg);
        }
    };

    const handleClose = () => {
        if (!revokeTrial.isPending) onClose();
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
                }}
            >
                <Typography variant="h6" component="div">
                    Revoke trial
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={revokeTrial.isPending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {user && hasOrgs && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            Revokes the trial on the selected organisation for{' '}
                            <strong>{user.email || user.username}</strong>. The Stripe trial subscription
                            is cancelled immediately and the wallet returns to free usage.
                        </Alert>
                    )}

                    {willClearStash && (
                        <Alert severity="warning" sx={{ mb: 2 }}>
                            This user owns no organisations yet. Any stashed trial intent will be cleared;
                            it never reached a wallet.
                        </Alert>
                    )}

                    {hasOrgs && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            This user already owns {ownedOrgs!.length} organisation
                            {ownedOrgs!.length === 1 ? '' : 's'}. Trial revocation for existing organisations is
                            managed on the org screen.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button onClick={handleClose} disabled={revokeTrial.isPending} variant="outlined">
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color="warning"
                    variant="contained"
                    disabled={submitDisabled}
                    startIcon={revokeTrial.isPending ? <CircularProgress size={20} /> : null}
                >
                    {revokeTrial.isPending ? 'Revoking…' : 'Revoke trial'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default RevokeTrialDialog;
