import { FC, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import FormControl from '@mui/material/FormControl'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { TypesCodeAgentRuntime } from '../../api/api'
import AgentHarness, { getAgentHarnessLabel } from '../agent/AgentHarness'

/**
 * Adds a named flavour of an existing harness — the same agent pointed at a
 * different provider or model, e.g. one opencode on qwen and another on
 * deepseek. The name is what distinguishes it, so it is required and must be
 * unique within the harness.
 */
const AddCodeAgentFlavourDialog: FC<{
  open: boolean
  runtimes: string[]
  existingNames: (runtime: string) => string[]
  onClose: () => void
  onAdd: (runtime: string, name: string) => void
}> = ({ open, runtimes, existingNames, onClose, onAdd }) => {
  const [runtime, setRuntime] = useState<string>(runtimes[0] || TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode)
  const [name, setName] = useState('')

  const trimmed = name.trim()
  const taken = existingNames(runtime).some((existing) => existing.toLowerCase() === trimmed.toLowerCase())
  const error = trimmed && taken ? `${getAgentHarnessLabel(runtime)} already has a flavour called "${trimmed}"` : ''

  const submit = () => {
    if (!trimmed || taken) return
    onAdd(runtime, trimmed)
    setName('')
    onClose()
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>Add a coding agent flavour</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 0.5 }}>
          <Typography variant="body2" color="text.secondary">
            A flavour is another configuration of a harness you already have — same
            agent, different provider or model. It appears as its own row and its own
            option when starting a task.
          </Typography>

          <FormControl fullWidth size="small">
            <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 0.75 }}>
              Harness
            </Typography>
            <Select
              value={runtime}
              onChange={(event) => setRuntime(event.target.value)}
              renderValue={(value) => <AgentHarness runtime={value} variant="long" size={16} showTooltip={false} />}
            >
              {runtimes.map((option) => (
                <MenuItem key={option} value={option}>
                  <AgentHarness runtime={option} variant="long" size={16} showTooltip={false} />
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <div>
            <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 0.75 }}>
              Name
            </Typography>
            <TextField
              autoFocus
              fullWidth
              size="small"
              value={name}
              error={!!error}
              helperText={error || 'For example "qwen" or "deepseek"'}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => { if (event.key === 'Enter') submit() }}
              inputProps={{ 'aria-label': 'Flavour name' }}
            />
          </div>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" disabled={!trimmed || taken} onClick={submit}>
          Add
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default AddCodeAgentFlavourDialog
