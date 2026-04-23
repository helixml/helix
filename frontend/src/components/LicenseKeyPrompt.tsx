import React, { useState, useEffect, useCallback, useRef } from "react";
import { Box, Button, TextField, Typography, Link, Alert } from "@mui/material";
import { useAccount } from "../hooks/useAccount";
import useApi from "../hooks/useApi";

const PENDING_LICENSE_KEY = "helix_pending_license_key";

interface LicenseKeyPromptProps {
  gracePeriodExpired?: boolean;
}

export const LicenseKeyPrompt: React.FC<LicenseKeyPromptProps> = ({
  gracePeriodExpired = false,
}) => {
  const account = useAccount();
  const api = useApi();
  const [licenseKey, setLicenseKey] = useState(
    () => localStorage.getItem(PENDING_LICENSE_KEY) || "",
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const isLoggedIn = !!account.user;

  const submitLicenseKey = useCallback(
    async (key: string) => {
      setLoading(true);
      setError(null);
      try {
        await api.post("/api/v1/license", {
          license_key: key,
        });
        localStorage.removeItem(PENDING_LICENSE_KEY);
        window.location.reload();
      } catch (error: any) {
        console.error("Error setting license key:", error);
        setError(
          error?.response?.data || error?.message || "Failed to set license key",
        );
      } finally {
        setLoading(false);
      }
    },
    [api],
  );

  // Auto-submit pending license key after login
  const submittedRef = useRef(false);
  useEffect(() => {
    if (!isLoggedIn || submittedRef.current) return;
    const pending = localStorage.getItem(PENDING_LICENSE_KEY);
    if (pending) {
      submittedRef.current = true;
      submitLicenseKey(pending);
    }
  }, [isLoggedIn, submitLicenseKey]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!licenseKey.trim()) return;

    if (isLoggedIn) {
      await submitLicenseKey(licenseKey);
    } else {
      // Store for later and prompt login
      localStorage.setItem(PENDING_LICENSE_KEY, licenseKey);
      setSaved(true);
      account.setShowLoginWindow(true);
    }
  };

  return (
    <Box
      sx={{
        position: "fixed",
        top: 0,
        right: 0,
        bottom: 0,
        width: 500,
        maxWidth: "100vw",
        zIndex: 9999,
        backgroundColor: "background.paper",
        borderLeft: "1px solid",
        borderColor: "divider",
        display: "flex",
        flexDirection: "column",
        overflow: "auto",
        p: 3,
      }}
    >
      {gracePeriodExpired && (
        <Alert severity="error" sx={{ mb: 2 }}>
          <strong>License required:</strong> The application has been disabled
          because no valid license key was entered. Please enter a license key
          below to continue using Helix.
        </Alert>
      )}
      <Typography variant="h5" gutterBottom>
        Get a License Key to use Helix ✨
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        From $199/year for individual developer licenses
      </Typography>
      {account.serverConfig?.license && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          License Expired! Organization:{" "}
          {account.serverConfig.license.organization} | Valid Until:{" "}
          {new Date(
            account.serverConfig.license.valid_until,
          ).toLocaleDateString()}{" "}
          | Users: {account.serverConfig.license.limits?.users} | Machines:{" "}
          {account.serverConfig.license.limits?.machines}
        </Alert>
      )}
      <Typography paragraph>
        Buy licenses:{" "}
        <Link
          href="https://helix.ml/account/licenses"
          target="_blank"
          rel="noopener"
        >
          helix.ml/account/licenses
        </Link>
      </Typography>
      {saved && !isLoggedIn && (
        <Alert severity="info" sx={{ mb: 2 }}>
          License key saved. Please log in to activate.
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
          onChange={(e) => {
            setLicenseKey(e.target.value);
            setSaved(false);
          }}
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
          {loading ? "Activating..." : "Save License Key"}
        </Button>
      </form>
    </Box>
  );
};
