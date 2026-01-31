import React, { FC, useState, useEffect } from 'react';
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
    FormControlLabel,
    Checkbox,
    IconButton,
    Typography,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { useCreateUser } from '../../services/dashboardService';
import useSnackbar from '../../hooks/useSnackbar';

interface CreateUserDialogProps {
    open: boolean;
    onClose: () => void;
}

const CreateUserDialog: FC<CreateUserDialogProps> = ({ open, onClose }) => {
    const [formData, setFormData] = useState({
        email: '',
        password: '',
        full_name: '',
        admin: false,
    });
    const [error, setError] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);
    const createUser = useCreateUser();
    const snackbar = useSnackbar();

    useEffect(() => {
        if (open) {
            setFormData({
                email: '',
                password: '',
                full_name: '',
                admin: false,
            });
            setError('');
        }
    }, [open]);

    const validateForm = () => {
        if (!formData.email.trim()) {
            setError('Email is required');
            return false;
        }
        if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email.trim())) {
            setError('Please enter a valid email address');
            return false;
        }
        if (!formData.password.trim()) {
            setError('Password is required');
            return false;
        }
        if (formData.password.length < 8) {
            setError('Password must be at least 8 characters long');
            return false;
        }
        return true;
    };

    const handleSubmit = async () => {
        if (!validateForm()) return;

        setError('');
        setIsSubmitting(true);

        try {
            await createUser.mutateAsync({
                email: formData.email.trim(),
                password: formData.password,
                full_name: formData.full_name.trim() || undefined,
                admin: formData.admin,
            });

            snackbar.success('User created successfully');
            onClose();
        } catch (err) {
            console.error('Failed to create user:', err);
            const errorMessage = err instanceof Error ? err.message : 'Failed to create user';
            setError(errorMessage);
            snackbar.error(errorMessage);
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleClose = () => {
        setError('');
        onClose();
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
                    Create New User
                </Typography>
                <IconButton
                    aria-label="close"
                    onClick={handleClose}
                    disabled={isSubmitting}
                    sx={{ color: 'rgba(255, 255, 255, 0.7)' }}
                >
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent>
                <Box sx={{ mt: 2 }}>
                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }}>
                            {error}
                        </Alert>
                    )}

                    <TextField
                        fullWidth
                        label="Email"
                        type="email"
                        value={formData.email}
                        onChange={(e) =>
                            setFormData({ ...formData, email: e.target.value })
                        }
                        margin="normal"
                        required
                        disabled={isSubmitting}
                        autoComplete="off"
                    />

                    <TextField
                        fullWidth
                        label="Password"
                        type="password"
                        value={formData.password}
                        onChange={(e) =>
                            setFormData({ ...formData, password: e.target.value })
                        }
                        margin="normal"
                        required
                        disabled={isSubmitting}
                        helperText="Must be at least 8 characters long"
                        autoComplete="new-password"
                    />

                    <TextField
                        fullWidth
                        label="Full Name"
                        value={formData.full_name}
                        onChange={(e) =>
                            setFormData({ ...formData, full_name: e.target.value })
                        }
                        margin="normal"
                        disabled={isSubmitting}
                        autoComplete="off"
                    />

                    <FormControlLabel
                        control={
                            <Checkbox
                                checked={formData.admin}
                                onChange={(e) =>
                                    setFormData({ ...formData, admin: e.target.checked })
                                }
                                disabled={isSubmitting}
                            />
                        }
                        label="Admin"
                        sx={{ mt: 2 }}
                    />
                </Box>
            </DialogContent>
            <DialogActions sx={{ p: 2 }}>
                <Button
                    onClick={handleSubmit}
                    color="secondary"
                    variant="contained"
                    disabled={isSubmitting}
                    startIcon={isSubmitting ? <CircularProgress size={20} /> : null}
                >
                    {isSubmitting ? 'Creating...' : 'Create'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default CreateUserDialog;

