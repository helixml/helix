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
    const [orgId, setOrgId] = useState<string>('');
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

    // Pre-select the only owned org when there's exactly one, so the admin
    // doesn't have to interact with a one-item dropdown. Multi-org or
    // no-org cases require an explicit choice (or none, for the stash path).
    useEffect(() => {
        if (!ownedOrgs) return;
        if (ownedOrgs.length === 1) {
            setOrgId(ownedOrgs[0].id);
        } else {
            setOrgId('');
        }
    }, [ownedOrgs]);

    const hasOrgs = (ownedOrgs?.length ?? 0) > 0;
    const willStash = !isLoadingOrgs && !hasOrgs;
    const submitDisabled =
        grantCredits.isPending ||
        isLoadingOrgs ||
        (hasOrgs && !orgId);

    const handleSubmit = async () => {
        if (!user?.id) return;
        const creditsNum = parseFloat(credits);
        if (!Number.isFinite(creditsNum) || creditsNum <= 0) {
            setError('Credits must be greater than 0');
            return;
        }
        if (hasOrgs && !orgId) {
            setError('Pick which organisation receives the grant');
            return;
        }
        try {
            const result = await grantCredits.mutateAsync({
                userId: user.id,
                credits: creditsNum,
                orgId: hasOrgs ? orgId : undefined,
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
                            Granting credits to <strong>{user.email || user.username}</strong>. The grant lands
                            on the wallet of the selected organisation, regardless of subscription state.
                        </Alert>
                    )}

                    {willStash && (
                        <Alert severity="warning" sx={{ mb: 2 }}>
                            This user owns no organisations yet. The grant will be stashed and applied
                            automatically when they create their first owned org.
                        </Alert>
                    )}

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    {hasOrgs && (
                        <FormControl fullWidth margin="normal" disabled={grantCredits.isPending}>
                            <InputLabel id="grant-credits-org-label">Organisation</InputLabel>
                            <Select
                                labelId="grant-credits-org-label"
                                label="Organisation"
                                value={orgId}
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
                    disabled={submitDisabled}
                    startIcon={grantCredits.isPending ? <CircularProgress size={20} /> : null}
                >
                    {grantCredits.isPending ? 'Granting…' : 'Grant credits'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default GrantCreditsDialog;
