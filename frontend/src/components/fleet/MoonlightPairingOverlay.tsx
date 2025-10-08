import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Typography,
  Box,
  Alert,
  CircularProgress,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  Chip,
} from '@mui/material'
import {
  PhoneAndroid as PhoneIcon,
  Computer as ComputerIcon,
  Tv as TvIcon,
  SportsEsports as GamepadIcon,
  AddBox as AddComputerIcon,
} from '@mui/icons-material'

import useApi from '../../hooks/useApi'

interface PendingPairRequest {
  client_name: string
  uuid: string
  pin: string
  expires_at: number
}

interface MoonlightPairingOverlayProps {
  open: boolean
  onClose: () => void
  onPairingComplete?: () => void
}

const MoonlightPairingOverlay: FC<MoonlightPairingOverlayProps> = ({
  open,
  onClose,
  onPairingComplete,
}) => {
  const api = useApi()
  
  const [pendingRequests, setPendingRequests] = useState<PendingPairRequest[]>([])
  const [selectedRequest, setSelectedRequest] = useState<PendingPairRequest | null>(null)
  const [enteredPin, setEnteredPin] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const loadPendingRequests = async (showLoading = false) => {
    try {
      if (showLoading) {
        setLoading(true)
      }
      setError(null)

      const response = await api.get('/api/v1/wolf/pairing/pending')

      // api.get() already returns res.data, so response IS the array
      const requests = response || []
      setPendingRequests(requests)
    } catch (err: any) {
      console.error('Failed to load pending pair requests:', err)
      setError(err.message || 'Failed to load pending requests')
    } finally {
      if (showLoading) {
        setLoading(false)
      }
    }
  }

  const completePairing = async () => {
    if (!selectedRequest || !enteredPin.trim()) {
      return
    }

    try {
      setLoading(true)
      setError(null)

      await api.post('/api/v1/wolf/pairing/complete', {
        uuid: selectedRequest.uuid,
        pin: enteredPin,
      })

      setSuccess(true)
      setEnteredPin('')
      
      // Refresh pending requests
      await loadPendingRequests()
      
      if (onPairingComplete) {
        onPairingComplete()
      }

      // Auto close after success
      setTimeout(() => {
        handleClose()
      }, 2000)
      
    } catch (err: any) {
      console.error('Failed to complete pairing:', err)
      setError(err.message || 'Pairing failed')
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    setSelectedRequest(null)
    setEnteredPin('')
    setError(null)
    setSuccess(false)
    onClose()
  }

  const getClientIcon = (clientName: string) => {
    const name = clientName.toLowerCase()
    if (name.includes('android') || name.includes('phone')) {
      return <PhoneIcon />
    } else if (name.includes('tv') || name.includes('android tv')) {
      return <TvIcon />
    } else if (name.includes('nintendo') || name.includes('switch') || name.includes('steam')) {
      return <GamepadIcon />
    } else {
      return <ComputerIcon />
    }
  }

  const formatTimeRemaining = (expiresAt: number) => {
    const now = Date.now()
    const remaining = Math.max(0, expiresAt - now)
    const minutes = Math.floor(remaining / 60000)
    const seconds = Math.floor((remaining % 60000) / 1000)
    return `${minutes}:${seconds.toString().padStart(2, '0')}`
  }

  useEffect(() => {
    if (open) {
      loadPendingRequests(true) // Show loading spinner on initial load

      // Poll for new requests every 5 seconds (no loading spinner during polling)
      const interval = setInterval(() => loadPendingRequests(false), 5000)
      return () => clearInterval(interval)
    }
  }, [open])

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        Pair Moonlight Client
      </DialogTitle>

      <DialogContent>
        {success ? (
          <Alert severity="success" sx={{ mb: 2 }}>
            Pairing completed successfully! You can now access your sessions via Moonlight.
          </Alert>
        ) : (
          <>
            <Alert severity="info" icon={<AddComputerIcon />} sx={{ mb: 2 }}>
              <Typography variant="body2" fontWeight="bold">
                In Moonlight: Add PC → Enter <code style={{ padding: '2px 6px', background: 'rgba(0,0,0,0.1)', borderRadius: '3px' }}>{window.location.hostname}</code>
              </Typography>
              <Typography variant="caption" sx={{ mt: 1, display: 'block' }}>
                For iOS: Use <a href="https://apps.apple.com/us/app/voidlink/id6747717070" target="_blank" rel="noopener noreferrer" style={{ color: 'inherit', textDecoration: 'underline' }}>VoidLink</a> - allows external network connections outside your LAN
              </Typography>
            </Alert>

            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
              After adding the PC, Moonlight will show a 4-digit PIN. Select the pairing request below and enter that PIN to complete the connection.
            </Typography>

            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}

            {loading && !selectedRequest ? (
              <Box display="flex" justifyContent="center" py={4}>
                <CircularProgress />
              </Box>
            ) : pendingRequests.length === 0 ? (
              <Alert severity="info">
                No pending pairing requests. Start the pairing process from your Moonlight client to continue.
              </Alert>
            ) : !selectedRequest ? (
              <>
                <Typography variant="subtitle2" gutterBottom>
                  Pending Pairing Requests: ({pendingRequests.length})
                </Typography>
                {console.log('🎨 Rendering pending requests:', pendingRequests)}
                <List>
                  {pendingRequests.map((request) => (
                    <ListItem
                      key={request.uuid}
                      button
                      onClick={() => setSelectedRequest(request)}
                      sx={{
                        border: 1,
                        borderColor: 'divider',
                        borderRadius: 1,
                        mb: 1,
                      }}
                    >
                      <ListItemIcon>
                        {getClientIcon(request.client_name)}
                      </ListItemIcon>
                      <ListItemText
                        primary={request.client_name}
                        secondary={`Expires in: ${formatTimeRemaining(request.expires_at)}`}
                      />
                      <Chip label="Click to pair" size="small" />
                    </ListItem>
                  ))}
                </List>
              </>
            ) : (
              <Box>
                <Typography variant="subtitle2" gutterBottom>
                  Pairing with: {selectedRequest.client_name}
                </Typography>
                
                <Alert severity="info" sx={{ mb: 2 }}>
                  Enter the PIN displayed on your Moonlight client to complete the pairing.
                </Alert>

                <TextField
                  fullWidth
                  label="PIN Code"
                  value={enteredPin}
                  onChange={(e) => setEnteredPin(e.target.value)}
                  placeholder="Enter 4-digit PIN"
                  inputProps={{
                    maxLength: 4,
                    pattern: '[0-9]*',
                  }}
                  sx={{ mb: 2 }}
                  autoFocus
                />

                <Box display="flex" gap={1}>
                  <Button
                    variant="outlined"
                    onClick={() => setSelectedRequest(null)}
                  >
                    Back
                  </Button>
                  <Button
                    variant="contained"
                    onClick={completePairing}
                    disabled={!enteredPin.trim() || enteredPin.length !== 4 || loading}
                  >
                    {loading ? <CircularProgress size={20} /> : 'Complete Pairing'}
                  </Button>
                </Box>
              </Box>
            )}
          </>
        )}
      </DialogContent>

      <DialogActions>
        <Button onClick={() => loadPendingRequests(true)} disabled={loading}>
          Refresh
        </Button>
        <Button onClick={handleClose}>
          {success ? 'Close' : 'Cancel'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default MoonlightPairingOverlay