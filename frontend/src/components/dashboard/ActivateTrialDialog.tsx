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
    FormControl,
    InputLabel,
    Select,
    MenuItem,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesUser } from '../../api/api';
import { useAdminActivateTrial, useAdminUserOwnedOrgs } from '../../services/dashboardService';
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
    const [orgId, setOrgId] = useState('');
    // '' = Stripe trial (uses days); 'pro' = paid plan via PlanOverride (no Stripe).
    const [plan, setPlan] = useState('');
    const [error, setError] = useState('');
    const activateTrial = useAdminActivateTrial();
    const snackbar = useSnackbar();
    const { data: ownedOrgs, isLoading: isLoadingOrgs } = useAdminUserOwnedOrgs(user?.id, open);
    const soleOwnedOrgId = ownedOrgs?.length === 1 ? ownedOrgs[0].id : '';
    const effectiveOrgId = orgId || soleOwnedOrgId;

    useEffect(() => {
        if (open) {
            setDays(String(DEFAULT_DAYS));
            setCredits(String(DEFAULT_CREDITS));
            setOrgId('');
            setPlan('');
            setError('');
        }
    }, [open, user?.id]);

    const hasOrgs = (ownedOrgs?.length ?? 0) > 0;

    const handleSubmit = async () => {
        if (!user?.id) return;
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
        if (hasOrgs && !effectiveOrgId) {
            setError('Pick which organisation to activate');
            return;
        }
        try {
            const result = await activateTrial.mutateAsync({
                userId: user.id,
                orgId: hasOrgs ? effectiveOrgId : undefined,
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

                    {hasOrgs && (
                        <FormControl fullWidth margin="normal" disabled={activateTrial.isPending}>
                            <InputLabel id="activate-trial-org-label">Organisation</InputLabel>
                            <Select
                                labelId="activate-trial-org-label"
                                label="Organisation"
                                value={effectiveOrgId}
                                onChange={(e) => setOrgId(e.target.value as string)}
                            >
                                {ownedOrgs!.map((org) => (
                                    <MenuItem key={org.id} value={org.id}>
                                        {org.display_name || org.name} ({org.id})
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

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
                    disabled={activateTrial.isPending || isLoadingOrgs || (hasOrgs && !effectiveOrgId)}
                    startIcon={activateTrial.isPending ? <CircularProgress size={20} /> : null}
                >
                    {activateTrial.isPending ? 'Activating…' : plan === 'pro' ? 'Activate paid plan' : 'Activate trial'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ActivateTrialDialog;
