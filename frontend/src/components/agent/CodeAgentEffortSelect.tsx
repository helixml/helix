import { FC } from 'react'
import Box from '@mui/material/Box'
import FormControl from '@mui/material/FormControl'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Typography from '@mui/material/Typography'

export type CodeAgentEffort = 'default' | 'low' | 'medium' | 'high' | 'xhigh' | 'max' | 'ultra'

type CodeAgentEffortOption = {
  value: CodeAgentEffort
  label: string
  description: string
}

export const CLAUDE_CODE_EFFORT_OPTIONS: ReadonlyArray<CodeAgentEffortOption> = [
  { value: 'default', label: 'Default', description: 'Use the model/runtime default' },
  { value: 'low', label: 'Low', description: 'Faster responses with less reasoning' },
  { value: 'medium', label: 'Medium', description: 'Balanced reasoning effort' },
  { value: 'high', label: 'High', description: 'More thorough reasoning, usually slower' },
  { value: 'xhigh', label: 'Extra High', description: 'Maximum persistent reasoning for supported models' },
  { value: 'max', label: 'Max', description: 'Deepest reasoning for supported models' },
]

export const CODEX_EFFORT_OPTIONS: ReadonlyArray<CodeAgentEffortOption> = [
  { value: 'default', label: 'Default', description: 'Use the model/runtime default' },
  { value: 'low', label: 'Low', description: 'Faster responses with less reasoning' },
  { value: 'medium', label: 'Medium', description: 'Balanced reasoning effort' },
  { value: 'high', label: 'High', description: 'More thorough reasoning, usually slower' },
  { value: 'xhigh', label: 'Extra High', description: 'Extra high reasoning depth for supported models' },
  { value: 'max', label: 'Max', description: 'Maximum reasoning depth for supported models' },
  { value: 'ultra', label: 'Ultra', description: 'Deepest reasoning for supported models' },
]

/**
 * Effort options for a code agent.
 *
 * `runtime` only decides which tiers the *harness* can express. The model
 * decides which of them the provider will actually accept, and the two differ:
 * qwen3.8-27b rejects `high` (the value this list used to offer unconditionally)
 * while accepting `xhigh`. Sending a value the provider rejects is a hard 400
 * that the agent retries and then aborts the turn on, so when the backend tells
 * us what a model supports — `model.reasoning_efforts.supported`, see
 * api/pkg/model/reasoning_efforts.go — narrow to that set.
 *
 * `supportedEfforts` undefined means "Helix has no profile for this model": keep
 * the full runtime list rather than guessing a narrower one, since an empty
 * selector would be worse than an occasionally-wrong option.
 */
export const getCodeAgentEffortOptions = (
  runtime: string,
  supportedEfforts?: readonly string[],
): ReadonlyArray<CodeAgentEffortOption> => {
  const runtimeOptions = runtime === 'claude_code' ? CLAUDE_CODE_EFFORT_OPTIONS : CODEX_EFFORT_OPTIONS
  if (!supportedEfforts || supportedEfforts.length === 0) return runtimeOptions

  const supported = new Set(supportedEfforts.map((effort) => effort.toLowerCase()))
  // 'default' is not a provider value — it means "send nothing" — so it always
  // survives the filter.
  const narrowed = runtimeOptions.filter(
    (option) => option.value === 'default' || supported.has(option.value),
  )
  return narrowed.length > 1 ? narrowed : runtimeOptions
}

export const CodeAgentEffortSelect: FC<{
  options: ReadonlyArray<CodeAgentEffortOption>
  value: string
  onChange: (value: CodeAgentEffort) => void
  disabled?: boolean
}> = ({ options, value, onChange, disabled }) => (
  <Box sx={{ minWidth: 170 }}>
    <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
      Effort
    </Typography>
    <FormControl fullWidth size="small">
      <Select
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value as CodeAgentEffort)}
        renderValue={(selected) => options.find((option) => option.value === selected)?.label ?? selected}
      >
        {options.map((option) => (
          <MenuItem key={option.value} value={option.value}>
            <Box>
              <Typography variant="body2">{option.label}</Typography>
              <Typography variant="caption" color="text.secondary">
                {option.description}
              </Typography>
            </Box>
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  </Box>
)

export default CodeAgentEffortSelect
