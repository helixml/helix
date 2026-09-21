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
    Radio,
    RadioGroup,
    FormControlLabel,
    Divider,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesOrgDetails } from '../../api/api';
import { useAdminSetOrgPlan } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface AdminOrgPlanDialogProps {
    open: boolean;
    onClose: () => void;
    org: TypesOrgDetails | null;
}

const PLAN_OPTIONS = [
    {
        value: 'pro',
        label: 'Force Pro',
        description: "Paid tier regardless of Stripe — for customers who paid out-of-band. Never reverted by a Stripe webhook.",
    },
    {
        value: 'free',
        label: 'Force Free',
        description: 'Free tier regardless of Stripe. Use to cap usage without cancelling a subscription.',
    },
];

// Not a plan choice — the recovery action that undoes a forced override.
// Only offered when there is actually an override to remove.
const REMOVE_OVERRIDE_OPTION = {
    label: 'Remove override (back to Stripe)',
    description: "Delete the forced plan — the org returns to whatever its Stripe subscription says.",
};

const AdminOrgPlanDialog: FC<AdminOrgPlanDialogProps> = ({ open, onClose, org }) => {
    const [plan, setPlan] = useState('');
    const [error, setError] = useState('');
    const setOrgPlan = useAdminSetOrgPlan();
    const snackbar = useSnackbar();

    const orgId = org?.organization?.id || '';
    const orgName = org?.organization?.display_name || org?.organization?.name || '';
    const currentOverride = org?.wallet?.plan_override || '';

    useEffect(() => {
        if (open) {
            setPlan(currentOverride);
            setError('');
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open]);

    const handleSubmit = async () => {
        if (!orgId) return;
        try {
            await setOrgPlan.mutateAsync({ orgId, plan });
            snackbar.success(
                plan
                    ? `${orgName} forced to ${plan} (Stripe ignored)`
                    : `${orgName} now follows Stripe (override cleared)`
            );
            onClose();
        } catch (err: any) {
            setError(err?.response?.data?.error || err?.message || 'Failed to set plan');
        }
    };

    const handleClose = () => {
        if (!setOrgPlan.isPending) onClose();
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
                    Set plan
                </Typography>
                <IconButton aria-label="close" onClick={handleClose} disabled={setOrgPlan.isPending}>
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 1 }}>
                    <Alert severity="info" sx={{ mb: 2 }}>
                        {currentOverride
                            ? <>Current: <strong>forced to {currentOverride}</strong> (Stripe ignored).</>
                            : <>Current: <strong>following Stripe</strong> (no override set).</>}
                    </Alert>

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    <RadioGroup value={plan} onChange={(e) => setPlan(e.target.value)}>
                        {PLAN_OPTIONS.map((option) => (
                            <Box
                                key={option.value}
                                sx={{
                                    border: '1px solid',
                                    borderColor: plan === option.value ? 'action.selected' : 'divider',
                                    borderRadius: 1,
                                    px: 1.5,
                                    py: 0.5,
                                    mb: 1,
                                }}
                            >
                                <FormControlLabel
                                    value={option.value}
                                    control={<Radio size="small" />}
                                    label={
                                        <Box sx={{ py: 0.5 }}>
                                            <Typography variant="body2">{option.label}</Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                                {option.description}
                                            </Typography>
                                        </Box>
                                    }
                                    disabled={setOrgPlan.isPending}
                                />
                            </Box>
                        ))}
                        {currentOverride && (
                            <>
                                <Divider sx={{ my: 1.5 }} />
                                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                                    Undo the forced plan:
                                </Typography>
                                <Box
                                    sx={{
                                        border: '1px solid',
                                        borderColor: plan === '' ? 'action.selected' : 'divider',
                                        borderRadius: 1,
                                        px: 1.5,
                                        py: 0.5,
                                    }}
                                >
                                    <FormControlLabel
                                        value=""
                                        control={<Radio size="small" />}
                                        label={
                                            <Box sx={{ py: 0.5 }}>
                                                <Typography variant="body2">{REMOVE_OVERRIDE_OPTION.label}</Typography>
                                                <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                                    {REMOVE_OVERRIDE_OPTION.description}
                                                </Typography>
                                            </Box>
                                        }
                                        disabled={setOrgPlan.isPending}
                                    />
                                </Box>
                            </>
                        )}
                    </RadioGroup>
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button onClick={handleClose} disabled={setOrgPlan.isPending} variant="outlined">
                    Cancel
                </Button>
                <Button
                    onClick={handleSubmit}
                    color="secondary"
                    variant="contained"
                    disabled={setOrgPlan.isPending || plan === currentOverride}
                    startIcon={setOrgPlan.isPending ? <CircularProgress size={20} /> : null}
                >
                    {setOrgPlan.isPending ? 'Applying…' : 'Apply plan'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default AdminOrgPlanDialog;
