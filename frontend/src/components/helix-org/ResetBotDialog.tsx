// Confirmation for the destructive Reset. What each scope deletes and keeps
// comes from the server (GET /bots/{id}/reset) so this dialog cannot drift
// from the real contract. The operator must pick a scope and type the bot's
// name before the Reset button enables.

import { FC, useEffect, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import FormControlLabel from '@mui/material/FormControlLabel'
import Radio from '@mui/material/Radio'
import RadioGroup from '@mui/material/RadioGroup'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { BotResetScope, useBotResetPlan } from '../../services/helixOrgService'

const SCOPE_LABELS: Record<BotResetScope, string> = {
  session: 'Reset session',
  session_and_home: 'Reset session and bot home',
}

export interface ResetBotDialogProps {
  open: boolean
  botId: string
  // The name the operator must type; the bot id when it has no name.
  botName: string
  onClose: () => void
  onReset: (scope: BotResetScope) => Promise<unknown>
}

const ResetBotDialog: FC<ResetBotDialogProps> = ({ open, botId, botName, onClose, onReset }) => {
  const { data: plan, isLoading, error: planError } = useBotResetPlan(botId, { enabled: open })
  const [scope, setScope] = useState<BotResetScope>('session')
  const [typed, setTyped] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setTyped('')
    setError('')
    setScope('session')
  }, [open])

  const scopes = plan?.scopes ?? []
  const selected = scopes.find((s) => s.scope === scope)
  // A session-only reset is unavailable while the bot home still lives in
  // the session; offer the broader scope instead of silently widening.
  useEffect(() => {
    if (selected && !selected.available) {
      const fallback = scopes.find((s) => s.available)
      if (fallback) setScope(fallback.scope as BotResetScope)
    }
  }, [selected, scopes])

  const nameMatches = typed === botName
  const canSubmit = !!selected?.available && nameMatches && !submitting

  const submit = async () => {
    if (!canSubmit) return
    setSubmitting(true)
    setError('')
    try {
      await onReset(scope)
      onClose()
    } catch (err: any) {
      // A failed or partial teardown is reported verbatim: the server says
      // what may still exist (for example a home that could not be erased).
      setError(err?.response?.data?.error ?? err?.message ?? 'Reset failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onClose={submitting ? undefined : onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Reset org bot</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          Reset deliberately starts <b style={{ fontFamily: 'monospace' }}>{botId}</b> fresh.
          Use Restart instead to recover the bot without losing anything.
        </Typography>

        {isLoading && <CircularProgress size={20} />}
        {planError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            Could not load what a reset would delete. Reset is unavailable.
          </Alert>
        )}

        {scopes.length > 0 && (
          <RadioGroup
            value={scope}
            onChange={(e) => setScope(e.target.value as BotResetScope)}
            sx={{ mb: 1 }}
          >
            {scopes.map((s) => (
              <Box key={s.scope}>
                <FormControlLabel
                  value={s.scope}
                  disabled={!s.available || submitting}
                  control={<Radio size="small" />}
                  label={SCOPE_LABELS[s.scope as BotResetScope] ?? s.scope}
                  data-testid={`reset-scope-${s.scope}`}
                />
                {!s.available && s.unavailable_reason && (
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', ml: 4 }}>
                    {s.unavailable_reason}
                  </Typography>
                )}
              </Box>
            ))}
          </RadioGroup>
        )}

        {selected && (
          <Box data-testid="reset-declaration" sx={{ mb: 2 }}>
            <Typography variant="subtitle2" color="error">Will be deleted</Typography>
            <Box component="ul" sx={{ mt: 0.5, mb: 1.5, pl: 3 }}>
              {(selected.erases ?? []).map((item) => (
                <li key={item}><Typography variant="body2">{item}</Typography></li>
              ))}
            </Box>
            <Typography variant="subtitle2">Will be kept</Typography>
            <Box component="ul" sx={{ mt: 0.5, mb: 0, pl: 3 }}>
              {(selected.keeps ?? []).map((item) => (
                <li key={item}><Typography variant="body2">{item}</Typography></li>
              ))}
            </Box>
          </Box>
        )}

        <Typography variant="body2" sx={{ mb: 1 }}>
          Type <b style={{ fontFamily: 'monospace' }}>{botName}</b> to confirm.
        </Typography>
        <TextField
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          size="small"
          fullWidth
          autoComplete="off"
          disabled={submitting || !selected}
          inputProps={{ 'aria-label': 'Bot name confirmation', 'data-testid': 'reset-confirm-input' }}
        />

        {error && (
          <Alert severity="error" sx={{ mt: 2 }} data-testid="reset-error">
            {error}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={submitting}>Cancel</Button>
        <Button
          variant="contained"
          color="error"
          onClick={() => { void submit() }}
          disabled={!canSubmit}
          data-testid="reset-confirm"
        >
          {submitting ? 'Resetting…' : 'Reset bot'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default ResetBotDialog
