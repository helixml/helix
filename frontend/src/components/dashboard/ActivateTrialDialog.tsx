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
import { useAdminActivateTrial } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface ActivateTrialDialogProps {
    open: boolean;
    onClose: () => void;
    user: TypesUser | null;
}

const DEFAULT_DAYS = 90;
const DEFAULT_CREDITS = 100;

const ActivateTrialDialog: FC<ActivateTrialDialogProps> = ({ open, onClose, user }) => {
    const [days, setDays] = useState(String(DEFAULT_DAYS));
    const [credits, setCredits] = useState(String(DEFAULT_CREDITS));
    const [error, setError] = useState('');
    const activateTrial = useAdminActivateTrial();
    const snackbar = useSnackbar();

    useEffect(() => {
        if (open) {
            setDays(String(DEFAULT_DAYS));
            setCredits(String(DEFAULT_CREDITS));
            setError('');
        }
    }, [open]);

    const handleSubmit = async () => {
        if (!user?.id) return;
        const daysNum = parseInt(days, 10);
        const creditsNum = parseFloat(credits);
        if (!Number.isFinite(daysNum) || daysNum <= 0) {
            setError('Days must be a positive number');
            return;
        }
        if (!Number.isFinite(creditsNum) || creditsNum < 0) {
            setError('Credits must be zero or positive');
            return;
        }
        try {
            const result = await activateTrial.mutateAsync({
                userId: user.id,
                days: daysNum,
                credits: creditsNum,
            });
            const status = (result as any)?.status as string | undefined;
            if (status === 'stashed') {
                snackbar.success(`Trial intent stashed — applied when ${user.email} creates their first org`);
            } else {
                snackbar.success(`${daysNum}-day trial activated for ${user.email}`);
            }
            onClose();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err?.message || 'Failed to activate trial';
            setError(msg);
        }
    };

    const handleClose = () => {
        if (!activateTrial.isPending) onClose();
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
                    Activate Trial
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={activateTrial.isPending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {user && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            Granting trial to <strong>{user.email || user.username}</strong>. If the user has no
                            organization yet, the trial is parked on the user and applied when they create their
                            first org. Otherwise it is applied to their oldest owned org immediately.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    <TextField
                        fullWidth
                        label="Days"
                        value={days}
                        onChange={(e) => setDays(e.target.value)}
                        margin="normal"
                        disabled={activateTrial.isPending}
                        helperText="Length of the free trial in days"
                    />

                    <TextField
                        fullWidth
                        label="Credits (USD)"
                        value={credits}
                        onChange={(e) => setCredits(e.target.value)}
                        margin="normal"
                        disabled={activateTrial.isPending}
                        helperText="Credit balance added to the wallet at trial start"
                    />
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button onClick={handleClose} disabled={activateTrial.isPending} variant="outlined">
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color="secondary"
                    variant="contained"
                    disabled={activateTrial.isPending}
                    startIcon={activateTrial.isPending ? <CircularProgress size={20} /> : null}
                >
                    {activateTrial.isPending ? 'Activating…' : 'Activate Trial'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ActivateTrialDialog;
