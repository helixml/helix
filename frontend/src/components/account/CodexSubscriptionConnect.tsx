import React, { useEffect, useRef, useState } from 'react'
import { Alert, Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, Divider, IconButton, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { ArrowLeft, CircleCheck, Copy, ExternalLink, FileJson, KeyRound } from 'lucide-react'
import { TypesCodexAuthCredentials, TypesOwnerType } from '../../api/api'
import { useQueryClient } from '@tanstack/react-query'
import { copyTextToClipboard } from '../../utils/clipboard'
import { APP_MONO_FONT_FAMILY } from '../../styles/typography'
import {
  useCodexSubscriptions,
  useCancelCodexLogin,
  useCreateCodexSubscription,
  useDeleteCodexSubscription,
  usePollCodexLogin,
  useStartCodexLogin,
  codexSubscriptionsQueryKey,
} from '../../services/codexSubscriptionsService'
import { codeAgentHarnessesQueryKey } from '../../services/codeAgentHarnessesService'

interface Props {
  orgId?: string
  enableForOrgId?: string
}

export const CODEX_DEVICE_AUTH_URL = 'https://auth.openai.com/codex/device'

const actionSx = { textTransform: 'none' } as const
type ConnectMethod = 'choose' | 'device' | 'import' | 'success'

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

export default function CodexSubscriptionConnect({ orgId, enableForOrgId }: Props) {
  const queryClient = useQueryClient()
  const queryClientRef = useRef(queryClient)
  queryClientRef.current = queryClient
  const { data: subscriptions = [] } = useCodexSubscriptions()
  const createSubscription = useCreateCodexSubscription()
  const deleteSubscription = useDeleteCodexSubscription()
  const startLogin = useStartCodexLogin()
  const cancelLogin = useCancelCodexLogin()
  const [open, setOpen] = useState(false)
  const [method, setMethod] = useState<ConnectMethod>('choose')
  const [disconnectOpen, setDisconnectOpen] = useState(false)
  const [value, setValue] = useState('')
  const [error, setError] = useState('')
  const [loginError, setLoginError] = useState('')
  const [loginSessionId, setLoginSessionId] = useState('')
  const loginSessionIdRef = useRef('')
  const dialogOpenRef = useRef(false)
  const deviceFlowActiveRef = useRef(false)
  const { data: loginStatus } = usePollCodexLogin(loginSessionId)
  const subscription = orgId
    ? subscriptions.find((candidate) => candidate.owner_type === 'org' && candidate.owner_id === orgId)
    : subscriptions.find((candidate) => candidate.owner_type === 'user')
  const loginFound = loginStatus?.found ?? false
  const deviceCode = loginStatus?.code
  const deviceError = loginError || loginStatus?.error

  useEffect(() => {
    if (!loginFound) return
    if (enableForOrgId) {
      queryClientRef.current.invalidateQueries({ queryKey: codeAgentHarnessesQueryKey(enableForOrgId) })
    }
    loginSessionIdRef.current = ''
    deviceFlowActiveRef.current = false
    setLoginSessionId('')
    setMethod('success')
  }, [loginFound, enableForOrgId])

  const closeDialog = () => {
    dialogOpenRef.current = false
    deviceFlowActiveRef.current = false
    if (loginSessionIdRef.current) cancelLogin.mutate(loginSessionIdRef.current)
    loginSessionIdRef.current = ''
    setOpen(false)
    setMethod('choose')
    setLoginSessionId('')
    setLoginError('')
    if (method === 'success') {
      queryClient.invalidateQueries({ queryKey: codexSubscriptionsQueryKey })
    }
  }

  const openDialog = () => {
    setValue('')
    setError('')
    setLoginError('')
    setLoginSessionId('')
    setMethod('choose')
    dialogOpenRef.current = true
    deviceFlowActiveRef.current = false
    setOpen(true)
  }

  const generateDeviceCode = () => {
    setMethod('device')
    deviceFlowActiveRef.current = true
    setLoginError('')
    setLoginSessionId('')
    startLogin.mutate({ organization_id: enableForOrgId }, {
      onSuccess: (result) => {
        const sessionID = result.session_id || ''
        if (!sessionID) return
        if (!dialogOpenRef.current || !deviceFlowActiveRef.current) {
          cancelLogin.mutate(sessionID)
          return
        }
        loginSessionIdRef.current = sessionID
        setLoginSessionId(sessionID)
      },
      onError: (err) => {
        if (!dialogOpenRef.current || !deviceFlowActiveRef.current) return
        setLoginError(err instanceof Error ? err.message : 'Failed to start ChatGPT login')
      },
    })
  }

  const chooseMethod = () => {
    deviceFlowActiveRef.current = false
    if (loginSessionIdRef.current) cancelLogin.mutate(loginSessionIdRef.current)
    loginSessionIdRef.current = ''
    setLoginSessionId('')
    setLoginError('')
    setMethod('choose')
  }

  const connect = async () => {
    try {
      const credentials = parseCredentials(value)
      await createSubscription.mutateAsync({
        name: 'My Codex Subscription',
        credentials,
        organization_id: enableForOrgId,
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
        <DialogTitle sx={{ pb: 0.5 }}>
          {method === 'success' ? 'ChatGPT Subscription Connected' : 'Connect ChatGPT Subscription'}
        </DialogTitle>
        <DialogContent sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2.5 }}>
          {method === 'choose' && (
            <Stack spacing={1.5}>
              <Typography variant="body2" color="text.secondary">
                Choose how to connect your ChatGPT account for Codex.
              </Typography>
              <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 2 }}>
                <Stack direction="row" spacing={1.5} alignItems="flex-start">
                  <KeyRound size={20} />
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography variant="subtitle2">Generate a device code</Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 1.5 }}>
                      Helix starts a temporary headless sandbox and produces a one-time code.
                    </Typography>
                    <Button
                      variant="contained"
                      color="secondary"
                      onClick={generateDeviceCode}
                      sx={actionSx}
                    >
                      Generate code
                    </Button>
                  </Box>
                </Stack>
              </Box>
              <Divider><Typography variant="caption" color="text.secondary">or</Typography></Divider>
              <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 2 }}>
                <Stack direction="row" spacing={1.5} alignItems="flex-start">
                  <FileJson size={20} />
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography variant="subtitle2">Import auth.json</Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 1.5 }}>
                      Run Codex login yourself and paste the resulting credentials.
                    </Typography>
                    <Button variant="outlined" onClick={() => setMethod('import')} sx={actionSx}>
                      Enter credentials
                    </Button>
                  </Box>
                </Stack>
              </Box>
            </Stack>
          )}

          {method === 'device' && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                {deviceCode
                  ? 'Copy this code, then continue to ChatGPT to authorize Codex.'
                  : 'Starting a temporary headless sandbox to generate your code…'}
              </Typography>
              {deviceCode ? (
                <Stack spacing={1.5} alignItems="flex-start">
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={0.5}
                    sx={{ minHeight: 48, px: 1.5, borderRadius: 1, bgcolor: 'action.hover' }}
                  >
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
                        onClick={() => {
                          void copyTextToClipboard(deviceCode).catch((error) => {
                            console.error('Failed to copy device code', error)
                          })
                        }}
                      >
                        <Copy size={14} />
                      </IconButton>
                    </Tooltip>
                  </Stack>
                  <Button
                    href={loginStatus?.url || CODEX_DEVICE_AUTH_URL}
                    target="_blank"
                    rel="noreferrer"
                    variant="contained"
                    color="secondary"
                    startIcon={<ExternalLink size={16} />}
                    sx={actionSx}
                  >
                    Continue to ChatGPT
                  </Button>
                  {!deviceError && (
                    <Stack direction="row" spacing={1} alignItems="center">
                      <CircularProgress size={16} />
                      <Box>
                        <Typography variant="body2">Waiting for OpenAI callback…</Typography>
                        <Typography variant="caption" color="text.secondary">
                          Keep this dialog open while you finish authorization in ChatGPT.
                        </Typography>
                      </Box>
                    </Stack>
                  )}
                </Stack>
              ) : !deviceError ? (
                <Stack direction="row" alignItems="center" spacing={1} sx={{ minHeight: 48 }}>
                  <CircularProgress size={16} />
                  <Typography variant="body2" color="text.secondary">
                    {startLogin.isPending ? 'Starting secure login…' : 'Waiting for device code…'}
                  </Typography>
                </Stack>
              ) : null}
              {deviceError && <Alert severity="error" sx={{ mt: 1.5 }}>{deviceError}</Alert>}
            </Box>
          )}

          {method === 'success' && (
            <Stack spacing={1.5} alignItems="center" sx={{ py: 2, textAlign: 'center' }}>
              <Box sx={{ color: 'success.main', display: 'flex' }}>
                <CircleCheck size={40} />
              </Box>
              <Box>
                <Typography variant="h6">Codex connected</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  OpenAI authorization completed and the Codex harness is ready to use.
                </Typography>
              </Box>
            </Stack>
          )}

          {method === 'import' && (
            <Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                Run <Box component="code" sx={{ fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' }}>codex login</Box>
                {' '}locally and paste{' '}
                <Box component="code" sx={{ fontFamily: APP_MONO_FONT_FAMILY, fontSize: '0.8125rem' }}>~/.codex/auth.json</Box>
                {' '}below.
              </Typography>
              <Alert severity="warning" sx={{ mb: 1.5 }}>
                This file contains account credentials. Helix encrypts it before storing it and only releases it to your Codex sessions.
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
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, py: 2 }}>
          {method !== 'choose' && method !== 'success' && (
            <Button
              startIcon={<ArrowLeft size={16} />}
              onClick={chooseMethod}
              sx={{ ...actionSx, mr: 'auto' }}
            >
              Back
            </Button>
          )}
          <Button onClick={closeDialog} sx={actionSx}>
            {method === 'success' ? 'Done' : 'Cancel'}
          </Button>
          {method === 'import' && (
            <Button
              variant="contained"
              color="secondary"
              disabled={!value || createSubscription.isPending}
              onClick={connect}
              sx={actionSx}
            >
              Import
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </>
  )
}
