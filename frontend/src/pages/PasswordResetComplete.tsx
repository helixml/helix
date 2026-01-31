import React, { useState, useEffect } from 'react';
import {
  Container,
  Box,
  Typography,
  TextField,
  Button,
  Alert,
  Paper,
  InputAdornment,
  IconButton,
} from '@mui/material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import useRouter from '../hooks/useRouter';
import { TypesPasswordResetCompleteRequest } from '../api/api';
import useThemeConfig from '../hooks/useThemeConfig';

const PasswordResetComplete: React.FC = () => {
  const [password, setPassword] = useState('');
  const [passwordConfirm, setPasswordConfirm] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [showPasswordConfirm, setShowPasswordConfirm] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [token, setToken] = useState<string | null>(null);

  const api = useApi();
  const snackbar = useSnackbar();
  const router = useRouter();
  const apiClient = api.getApiClient();
  const themeConfig = useThemeConfig();

  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const accessToken = urlParams.get('token');
    
    if (!accessToken) {
      setError('Missing access token. Please use the link from your email.');
    } else {
      setToken(accessToken);
    }
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!token) {
      setError('Missing access token. Please use the link from your email.');
      return;
    }

    if (!password || !passwordConfirm) {
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
      const request: TypesPasswordResetCompleteRequest = {
        access_token: token,
        new_password: password,
      };

      await apiClient.v1AuthPasswordResetCompleteCreate(request);
      
      snackbar.success('Password reset successful. You are now logged in.');
      
      setTimeout(() => {
        window.location.href = '/';
      }, 1000);
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || err?.message || 'Failed to reset password';
      setError(errorMessage);
      snackbar.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  if (!token) {
    return (
      <Box
        sx={{
          minHeight: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: themeConfig.darkBackgroundImage || `linear-gradient(135deg, ${themeConfig.darkBackgroundColor} 0%, ${themeConfig.neutral800} 100%)`,
          p: 3,
        }}
      >
        <Container maxWidth="sm">
          <Paper
            elevation={0}
            sx={{
              p: 4,
              borderRadius: 2,
              backgroundColor: themeConfig.darkPanel,
              border: `1px solid ${themeConfig.darkBorder}`,
            }}
          >
            <Alert
              severity="error"
              sx={{
                backgroundColor: 'rgba(244, 67, 54, 0.1)',
                color: '#F44336',
                border: '1px solid rgba(244, 67, 54, 0.3)',
              }}
            >
              {error || 'Missing access token. Please use the link from your email.'}
            </Alert>
          </Paper>
        </Container>
      </Box>
    );
  }

  return (
      <Box
        sx={{
          minHeight: '100vh',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: themeConfig.darkBackgroundImage || `linear-gradient(135deg, ${themeConfig.darkBackgroundColor} 0%, ${themeConfig.neutral800} 100%)`,
          p: 3,
        }}
      >
        <Container maxWidth="sm">
          <Paper
            elevation={0}
            sx={{
              p: 4,
              borderRadius: 2,
              backgroundColor: themeConfig.darkPanel,
              border: `1px solid ${themeConfig.darkBorder}`,
            }}
          >
          <Typography
            variant="h4"
            sx={{
              mb: 3,
              fontWeight: 700,
              textAlign: 'center',
              background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
              WebkitBackgroundClip: 'text',
              WebkitTextFillColor: 'transparent',
              backgroundClip: 'text',
            }}
          >
            Set New Password
          </Typography>

          <form onSubmit={handleSubmit}>
            {error && (
              <Alert
                severity="error"
                sx={{
                  mb: 3,
                  backgroundColor: 'rgba(244, 67, 54, 0.1)',
                  color: '#F44336',
                  border: '1px solid rgba(244, 67, 54, 0.3)',
                }}
              >
                {error}
              </Alert>
            )}

            <Typography
              variant="body2"
              sx={{
                mb: 3,
                color: themeConfig.darkTextFaded,
                textAlign: 'center',
              }}
            >
              Please enter your new password below.
            </Typography>

            <TextField
              autoFocus
              fullWidth
              label="New Password"
              type={showPassword ? 'text' : 'password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={loading}
              sx={{
                mb: 2,
                '& .MuiOutlinedInput-root': {
                  '& fieldset': {
                    borderColor: themeConfig.darkBorder,
                  },
                  '&:hover fieldset': {
                    borderColor: themeConfig.tealRoot,
                  },
                  '&.Mui-focused fieldset': {
                    borderColor: themeConfig.tealRoot,
                  },
                },
                '& .MuiInputLabel-root': {
                  color: themeConfig.darkTextFaded,
                },
                '& .MuiInputLabel-root.Mui-focused': {
                  color: themeConfig.tealRoot,
                },
                '& .MuiOutlinedInput-input': {
                  color: themeConfig.darkText,
                },
              }}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      onClick={() => setShowPassword(!showPassword)}
                      edge="end"
                      sx={{ color: themeConfig.darkTextFaded }}
                    >
                      {showPassword ? <VisibilityOffIcon /> : <VisibilityIcon />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />

            <TextField
              fullWidth
              label="Confirm New Password"
              type={showPasswordConfirm ? 'text' : 'password'}
              value={passwordConfirm}
              onChange={(e) => setPasswordConfirm(e.target.value)}
              disabled={loading}
              sx={{
                mb: 3,
                '& .MuiOutlinedInput-root': {
                  '& fieldset': {
                    borderColor: themeConfig.darkBorder,
                  },
                  '&:hover fieldset': {
                    borderColor: themeConfig.tealRoot,
                  },
                  '&.Mui-focused fieldset': {
                    borderColor: themeConfig.tealRoot,
                  },
                },
                '& .MuiInputLabel-root': {
                  color: themeConfig.darkTextFaded,
                },
                '& .MuiInputLabel-root.Mui-focused': {
                  color: themeConfig.tealRoot,
                },
                '& .MuiOutlinedInput-input': {
                  color: themeConfig.darkText,
                },
              }}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      onClick={() => setShowPasswordConfirm(!showPasswordConfirm)}
                      edge="end"
                      sx={{ color: themeConfig.darkTextFaded }}
                    >
                      {showPasswordConfirm ? <VisibilityOffIcon /> : <VisibilityIcon />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />

            <Button
              type="submit"
              fullWidth
              variant="contained"
              disabled={loading}
              sx={{
                mb: 2,
                backgroundColor: themeConfig.tealRoot,
                color: '#000',
                fontWeight: 600,
                '&:hover': {
                  backgroundColor: '#00B8CC',
                },
                '&:disabled': {
                  backgroundColor: `${themeConfig.tealRoot}80`,
                },
              }}
            >
              {loading ? 'Resetting Password...' : 'Reset Password'}
            </Button>
          </form>
        </Paper>
      </Container>
    </Box>
  );
};

export default PasswordResetComplete;

