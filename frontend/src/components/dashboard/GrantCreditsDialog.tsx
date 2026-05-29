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
import { useAdminGrantCredits } from '../../services/dashboardService';
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

    useEffect(() => {
        if (open) {
            setCredits(String(DEFAULT_CREDITS));
            setError('');
        }
    }, [open]);

    const handleSubmit = async () => {
        if (!user?.id) return;
        const creditsNum = parseFloat(credits);
        if (!Number.isFinite(creditsNum) || creditsNum <= 0) {
            setError('Credits must be greater than 0');
            return;
        }
        try {
            const result = await grantCredits.mutateAsync({
                userId: user.id,
                credits: creditsNum,
            });
            const status = (result as any)?.status as string | undefined;
            if (status === 'stashed') {
                snackbar.success(`Credits stashed, applied when ${user.email} creates their first org`);
            } else {
                snackbar.success(`$${creditsNum.toFixed(2)} credited to ${user.email}`);
            }
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
                    Grant credits
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={grantCredits.isPending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {user && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            Granting credits to <strong>{user.email || user.username}</strong>. Top-up lands on the
                            user's oldest owned org regardless of subscription state. If the user has no organization
                            yet, the grant is parked on the user and applied when they create their first org.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    <TextField
                        fullWidth
                        label="Credits (USD)"
                        value={credits}
                        onChange={(e) => setCredits(e.target.value)}
                        margin="normal"
                        disabled={grantCredits.isPending}
                        helperText="Amount credited to the wallet"
                    />
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
                    disabled={grantCredits.isPending}
                    startIcon={grantCredits.isPending ? <CircularProgress size={20} /> : null}
                >
                    {grantCredits.isPending ? 'Granting…' : 'Grant credits'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default GrantCreditsDialog;
