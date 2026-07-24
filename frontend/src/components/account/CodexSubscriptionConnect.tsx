import React, { useEffect, useState } from 'react'
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField, Typography } from '@mui/material'
import { TypesCodexAuthCredentials, TypesOwnerType } from '../../api/api'
import { useQueryClient } from '@tanstack/react-query'
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
  const [value, setValue] = useState('')
  const [error, setError] = useState('')
  const [loginSessionId, setLoginSessionId] = useState('')
  const { data: loginStatus } = usePollCodexLogin(loginSessionId)
  const subscription = subscriptions[0]
  const loginFound = loginStatus?.found ?? false

  useEffect(() => {
    if (!loginFound) return
	queryClient.invalidateQueries({ queryKey: codexSubscriptionsQueryKey })
    setLoginSessionId('')
    setOpen(false)
  }, [loginFound])

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
      setOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect Codex subscription')
    }
  }

  if (subscription?.id) {
    return (
      <Button
        color="error"
        variant="outlined"
        disabled={deleteSubscription.isPending}
        onClick={() => deleteSubscription.mutate(subscription.id!)}
      >
        Disconnect
      </Button>
    )
  }

  return (
    <>
      <Button
        variant="outlined"
        disabled={startLogin.isPending}
        onClick={async () => {
          const result = await startLogin.mutateAsync()
          setLoginSessionId(result.session_id || '')
          setOpen(true)
        }}
      >
        Sign in
      </Button>
      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Connect ChatGPT Subscription</DialogTitle>
        <DialogContent>
          {loginSessionId && (
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" sx={{ mb: 1 }}>Complete device authentication in your browser.</Typography>
              {loginStatus?.url && <Button href={loginStatus.url} target="_blank" rel="noreferrer" variant="contained">Open ChatGPT</Button>}
              {loginStatus?.code && <Typography sx={{ mt: 1, fontFamily: 'monospace', fontWeight: 600 }}>{loginStatus.code}</Typography>}
              {loginStatus?.error && <Alert severity="error" sx={{ mt: 1 }}>{loginStatus.error}</Alert>}
            </Box>
          )}
          <Typography variant="body2" sx={{ mb: 2 }}>Alternatively, run <code>codex login</code> locally and paste <code>~/.codex/auth.json</code> below.</Typography>
          <Alert severity="warning" sx={{ mb: 2 }}>
            This file contains account credentials. Helix encrypts it before storing it and only releases it to your desktop sessions.
          </Alert>
          <TextField
            autoFocus
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
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button variant="contained" disabled={!value || createSubscription.isPending} onClick={connect}>Import</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
