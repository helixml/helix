import React, { useState, useRef } from 'react';
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
import useRouter from '../../hooks/useRouter';
import { TypesLoginRequest, TypesRegisterRequest } from '../../api/api';
import { useGetConfig } from '../../services/userService';

interface LoginRegisterDialogProps {
  open: boolean;
  onClose: () => void;
}

const LoginRegisterDialog: React.FC<LoginRegisterDialogProps> = ({ open, onClose }) => {
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [email, setEmail] = useState('');
  const [fullName, setFullName] = useState('');
  const [password, setPassword] = useState('');
  const [passwordConfirm, setPasswordConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // Refs to read actual DOM values on submit (iOS autofill doesn't trigger onChange)
  const formRef = useRef<HTMLFormElement>(null);

  const { data: config, isLoading: isLoadingConfig } = useGetConfig()

  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();
  const apiClient = api.getApiClient();

  const isRegistrationDisabled = mode === 'register' && config?.registration_enabled === false;

  const handleClose = () => {
    setMode('login');
    setEmail('');
    setFullName('');
    setPassword('');
    setPasswordConfirm('');
    setError(null);
    onClose();
  };

  const handleLogin = async () => {
    // Read from DOM to handle iOS autofill (which doesn't trigger onChange)
    const form = formRef.current;
    const emailValue = (form?.querySelector('input[name="username"]') as HTMLInputElement)?.value || email;
    const passwordValue = (form?.querySelector('input[name="password"]') as HTMLInputElement)?.value || password;

    if (!emailValue || !passwordValue) {
      setError('Please fill in all fields');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const loginRequest: TypesLoginRequest = {
        email: emailValue,
        password: passwordValue,
      };

      await apiClient.v1AuthLoginCreate(loginRequest);
      
      snackbar.success('Login successful');
      handleClose();
      window.location.reload();
    } catch (err: any) {
      let errorMessage = 'Login failed';
      if (err?.response?.data) {
        if (typeof err.response.data === 'string') {
          errorMessage = err.response.data;
        } else if (err.response.data.message) {
          errorMessage = err.response.data.message;
        } else if (err.response.data.error) {
          errorMessage = err.response.data.error;
        } else {
          errorMessage = JSON.stringify(err.response.data);
        }
      } else if (err?.message) {
        errorMessage = err.message;
      }
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleRegister = async () => {
    // Read from DOM to handle iOS autofill (which doesn't trigger onChange)
    const form = formRef.current;
    const emailValue = (form?.querySelector('input[name="username"]') as HTMLInputElement)?.value || email;
    const fullNameValue = (form?.querySelector('input[name="name"]') as HTMLInputElement)?.value || fullName;
    const passwordValue = (form?.querySelector('input[name="password"]') as HTMLInputElement)?.value || password;
    const passwordConfirmValue = (form?.querySelector('input[name="password-confirm"]') as HTMLInputElement)?.value || passwordConfirm;

    if (!emailValue || !fullNameValue || !passwordValue || !passwordConfirmValue) {
      setError('Please fill in all fields');
      return;
    }

    if (passwordValue !== passwordConfirmValue) {
      setError('Passwords do not match');
      return;
    }

    if (passwordValue.length < 8) {
      setError('Password must be at least 8 characters long');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const registerRequest: TypesRegisterRequest = {
        email: emailValue,
        full_name: fullNameValue,
        password: passwordValue,
        password_confirm: passwordConfirmValue,
      };

      await apiClient.v1AuthRegisterCreate(registerRequest);
      
      snackbar.success('Registration successful');
      handleClose();
      window.location.reload();
    } catch (err: any) {
      let errorMessage = 'Registration failed';
      if (err?.response?.data) {
        if (typeof err.response.data === 'string') {
          errorMessage = err.response.data;
        } else if (err.response.data.message) {
          errorMessage = err.response.data.message;
        } else if (err.response.data.error) {
          errorMessage = err.response.data.error;
        } else {
          errorMessage = JSON.stringify(err.response.data);
        }
      } else if (err?.message) {
        errorMessage = err.message;
      }
      setError(errorMessage);
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
      disablePortal
      keepMounted
      disableEnforceFocus
      TransitionProps={{ tabIndex: 'none' } as any}
    >
      <DialogTitle sx={{ m: 0, p: 2 }}>
        {mode === 'login' ? 'Login' : 'Register'}
      </DialogTitle>

      <form ref={formRef} onSubmit={handleSubmit}>
        <DialogContent sx={{ p: 3 }}>
          {isRegistrationDisabled && (
            <Alert severity="info" sx={{ mb: 2 }}>
              New account registrations are disabled. Please contact your server administrator.
            </Alert>
          )}
          <TextField
            autoFocus
            margin="dense"
            label="Email"
            type="text"
            fullWidth
            variant="outlined"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            disabled={isRegistrationDisabled}
            inputProps={{
              name: 'username',
              autoComplete: 'username',
              inputMode: 'email',
              'data-1p-ignore': 'false',
            }}
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

          {mode === 'register' && (
            <TextField
              margin="dense"
              name="name"
              label="Full Name"
              type="text"
              autoComplete="name"
              fullWidth
              variant="outlined"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              disabled={isRegistrationDisabled}
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
          )}

          <TextField
            margin="dense"
            label="Password"
            type="password"
            fullWidth
            variant="outlined"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={isRegistrationDisabled}
            inputProps={{
              name: 'password',
              autoComplete: mode === 'login' ? 'current-password' : 'new-password',
              'data-1p-ignore': 'false',
            }}
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

          {mode === 'login' && (
            <Box sx={{ width: '100%', textAlign: 'right', mt: 1, mb: 1 }}>
              <Typography
                component="span"
                sx={{
                  color: '#A0AEC0',
                  fontSize: '0.875rem',
                }}
              >               
                <Button
                  variant="text"
                  size="small"
                  onClick={() => {
                    handleClose();
                    router.navigate('password-reset');
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
                  Password Reset
                </Button>
              </Typography>
            </Box>
          )}

          {mode === 'register' && (
            <TextField
              margin="dense"
              name="password-confirm"
              label="Confirm Password"
              type="password"
              autoComplete="new-password"
              fullWidth
              variant="outlined"
              value={passwordConfirm}
              onChange={(e) => setPasswordConfirm(e.target.value)}
              disabled={isRegistrationDisabled}
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
          {error && (
            <Alert severity="error" sx={{ width: '100%', mb: 1 }}>
              {error}
            </Alert>
          )}
          <Button
            type="submit"
            variant="contained"
            color="secondary"
            disabled={loading || isRegistrationDisabled}
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
                  setFullName('');
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

