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
    FormControl,
    InputLabel,
    Select,
    MenuItem,
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
    const [orgId, setOrgId] = useState('');
    const [error, setError] = useState('');
    const revokeTrial = useAdminRevokeTrial();
    const snackbar = useSnackbar();
    const { data: ownedOrgs, isLoading: isLoadingOrgs } = useAdminUserOwnedOrgs(user?.id, open);

    useEffect(() => {
        if (open) {
            setOrgId('');
            setError('');
        }
    }, [open]);

    // Pre-select the only owned org when there's exactly one, so the admin
    // doesn't have to interact with a one-item dropdown.
    useEffect(() => {
        if (!ownedOrgs) return;
        setOrgId(ownedOrgs.length === 1 ? ownedOrgs[0].id : '');
    }, [ownedOrgs]);

    const hasOrgs = (ownedOrgs?.length ?? 0) > 0;
    const willClearStash = !isLoadingOrgs && !hasOrgs;
    const submitDisabled = revokeTrial.isPending || isLoadingOrgs || (hasOrgs && !orgId);

    const handleSubmit = async () => {
        if (!user?.id) return;
        if (hasOrgs && !orgId) {
            setError('Pick which organisation to revoke the trial on');
            return;
        }
        try {
            await revokeTrial.mutateAsync({
                userId: user.id,
                orgId: hasOrgs ? orgId : undefined,
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

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    {hasOrgs && (
                        <FormControl fullWidth margin="normal" disabled={revokeTrial.isPending}>
                            <InputLabel id="revoke-trial-org-label">Organisation</InputLabel>
                            <Select
                                labelId="revoke-trial-org-label"
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
