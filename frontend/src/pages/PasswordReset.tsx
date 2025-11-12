import React, { useState } from 'react';
import {
  Container,
  Box,
  Typography,
  TextField,
  Button,
  Alert,
  Paper,
} from '@mui/material';
import useApi from '../hooks/useApi';
import useSnackbar from '../hooks/useSnackbar';
import { TypesPasswordResetRequest } from '../api/api';
import useThemeConfig from '../hooks/useThemeConfig';

const PasswordReset: React.FC = () => {
  const [email, setEmail] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);

  const api = useApi();
  const snackbar = useSnackbar();
  const apiClient = api.getApiClient();
  const themeConfig = useThemeConfig();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!email) {
      setError('Email is required');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const request: TypesPasswordResetRequest = {
        email,
      };

      await apiClient.v1AuthPasswordResetCreate(request);
      
      setSuccess(true);
      snackbar.success('Password reset email sent');
    } catch (err: any) {
      const errorMessage = err?.response?.data?.error || err?.message || 'Failed to send password reset email';
      setError(errorMessage);
      snackbar.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

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
            Reset Password
          </Typography>

          {success ? (
            <Box>
              <Alert
                severity="success"
                sx={{
                  mb: 3,
                  backgroundColor: 'rgba(76, 175, 80, 0.1)',
                  color: '#4CAF50',
                  border: '1px solid rgba(76, 175, 80, 0.3)',
                }}
              >
                <Typography variant="body1" sx={{ mb: 1, fontWeight: 600 }}>
                  Check your inbox
                </Typography>
                <Typography variant="body2">
                  We've sent a password reset link to {email}. Please check your email and click on the link to reset your password.
                </Typography>
              </Alert>
              <Button
                fullWidth
                variant="outlined"
                onClick={() => {
                  setSuccess(false);
                  setEmail('');
                }}
                sx={{
                  borderColor: themeConfig.tealRoot,
                  color: themeConfig.tealRoot,
                  '&:hover': {
                    borderColor: themeConfig.tealRoot,
                    backgroundColor: `${themeConfig.tealRoot}10`,
                  },
                }}
              >
                Send another email
              </Button>
            </Box>
          ) : (
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
                Enter your email address and we'll send you a link to reset your password.
              </Typography>

              <TextField
                autoFocus
                fullWidth
                label="Email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
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
                {loading ? 'Sending...' : 'Send Reset Link'}
              </Button>
            </form>
          )}
        </Paper>
      </Container>
    </Box>
  );
};

export default PasswordReset;

