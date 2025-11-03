import React, { useState } from 'react';
import {
  Button,
  Box,
  Typography,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Alert,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Divider,
  IconButton,
  Tooltip,
  Link,
} from '@mui/material';
import {
  SportsEsports as GamepadIcon,
  Computer as ComputerIcon,
  Launch as LaunchIcon,
  ContentCopy as CopyIcon,
  Info as InfoIcon,
  Android as AndroidIcon,
  Apple as AppleIcon,
  DesktopWindows as WindowsIcon,
} from '@mui/icons-material';

interface MoonlightConnectionButtonProps {
  sessionId: string;
  hostname?: string;
  port?: number;
  wolfLobbyPin?: string; // PIN for Wolf lobbies mode
  wolfMode?: string; // "apps" or "lobbies"
}

const MoonlightConnectionButton: React.FC<MoonlightConnectionButtonProps> = ({
  sessionId,
  hostname = window.location.hostname,
  port = 47989, // Wolf's default HTTP port
  wolfLobbyPin,
  wolfMode = "apps",
}) => {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  // Generate connection details
  const httpsUrl = `https://${hostname}:47984`; // Wolf's HTTPS port for pairing
  const httpUrl = `http://${hostname}:${port}`; // Wolf's HTTP port
  const appId = 1; // From our Wolf config - "Helix Desktop"
  // Use wolfLobbyPin if in lobbies mode, otherwise use default password
  const password = wolfMode === "lobbies" && wolfLobbyPin ? wolfLobbyPin : "helix123";

  const handleCopy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <>
      <Button
        variant="outlined"
        startIcon={<GamepadIcon />}
        onClick={() => setOpen(true)}
        sx={{ mr: 1 }}
      >
        Moonlight
      </Button>

      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          <Box display="flex" alignItems="center" gap={1}>
            <GamepadIcon color="primary" />
            Moonlight Game Streaming Connection
          </Box>
        </DialogTitle>
        
        <DialogContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            Connect to this Helix desktop using Moonlight for high-performance, 
            low-latency streaming with gamepad support and hardware acceleration.
          </Alert>

          <Alert severity="warning" sx={{ mb: 2 }}>
            <Typography variant="body2">
              <strong>Manual Setup Required:</strong> Moonlight clients don't support URL launching yet. 
              You'll need to manually add this server in your Moonlight client.
            </Typography>
          </Alert>

          <Divider sx={{ my: 2 }} />

          <Typography variant="h6" gutterBottom>
            Manual Connection Details
          </Typography>
          
          <Box sx={{ mb: 2 }}>
            <TextField
              label="Server Address"
              value={hostname}
              fullWidth
              InputProps={{
                readOnly: true,
                endAdornment: (
                  <IconButton onClick={() => handleCopy(hostname)}>
                    <CopyIcon />
                  </IconButton>
                ),
              }}
              sx={{ mb: 1 }}
            />
            
            <TextField
              label="Port (HTTP)"
              value={port}
              fullWidth
              InputProps={{
                readOnly: true,
                endAdornment: (
                  <IconButton onClick={() => handleCopy(port.toString())}>
                    <CopyIcon />
                  </IconButton>
                ),
              }}
              sx={{ mb: 1 }}
            />

            <TextField
              label="HTTP URL (for connection)"
              value={httpUrl}
              fullWidth
              InputProps={{
                readOnly: true,
                endAdornment: (
                  <IconButton onClick={() => handleCopy(httpUrl)}>
                    <CopyIcon />
                  </IconButton>
                ),
              }}
              sx={{ mb: 1 }}
              helperText="Use this URL when adding server manually in Moonlight client"
            />

            <TextField
              label="HTTPS URL (for pairing)"
              value={httpsUrl}
              fullWidth
              InputProps={{
                readOnly: true,
                endAdornment: (
                  <IconButton onClick={() => handleCopy(httpsUrl)}>
                    <CopyIcon />
                  </IconButton>
                ),
              }}
              sx={{ mb: 1 }}
              helperText="Use if client requires HTTPS or during initial pairing"
            />

            {wolfMode === "lobbies" && wolfLobbyPin && (
              <TextField
                label="Lobby PIN"
                value={password}
                type="password"
                fullWidth
                InputProps={{
                  readOnly: true,
                  endAdornment: (
                    <IconButton onClick={() => handleCopy(password)}>
                      <CopyIcon />
                    </IconButton>
                  ),
                }}
                helperText="Use this PIN to join the lobby (required for multi-user access)"
              />
            )}
          </Box>

          {copied && (
            <Alert severity="success" sx={{ mb: 2 }}>
              Copied to clipboard!
            </Alert>
          )}

          <Divider sx={{ my: 2 }} />

          <Typography variant="h6" gutterBottom>
            Moonlight Clients
          </Typography>
          <List dense>
            <ListItem>
              <ListItemIcon><WindowsIcon /></ListItemIcon>
              <ListItemText 
                primary="Windows PC" 
                secondary={
                  <Link href="https://github.com/moonlight-stream/moonlight-qt/releases" target="_blank">
                    Download Moonlight Qt
                  </Link>
                }
              />
            </ListItem>
            <ListItem>
              <ListItemIcon><AndroidIcon /></ListItemIcon>
              <ListItemText 
                primary="Android" 
                secondary={
                  <Link href="https://play.google.com/store/apps/details?id=com.limelight" target="_blank">
                    Google Play Store
                  </Link>
                }
              />
            </ListItem>
            <ListItem>
              <ListItemIcon><AppleIcon /></ListItemIcon>
              <ListItemText 
                primary="iOS/Apple TV" 
                secondary={
                  <Link href="https://apps.apple.com/app/moonlight-game-streaming/id1000551796" target="_blank">
                    App Store
                  </Link>
                }
              />
            </ListItem>
            <ListItem>
              <ListItemIcon><ComputerIcon /></ListItemIcon>
              <ListItemText 
                primary="Linux/macOS" 
                secondary={
                  <Link href="https://github.com/moonlight-stream/moonlight-qt/releases" target="_blank">
                    Moonlight Qt (Cross-platform)
                  </Link>
                }
              />
            </ListItem>
          </List>

          <Alert severity="info" sx={{ mt: 2 }}>
            <Typography variant="body2">
              <strong>Setup Instructions:</strong>
              <br />1. Install Moonlight client on your device
              <br />2. Add server manually using the hostname/IP above
              <br />3. {wolfMode === "lobbies" ? "Enter the lobby PIN when prompted" : "Enter the PIN if prompted during pairing"}
              <br />4. {wolfMode === "lobbies"
                ? "Join the lobby - multiple users can connect simultaneously"
                : "Select from available apps: Helix Desktop or individual sessions"}
            </Typography>
          </Alert>

          <Alert severity="success" sx={{ mt: 1 }}>
            <Typography variant="body2">
              <strong>Performance Benefits:</strong>
              <br />• Hardware-accelerated H.264/H.265 encoding
              <br />• Low-latency streaming optimized for gaming
              <br />• Gamepad and controller support
              <br />• Up to 4K@60Hz streaming capability
            </Typography>
          </Alert>
        </DialogContent>

        <DialogActions>
          <Button onClick={() => setOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default MoonlightConnectionButton;