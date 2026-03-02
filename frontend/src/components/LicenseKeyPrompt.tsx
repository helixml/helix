import React, { useState } from 'react';
import { Box, Button, TextField, Typography, Link, Alert } from '@mui/material';
import { useAccount } from '../hooks/useAccount';
import useApi from '../hooks/useApi';

export const LicenseKeyPrompt: React.FC = () => {
  const account = useAccount();
  const api = useApi();
  const [licenseKey, setLicenseKey] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Add debug logging
  console.log('LicenseKeyPrompt:', {
    deploymentId: account.serverConfig?.deployment_id,
    serverConfig: account.serverConfig
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await api.post('/api/v1/license', {
        license_key: licenseKey
      });
      window.location.reload(); // Reload to update config
    } catch (error: any) {
      console.error('Error setting license key:', error);
      setError(error?.response?.data || error?.message || 'Failed to set license key');
    } finally {
      setLoading(false);
    }
  };

  const isLoggedIn = !!account.user;

  return (
    <Box sx={{ maxWidth: 600, mx: 'auto', mt: 4, p: 3 }}>
      <Typography variant="h5" gutterBottom>
        Get your free Community License Key! 🕺🏽
      </Typography>
      {account.serverConfig?.license && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          License Expired!
          Organization: {account.serverConfig.license.organization} |
          Valid Until: {new Date(account.serverConfig.license.valid_until).toLocaleDateString()} |
          Users: {account.serverConfig.license.limits.users} |
          Machines: {account.serverConfig.license.limits.machines}
        </Alert>
      )}
      <Typography paragraph>
        Get your free Community license key from{' '}
        <Link href="https://helix.ml/account/licenses" target="_blank" rel="noopener">
          helix.ml/account/licenses
        </Link>
      </Typography>
      {!isLoggedIn && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Please log in with an admin account to activate your license key
        </Alert>
      )}
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      <form onSubmit={handleSubmit}>
        <TextField
          fullWidth
          label="License Key"
          value={licenseKey}
          onChange={(e) => setLicenseKey(e.target.value)}
          margin="normal"
          required
          error={!!error}
          disabled={!isLoggedIn}
        />
        <Button
          type="submit"
          variant="contained"
          color="primary"
          disabled={loading || !isLoggedIn}
          sx={{ mt: 2 }}
        >
          {loading ? 'Saving...' : 'Save License Key'}
        </Button>
      </form>
    </Box>
  );
}; 