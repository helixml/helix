import React, { FC, useEffect, useState } from 'react';
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
import { TypesUser } from '../../api/api';
import { useAdminGrantCredits, useAdminUserOwnedOrgs } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface GrantCreditsDialogProps {
    open: boolean;
    onClose: () => void;
    user: TypesUser | null;
}

const DEFAULT_CREDITS = 50;

const GrantCreditsDialog: FC<GrantCreditsDialogProps> = ({ open, onClose, user }) => {
    const [credits, setCredits] = useState(String(DEFAULT_CREDITS));
    const [error, setError] = useState('');
    const grantCredits = useAdminGrantCredits();
    const snackbar = useSnackbar();
    const { data: ownedOrgs, isLoading: isLoadingOrgs } = useAdminUserOwnedOrgs(user?.id, open);

    useEffect(() => {
        if (open) {
            setCredits(String(DEFAULT_CREDITS));
            setError('');
        }
    }, [open]);

    const hasOrgs = (ownedOrgs?.length ?? 0) > 0;
    const willStash = !isLoadingOrgs && !hasOrgs;
    const submitDisabled =
        grantCredits.isPending ||
        isLoadingOrgs ||
        hasOrgs;

    const handleSubmit = async () => {
        if (!user?.id) return;
        if (hasOrgs) return; // org-owning users are handled on the org screen
        const creditsNum = parseFloat(credits);
        if (!Number.isFinite(creditsNum) || creditsNum <= 0) {
            setError('Credits must be greater than 0');
            return;
        }
        try {
            await grantCredits.mutateAsync({
                userId: user.id,
                credits: creditsNum,
                orgId: undefined,
            });
            snackbar.success(`Credits stashed, applied when ${user.email} creates their first org`);
            onClose();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err?.message || 'Failed to grant credits';
            setError(msg);
        }
    };

    const handleClose = () => {
        if (!grantCredits.isPending) onClose();
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
                    Give them credits
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={grantCredits.isPending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {user && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            Granting credits to <strong>{user.email || user.username}</strong>. The grant is
                            stashed and applied to the wallet of their first owned organisation.
                        </Alert>
                    )}

                    {willStash && (
                        <Alert severity="warning" sx={{ mb: 2 }}>
                            This user owns no organisations yet. The grant will be stashed and applied
                            automatically when they create their first owned org.
                        </Alert>
                    )}

                    {hasOrgs && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            This user already owns {ownedOrgs!.length} organisation
                            {ownedOrgs!.length === 1 ? '' : 's'}. Credits for existing organisation wallets are
                            managed on the org screen.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    {!hasOrgs && (
                        <TextField
                            fullWidth
                            label="Credits (USD)"
                            value={credits}
                            onChange={(e) => setCredits(e.target.value)}
                            margin="normal"
                            disabled={grantCredits.isPending}
                            helperText="Amount credited to the wallet"
                            autoFocus
                        />
                    )}
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button onClick={handleClose} disabled={grantCredits.isPending} variant="outlined">
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color="secondary"
                    variant="contained"
                    disabled={submitDisabled}
                    startIcon={grantCredits.isPending ? <CircularProgress size={20} /> : null}
                >
                    {grantCredits.isPending ? 'Granting…' : 'Give credits'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default GrantCreditsDialog;
