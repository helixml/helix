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
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material'
import {
  PhoneAndroid as PhoneIcon,
  Computer as ComputerIcon,
  Tv as TvIcon,
  SportsEsports as GamepadIcon,
  AddBox as AddComputerIcon,
  Storage as StorageIcon,
} from '@mui/icons-material'
import { useQuery } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import { TypesWolfInstanceResponse } from '../../api/api'

interface PendingPairRequest {
  client_name: string
  uuid: string
  pin: string
  expires_at: number
  wolf_instance_id?: string  // Which Wolf instance this request is from
  wolf_instance_name?: string  // Display name of the Wolf instance
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
  const apiClient = api.getApiClient()

  const [pendingRequests, setPendingRequests] = useState<PendingPairRequest[]>([])
  const [selectedRequest, setSelectedRequest] = useState<PendingPairRequest | null>(null)
  const [enteredPin, setEnteredPin] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  // Fetch list of registered Wolf instances
  const { data: wolfInstances, isLoading: isLoadingInstances } = useQuery({
    queryKey: ['wolf-instances'],
    queryFn: async () => {
      const response = await apiClient.v1WolfInstancesList()
      return response.data
    },
    enabled: open, // Only fetch when dialog is open
    refetchInterval: 10000,
  })

  // Aggregate pairing requests from all Wolf instances
  const loadPendingRequests = async (showLoading = false) => {
    if (!wolfInstances || wolfInstances.length === 0) {
      setPendingRequests([])
      return
    }

    try {
      if (showLoading) {
        setLoading(true)
      }
      setError(null)

      // Fetch pairing requests from each Wolf instance in parallel
      const allRequests: PendingPairRequest[] = []
      await Promise.all(
        wolfInstances.map(async (instance) => {
          try {
            const response = await api.get(`/api/v1/wolf/pairing/pending?wolf_instance_id=${instance.id}`)
            const requests = response || []
            // Add wolf instance info to each request
            requests.forEach((req: PendingPairRequest) => {
              allRequests.push({
                ...req,
                wolf_instance_id: instance.id,
                wolf_instance_name: instance.name,
              })
            })
          } catch (err) {
            console.warn(`Failed to fetch pairing requests from Wolf instance ${instance.name}:`, err)
            // Continue with other instances
          }
        })
      )

      setPendingRequests(allRequests)
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
    if (!selectedRequest || !enteredPin.trim() || !selectedRequest.wolf_instance_id) {
      return
    }

    try {
      setLoading(true)
      setError(null)

      // Pass wolf_instance_id as query parameter
      await api.post(`/api/v1/wolf/pairing/complete?wolf_instance_id=${selectedRequest.wolf_instance_id}`, {
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
    if (open && wolfInstances && wolfInstances.length > 0) {
      loadPendingRequests(true) // Show loading spinner on initial load

      // Poll for new requests every 5 seconds (no loading spinner during polling)
      const interval = setInterval(() => loadPendingRequests(false), 5000)
      return () => clearInterval(interval)
    }
  }, [open, wolfInstances])

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
            <Alert severity="error" sx={{ mb: 2, fontWeight: 'bold' }}>
              <Typography variant="body2" fontWeight="bold" sx={{ mb: 1 }}>
                CRITICAL: Your Moonlight client MUST connect at 1080p resolution @ 60Hz (1920x1080 @ 60fps)
              </Typography>
              <Typography variant="body2">
                Using any other resolution or frame rate will result in severe video corruption and unusable streaming quality.
                Before connecting, configure your Moonlight client's streaming settings to exactly 1080p @ 60Hz.
              </Typography>
              <Typography variant="caption" sx={{ mt: 1, display: 'block', fontStyle: 'italic' }}>
                In Moonlight settings: Video → Resolution: 1080p (1920x1080) → Frame rate: 60 FPS
              </Typography>
            </Alert>

            <Alert severity="info" icon={<AddComputerIcon />} sx={{ mb: 2 }}>
              <Typography variant="body2" fontWeight="bold">
                In Moonlight: Add PC → Enter <code style={{ padding: '2px 6px', background: 'rgba(0,0,0,0.1)', borderRadius: '3px' }}>{window.location.hostname}</code>
              </Typography>
              <Typography variant="caption" sx={{ mt: 1, display: 'block' }}>
                For iOS: Use <a href="https://apps.apple.com/us/app/voidlink/id6747717070" target="_blank" rel="noopener noreferrer" style={{ color: 'inherit', textDecoration: 'underline' }}>VoidLink</a> - allows external network connections outside your LAN
              </Typography>
            </Alert>

            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
              After adding the PC and configuring 1080p @ 60Hz settings, Moonlight will show a 4-digit PIN. Select the pairing request below and enter that PIN to complete the connection.
            </Typography>

            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}

            {/* Show loading state while fetching Wolf instances or pairing requests */}
            {(isLoadingInstances || (loading && !selectedRequest)) ? (
              <Box display="flex" justifyContent="center" py={4}>
                <CircularProgress />
              </Box>
            ) : !wolfInstances || wolfInstances.length === 0 ? (
              <Alert severity="info">
                No Wolf sandbox instances are registered. Install a sandbox first using:
                <br />
                <code style={{ fontSize: '0.8em' }}>export RUNNER_TOKEN=TOKEN && ./install.sh --sandbox --controlplane-url URL</code>
              </Alert>
            ) : pendingRequests.length === 0 ? (
              <Alert severity="info">
                No pending pairing requests from any of the {wolfInstances.length} registered Wolf instance{wolfInstances.length !== 1 ? 's' : ''}.
                Start the pairing process from your Moonlight client to continue.
              </Alert>
            ) : !selectedRequest ? (
              <>
                <Typography variant="subtitle2" gutterBottom>
                  Pending Pairing Requests: ({pendingRequests.length}) from {new Set(pendingRequests.map(r => r.wolf_instance_id)).size} sandbox{new Set(pendingRequests.map(r => r.wolf_instance_id)).size !== 1 ? 'es' : ''}
                </Typography>
                {console.log('🎨 Rendering pending requests:', pendingRequests)}
                <List>
                  {pendingRequests.map((request) => (
                    <ListItem
                      key={`${request.wolf_instance_id}-${request.uuid}`}
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
                        primary={
                          <Box display="flex" alignItems="center" gap={1}>
                            <span>{request.client_name}</span>
                            {request.wolf_instance_name && (
                              <Chip
                                icon={<StorageIcon sx={{ fontSize: '0.9rem' }} />}
                                label={request.wolf_instance_name}
                                size="small"
                                variant="outlined"
                                sx={{ height: 20, '& .MuiChip-label': { px: 0.75 } }}
                              />
                            )}
                          </Box>
                        }
                        secondary={`Expires in: ${formatTimeRemaining(request.expires_at)}`}
                      />
                      <Chip label="Click to pair" size="small" color="primary" />
                    </ListItem>
                  ))}
                </List>
              </>
            ) : (
              <Box>
                <Typography variant="subtitle2" gutterBottom>
                  Pairing with: {selectedRequest.client_name}
                  {selectedRequest.wolf_instance_name && (
                    <Chip
                      icon={<StorageIcon sx={{ fontSize: '0.9rem' }} />}
                      label={`on ${selectedRequest.wolf_instance_name}`}
                      size="small"
                      sx={{ ml: 1, height: 20 }}
                    />
                  )}
                </Typography>

                <Alert severity="warning" sx={{ mb: 2, fontWeight: 'bold' }}>
                  <Typography variant="body2" fontWeight="bold">
                    ⚠️ REMINDER: Ensure Moonlight is set to 1080p @ 60Hz before connecting!
                  </Typography>
                  <Typography variant="caption">
                    Other resolutions/framerates will cause video corruption
                  </Typography>
                </Alert>

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
