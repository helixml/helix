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
    TextField,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesOrgDetails } from '../../api/api';
import { useAdminActivateTrial, useAdminGrantCredits, useAdminRevokeTrial } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

export type OrgBillingAction = 'activate' | 'revoke' | 'credits';

interface AdminOrgBillingDialogProps {
    open: boolean;
    onClose: () => void;
    action: OrgBillingAction;
    org: TypesOrgDetails | null;
}

const DEFAULT_TRIAL_DAYS = 90;

const actionMeta: Record<OrgBillingAction, { title: string; submit: string; color: 'secondary' | 'warning' }> = {
    activate: { title: 'Activate trial', submit: 'Activate trial', color: 'secondary' },
    revoke: { title: 'Revoke trial', submit: 'Revoke trial', color: 'warning' },
    credits: { title: 'Grant credits', submit: 'Grant credits', color: 'secondary' },
};

const AdminOrgBillingDialog: FC<AdminOrgBillingDialogProps> = ({ open, onClose, action, org }) => {
    const [days, setDays] = useState(String(DEFAULT_TRIAL_DAYS));
    const [credits, setCredits] = useState('0');
    const [error, setError] = useState('');
    const activateTrial = useAdminActivateTrial();
    const revokeTrial = useAdminRevokeTrial();
    const grantCredits = useAdminGrantCredits();
    const snackbar = useSnackbar();

    useEffect(() => {
        if (open) {
            setDays(String(DEFAULT_TRIAL_DAYS));
            setCredits(action === 'credits' ? '' : '0');
            setError('');
        }
    }, [open, action]);

    const orgId = org?.organization?.id || '';
    const orgName = org?.organization?.display_name || org?.organization?.name || '';
    const ownerId = org?.organization?.owner || '';

    const pending =
        action === 'activate' ? activateTrial.isPending : action === 'revoke' ? revokeTrial.isPending : grantCredits.isPending;

    const handleSubmit = async () => {
        if (!ownerId || !orgId) {
            setError('This organisation has no owner set; cannot apply billing actions');
            return;
        }
        try {
            if (action === 'activate') {
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
                await activateTrial.mutateAsync({ userId: ownerId, orgId, days: daysNum, credits: creditsNum });
                snackbar.success(`${daysNum}-day trial activated on ${orgName}`);
            } else if (action === 'revoke') {
                await revokeTrial.mutateAsync({ userId: ownerId, orgId });
                snackbar.success(`Trial revoked on ${orgName}`);
            } else {
                const creditsNum = parseFloat(credits);
                if (!Number.isFinite(creditsNum) || creditsNum <= 0) {
                    setError('Credits must be greater than 0');
                    return;
                }
                await grantCredits.mutateAsync({ userId: ownerId, orgId, credits: creditsNum });
                snackbar.success(`$${creditsNum.toFixed(2)} credited to ${orgName}`);
            }
            onClose();
        } catch (err: any) {
            setError(err?.response?.data?.error || err?.message || 'Action failed');
        }
    };

    const handleClose = () => {
        if (!pending) onClose();
    };

    const meta = actionMeta[action];

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
                    {meta.title}
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={pending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {action === 'activate' && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            Creates a Stripe trial subscription on <strong>{orgName}</strong>'s wallet for the
                            given number of days.
                        </Alert>
                    )}
                    {action === 'revoke' && (
                        <Alert severity="warning" sx={{ mb: 2 }}>
                            Cancels the trialing Stripe subscription on <strong>{orgName}</strong>'s wallet
                            immediately. The wallet returns to free usage. Paid subscriptions are never touched.
                        </Alert>
                    )}
                    {action === 'credits' && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            Adds credits to <strong>{orgName}</strong>'s wallet, regardless of subscription state.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    {action === 'activate' && (
                        <>
                            <TextField
                                fullWidth
                                label="Days"
                                value={days}
                                onChange={(e) => setDays(e.target.value)}
                                margin="normal"
                                disabled={pending}
                                helperText="Length of the free trial in days"
                            />
                            <TextField
                                fullWidth
                                label="Credits (USD)"
                                value={credits}
                                onChange={(e) => setCredits(e.target.value)}
                                margin="normal"
                                disabled={pending}
                                helperText="Credit balance added to the wallet at trial start"
                            />
                        </>
                    )}

                    {action === 'credits' && (
                        <TextField
                            fullWidth
                            label="Credits (USD)"
                            value={credits}
                            onChange={(e) => setCredits(e.target.value)}
                            margin="normal"
                            disabled={pending}
                            helperText="Amount credited to the wallet"
                            autoFocus
                        />
                    )}
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button onClick={handleClose} disabled={pending} variant="outlined">
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color={meta.color}
                    variant="contained"
                    disabled={pending}
                    startIcon={pending ? <CircularProgress size={20} /> : null}
                >
                    {pending ? 'Working…' : meta.submit}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default AdminOrgBillingDialog;
