import React, { useState } from 'react';
import { Box, Button, TextField, Typography, Link, Alert } from '@mui/material';
import { useAccount } from '../contexts/account';

export const LicenseKeyPrompt: React.FC = () => {
  const { user } = useAccount();
  const [licenseKey, setLicenseKey] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const response = await fetch('/api/v1/license', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ license_key: licenseKey }),
      });
      if (!response.ok) {
        throw new Error(await response.text() || 'Failed to set license key');
      }
      window.location.reload(); // Reload to update config
    } catch (error) {
      console.error('Error setting license key:', error);
      setError(error instanceof Error ? error.message : 'Failed to set license key');
    } finally {
      setLoading(false);
    }
  };

  if (!user?.is_admin) return null;

  return (
    <Box sx={{ maxWidth: 600, mx: 'auto', mt: 4, p: 3 }}>
      <Typography variant="h5" gutterBottom>
        License Key Required
      </Typography>
      <Typography paragraph>
        Please enter your license key from{' '}
        <Link href="https://deploy.helix.ml" target="_blank" rel="noopener">
          deploy.helix.ml
        </Link>
      </Typography>
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
        />
        <Button
          type="submit"
          variant="contained"
          color="primary"
          disabled={loading}
          sx={{ mt: 2 }}
        >
          {loading ? 'Saving...' : 'Save License Key'}
        </Button>
      </form>
    </Box>
  );
}; 