import React, { useEffect, useState } from 'react'
import { Alert, Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, Divider, IconButton, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { Copy, ExternalLink } from 'lucide-react'
import { TypesCodexAuthCredentials, TypesOwnerType } from '../../api/api'
import { useQueryClient } from '@tanstack/react-query'
import { copyTextToClipboard } from '../../utils/clipboard'
import { APP_MONO_FONT_FAMILY } from '../../styles/typography'
import {
  useCodexSubscriptions,
  useCreateCodexSubscription,
  useDeleteCodexSubscription,
  usePollCodexLogin,
  useStartCodexLogin,
  codexSubscriptionsQueryKey,
} from '../../services/codexSubscriptionsService'

interface Props {
  orgId?: string
}

export const CODEX_DEVICE_AUTH_URL = 'https://auth.openai.com/codex/device'

const actionSx = { textTransform: 'none' } as const

function parseCredentials(value: string): TypesCodexAuthCredentials {
  const credentials = JSON.parse(value) as TypesCodexAuthCredentials
  if (
    credentials.auth_mode !== 'chatgpt' ||
    !credentials.last_refresh ||
    !credentials.tokens?.id_token ||
    !credentials.tokens.access_token ||
    !credentials.tokens.refresh_token ||
    !credentials.tokens.account_id
  ) {
    throw new Error('This is not a complete ChatGPT Codex auth.json file.')
  }
  return credentials
}

export default function CodexSubscriptionConnect({ orgId }: Props) {
  const queryClient = useQueryClient()
  const { data: subscriptions = [] } = useCodexSubscriptions()
  const createSubscription = useCreateCodexSubscription()
  const deleteSubscription = useDeleteCodexSubscription()
  const startLogin = useStartCodexLogin()
  const [open, setOpen] = useState(false)
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  const [value, setValue] = useState('')
  const [error, setError] = useState('')
  const [loginError, setLoginError] = useState('')
  const [loginSessionId, setLoginSessionId] = useState('')
  const { data: loginStatus } = usePollCodexLogin(loginSessionId)
  const subscription = subscriptions[0]
  const loginFound = loginStatus?.found ?? false
  const deviceCode = loginStatus?.code

  useEffect(() => {
    if (!loginFound) return
    queryClient.invalidateQueries({ queryKey: codexSubscriptionsQueryKey })
    setLoginSessionId('')
    setOpen(false)
  }, [loginFound, queryClient])

  const closeDialog = () => {
    setOpen(false)
    setLoginSessionId('')
    setLoginError('')
  }

  const openDialog = () => {
    setValue('')
    setError('')
    setLoginError('')
    setLoginSessionId('')
    setOpen(true)
    startLogin.mutate(undefined, {
      onSuccess: (result) => setLoginSessionId(result.session_id || ''),
      onError: (err) => {
        setLoginError(err instanceof Error ? err.message : 'Failed to start ChatGPT login')
      },
    })
  }

  const connect = async () => {
    try {
      const credentials = parseCredentials(value)
      await createSubscription.mutateAsync({
        name: 'My Codex Subscription',
        credentials,
        ...(orgId ? { owner_type: TypesOwnerType.OwnerTypeOrg, owner_id: orgId } : {}),
      })
      setValue('')
      setError('')
      closeDialog()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect Codex subscription')
    }
  }

  const disconnectDialog = (
    <Dialog open={disconnectOpen} onClose={() => setDisconnectOpen(false)} maxWidth="xs" fullWidth>
      <DialogTitle>Disconnect ChatGPT Subscription</DialogTitle>
      <DialogContent>
        <DialogContentText>
          Are you sure you want to disconnect this ChatGPT subscription?
          Desktop sessions will stop using your Codex credentials.
        </DialogContentText>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={() => setDisconnectOpen(false)} sx={actionSx}>Cancel</Button>
        <Button
          color="error"
          variant="contained"
          disabled={deleteSubscription.isPending}
          onClick={() => {
            if (!subscription?.id) return
            deleteSubscription.mutate(subscription.id, {
              onSuccess: () => setDisconnectOpen(false),
            })
          }}
          sx={actionSx}
        >
          {deleteSubscription.isPending ? 'Disconnecting…' : 'Disconnect'}
        </Button>
      </DialogActions>
    </Dialog>
  )

  if (subscription?.id) {
    return (
      <>
        <Button
          size="small"
          color="error"
          variant="outlined"
          disabled={deleteSubscription.isPending}
          onClick={() => setDisconnectOpen(true)}
          sx={actionSx}
        >
          Disconnect
        </Button>
        {disconnectDialog}
      </>
    )
  }

  return (
    <>
      <Button
        size="small"
        variant="outlined"
        disabled={open}
        onClick={openDialog}
        sx={actionSx}
      >
        Connect
      </Button>
      <Dialog open={open} onClose={closeDialog} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ pb: 0.5 }}>Connect ChatGPT Subscription</DialogTitle>
        <DialogContent sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2.5 }}>
          <Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              Open ChatGPT, then enter the device code shown here.
            </Typography>
            <Stack
              direction={{ xs: 'column', sm: 'row' }}
              spacing={1.5}
              alignItems={{ xs: 'stretch', sm: 'center' }}
            >
              <Button
                href={loginStatus?.url || CODEX_DEVICE_AUTH_URL}
                target="_blank"
                rel="noreferrer"
                variant="contained"
                color="secondary"
                startIcon={<ExternalLink size={16} />}
                sx={actionSx}
              >
                Open ChatGPT
              </Button>
              {deviceCode ? (
                <Stack direction="row" alignItems="center" spacing={0.5} sx={{ minHeight: 40 }}>
                  <Typography
                    component="span"
                    sx={{
                      fontFamily: APP_MONO_FONT_FAMILY,
                      fontWeight: 600,
                      letterSpacing: '0.08em',
                      fontSize: '1rem',
                    }}
                  >
                    {deviceCode}
                  </Typography>
                  <Tooltip title="Copy code">
                    <IconButton
                      size="small"
                      aria-label="Copy device code"
                      onClick={() => copyTextToClipboard(deviceCode)}
                    >
                      <Copy size={14} />
                    </IconButton>
                  </Tooltip>
                </Stack>
              ) : (
                <Stack direction="row" alignItems="center" spacing={1} sx={{ minHeight: 40 }}>
                  <CircularProgress size={14} />
                  <Typography variant="body2" color="text.secondary">
                    Waiting for device code…
                  </Typography>
                </Stack>
              )}
            </Stack>
            {loginError && <Alert severity="error" sx={{ mt: 1.5 }}>{loginError}</Alert>}
            {loginStatus?.error && <Alert severity="error" sx={{ mt: 1.5 }}>{loginStatus.error}</Alert>}
          </Box>

          <Divider>
            <Typography variant="caption" color="text.secondary">or paste auth.json</Typography>
          </Divider>

          <Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              Run <Box component="code" sx={{ fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' }}>codex login</Box>
              {' '}locally and paste{' '}
              <Box component="code" sx={{ fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' }}>~/.codex/auth.json</Box>
              {' '}below.
            </Typography>
            <Alert severity="warning" sx={{ mb: 1.5 }}>
              This file contains account credentials. Helix encrypts it before storing it and only releases it to your desktop sessions.
            </Alert>
            <TextField
              fullWidth
              multiline
              minRows={6}
              type="password"
              label="Codex auth.json"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              error={!!error}
              helperText={error}
            />
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, py: 2 }}>
          <Button onClick={closeDialog} sx={actionSx}>Cancel</Button>
          <Button
            variant="contained"
            color="secondary"
            disabled={!value || createSubscription.isPending}
            onClick={connect}
            sx={actionSx}
          >
            Import
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
