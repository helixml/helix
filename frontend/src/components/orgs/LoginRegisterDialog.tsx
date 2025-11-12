import React, { useState } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Alert,
  Typography,
} from '@mui/material';
import DarkDialog from '../dialog/DarkDialog';
import useApi from '../../hooks/useApi';
import useSnackbar from '../../hooks/useSnackbar';
import { TypesLoginRequest, TypesRegisterRequest } from '../../api/api';

interface LoginRegisterDialogProps {
  open: boolean;
  onClose: () => void;
}

const LoginRegisterDialog: React.FC<LoginRegisterDialogProps> = ({ open, onClose }) => {
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [passwordConfirm, setPasswordConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const api = useApi();
  const snackbar = useSnackbar();
  const apiClient = api.getApiClient();

  const handleClose = () => {
    setMode('login');
    setEmail('');
    setPassword('');
    setPasswordConfirm('');
    setError(null);
    onClose();
  };

  const handleLogin = async () => {
    if (!email || !password) {
      setError('Please fill in all fields');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const loginRequest: TypesLoginRequest = {
        email,
        password,
      };

      await apiClient.v1AuthLoginCreate(loginRequest);
      
      snackbar.success('Login successful');
      handleClose();
      window.location.reload();
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || err?.message || 'Login failed';
      setError(errorMessage);
      snackbar.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleRegister = async () => {
    if (!email || !password || !passwordConfirm) {
      setError('Please fill in all fields');
      return;
    }

    if (password !== passwordConfirm) {
      setError('Passwords do not match');
      return;
    }

    if (password.length < 8) {
      setError('Password must be at least 8 characters long');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const registerRequest: TypesRegisterRequest = {
        email,
        password,
        password_confirm: passwordConfirm,
      };

      await apiClient.v1AuthRegisterCreate(registerRequest);
      
      snackbar.success('Registration successful');
      handleClose();
      window.location.reload();
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || err?.message || 'Registration failed';
      setError(errorMessage);
      snackbar.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (mode === 'login') {
      handleLogin();
    } else {
      handleRegister();
    }
  };

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle sx={{ m: 0, p: 2 }}>
        {mode === 'login' ? 'Login' : 'Register'}
      </DialogTitle>

      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ p: 3 }}>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}

          <TextField
            autoFocus
            margin="dense"
            label="Email"
            type="email"
            fullWidth
            variant="outlined"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            sx={{
              mb: 2,
              '& .MuiOutlinedInput-root': {
                '& fieldset': {
                  borderColor: '#2D3748',
                },
                '&:hover fieldset': {
                  borderColor: '#00E5FF',
                },
                '&.Mui-focused fieldset': {
                  borderColor: '#00E5FF',
                },
              },
              '& .MuiInputLabel-root': {
                color: '#A0AEC0',
              },
              '& .MuiInputLabel-root.Mui-focused': {
                color: '#00E5FF',
              },
              '& .MuiOutlinedInput-input': {
                color: '#F1F1F1',
              },
            }}
          />

          <TextField
            margin="dense"
            label="Password"
            type="password"
            fullWidth
            variant="outlined"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            sx={{
              mb: mode === 'register' ? 2 : 0,
              '& .MuiOutlinedInput-root': {
                '& fieldset': {
                  borderColor: '#2D3748',
                },
                '&:hover fieldset': {
                  borderColor: '#00E5FF',
                },
                '&.Mui-focused fieldset': {
                  borderColor: '#00E5FF',
                },
              },
              '& .MuiInputLabel-root': {
                color: '#A0AEC0',
              },
              '& .MuiInputLabel-root.Mui-focused': {
                color: '#00E5FF',
              },
              '& .MuiOutlinedInput-input': {
                color: '#F1F1F1',
              },
            }}
          />

          {mode === 'register' && (
            <TextField
              margin="dense"
              label="Confirm Password"
              type="password"
              fullWidth
              variant="outlined"
              value={passwordConfirm}
              onChange={(e) => setPasswordConfirm(e.target.value)}
              sx={{
                '& .MuiOutlinedInput-root': {
                  '& fieldset': {
                    borderColor: '#2D3748',
                  },
                  '&:hover fieldset': {
                    borderColor: '#00E5FF',
                  },
                  '&.Mui-focused fieldset': {
                    borderColor: '#00E5FF',
                  },
                },
                '& .MuiInputLabel-root': {
                  color: '#A0AEC0',
                },
                '& .MuiInputLabel-root.Mui-focused': {
                  color: '#00E5FF',
                },
                '& .MuiOutlinedInput-input': {
                  color: '#F1F1F1',
                },
              }}
            />
          )}
        </DialogContent>

        <DialogActions sx={{ p: 2, pt: 0, flexDirection: 'column', gap: 2 }}>
          <Button
            type="submit"
            variant="contained"
            color="secondary"
            disabled={loading}
            fullWidth
            sx={{
              backgroundColor: '#00E5FF',
              color: '#000',
              '&:hover': {
                backgroundColor: '#00B8CC',
              },
            }}
          >
            {loading ? 'Please wait...' : mode === 'login' ? 'Login' : 'Register'}
          </Button>
          <Box sx={{ width: '100%', textAlign: 'center' }}>
            <Typography
              component="span"
              sx={{
                color: '#A0AEC0',
                fontSize: '0.875rem',
              }}
            >
              {mode === 'login' ? "Don't have an account yet? " : 'Already have an account? '}
              <Button
                variant="text"
                size="small"
                onClick={() => {
                  setMode(mode === 'login' ? 'register' : 'login');
                  setError(null);
                  setPassword('');
                  setPasswordConfirm('');
                }}
                sx={{
                  color: '#00E5FF',
                  textTransform: 'none',
                  fontSize: '0.875rem',
                  minWidth: 'auto',
                  padding: '0 4px',
                  marginBottom: '2px',
                  '&:hover': {
                    backgroundColor: 'transparent',
                    textDecoration: 'underline',
                  },
                }}
              >
                {mode === 'login' ? 'Register here' : 'Login here'}
              </Button>
            </Typography>
          </Box>
        </DialogActions>
      </form>
    </DarkDialog>
  );
};

export default LoginRegisterDialog;

