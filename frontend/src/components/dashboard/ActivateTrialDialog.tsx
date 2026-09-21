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
    MenuItem,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesUser } from '../../api/api';
import { useAdminActivateTrial, useAdminRevokeTrial, useAdminUserOwnedOrgs } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface ActivateTrialDialogProps {
    open: boolean;
    onClose: () => void;
    user: TypesUser | null;
}

const DEFAULT_DAYS = 90;
// Intentionally 0: the form value is sent verbatim, no silent defaulting on
// either side. Stripe still credits the trial-period product allotment on
// subscription start via the invoice.paid webhook -- that's separate from
// the admin's choice here.
const DEFAULT_CREDITS = 0;

const ActivateTrialDialog: FC<ActivateTrialDialogProps> = ({ open, onClose, user }) => {
    const [days, setDays] = useState(String(DEFAULT_DAYS));
    const [credits, setCredits] = useState(String(DEFAULT_CREDITS));
    // '' = Stripe trial (uses days); 'pro' = paid plan via PlanOverride (no Stripe).
    const [plan, setPlan] = useState('');
    const [error, setError] = useState('');
    const activateTrial = useAdminActivateTrial();
    const revokeTrial = useAdminRevokeTrial();
    const snackbar = useSnackbar();
    const { data: ownedOrgs, isLoading: isLoadingOrgs } = useAdminUserOwnedOrgs(user?.id, open);

    // A stashed intent (granted before the user created an org) can be a
    // trial (days/credits), a paid-plan override, or stashed admin credits —
    // any of them prefills the form and can be undone via Clear.
    const hasStash = Boolean(
        user?.trial_days_on_first_org ||
        user?.trial_credits_on_first_org ||
        user?.plan_on_first_org ||
        user?.pending_admin_credits_on_first_org
    );
    const stashParts = [
        user?.trial_days_on_first_org ? `${user.trial_days_on_first_org}d` : '',
        user?.trial_credits_on_first_org ? `$${user.trial_credits_on_first_org}` : '',
        user?.plan_on_first_org ? `${user.plan_on_first_org} plan` : '',
        user?.pending_admin_credits_on_first_org ? `$${user.pending_admin_credits_on_first_org} credits` : '',
    ].filter(Boolean);
    const stashSummary = stashParts.length ? ` (${stashParts.join(', ')})` : '';

    useEffect(() => {
        if (open) {
            setDays(String(user?.trial_days_on_first_org || DEFAULT_DAYS));
            setCredits(String(user?.trial_credits_on_first_org ?? DEFAULT_CREDITS));
            setPlan(user?.plan_on_first_org === 'pro' ? 'pro' : '');
            setError('');
        }
    }, [open, user?.id, user?.trial_days_on_first_org, user?.trial_credits_on_first_org, user?.plan_on_first_org]);

    const hasOrgs = (ownedOrgs?.length ?? 0) > 0;

    const handleSubmit = async () => {
        if (!user?.id) return;
        if (hasOrgs) return; // org-owning users are handled on the org screen
        const isPaid = plan === 'pro';
        const daysNum = parseInt(days, 10);
        const creditsNum = parseFloat(credits);
        if (!isPaid && (!Number.isFinite(daysNum) || daysNum <= 0)) {
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
                orgId: undefined,
                days: isPaid ? 0 : daysNum,
                credits: creditsNum,
                plan,
            });
            const status = (result as any)?.status as string | undefined;
            if (status === 'stashed') {
                snackbar.success(`${isPaid ? 'Paid plan' : 'Trial'} intent stashed — applied when ${user.email} creates their first org`);
            } else {
                snackbar.success(isPaid ? `Paid (Pro) plan activated for ${user.email}` : `${daysNum}-day trial activated for ${user.email}`);
            }
            onClose();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err?.message || 'Failed to activate';
            setError(msg);
        }
    };

    const handleClearStash = async () => {
        if (!user?.id) return;
        try {
            await revokeTrial.mutateAsync({ userId: user.id });
            snackbar.success(`Stashed grant cleared for ${user.email || user.username}`);
            onClose();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err?.message || 'Failed to clear stashed grant';
            setError(msg);
        }
    };

    const handleClose = () => {
        if (!activateTrial.isPending && !revokeTrial.isPending) onClose();
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
                    Activate trial or plan
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={activateTrial.isPending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {user && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            This approves <strong>{user.email || user.username}</strong> and applies the selected
                            trial or plan. If they own no organisation yet, it is applied to their first one.
                        </Alert>
                    )}

                    {hasStash && (
                        <Alert severity="warning" sx={{ mb: 2 }}>
                            This user already has a stashed grant{stashSummary}. Activating replaces it; the
                            fields are prefilled with the current values.
                        </Alert>
                    )}

                    {hasOrgs && (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            This user already owns {ownedOrgs!.length} organisation
                            {ownedOrgs!.length === 1 ? '' : 's'}. Trials and plans for existing organisations
                            are managed on the org screen.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    {!hasOrgs && (
                        <>
                            <TextField
                                select
                                fullWidth
                                label="Plan"
                                value={plan}
                                onChange={(e) => setPlan(e.target.value)}
                                SelectProps={{ displayEmpty: true }}
                                margin="normal"
                                disabled={activateTrial.isPending}
                                helperText={
                                    plan === 'pro'
                                        ? 'Paid Pro — no Stripe, no card, indefinite (for customers who paid out-of-band)'
                                        : 'Stripe trial for the chosen number of days'
                                }
                            >
                                <MenuItem value="">Trial (Stripe)</MenuItem>
                                <MenuItem value="pro">Paid — Pro (no Stripe)</MenuItem>
                            </TextField>

                            <TextField
                                fullWidth
                                label="Days"
                                value={days}
                                onChange={(e) => setDays(e.target.value)}
                                margin="normal"
                                disabled={activateTrial.isPending || plan === 'pro'}
                                helperText={plan === 'pro' ? 'Not used for a paid plan' : 'Length of the free trial in days'}
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
                        </>
                    )}
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                {hasStash && (
                    <Button
                        onClick={handleClearStash}
                        color="warning"
                        variant="outlined"
                        disabled={activateTrial.isPending || revokeTrial.isPending}
                        startIcon={revokeTrial.isPending ? <CircularProgress size={20} /> : null}
                        sx={{ mr: 'auto' }}
                    >
                        {revokeTrial.isPending ? 'Clearing…' : 'Clear stashed grant'}
                    </Button>
                )}
                <Button onClick={handleClose} disabled={activateTrial.isPending || revokeTrial.isPending} variant="outlined">
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color="secondary"
                    variant="contained"
                    disabled={activateTrial.isPending || revokeTrial.isPending || isLoadingOrgs || hasOrgs}
                    startIcon={activateTrial.isPending ? <CircularProgress size={20} /> : null}
                >
                    {activateTrial.isPending ? 'Activating…' : plan === 'pro' ? 'Activate paid plan' : 'Activate trial'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ActivateTrialDialog;
