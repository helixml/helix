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

export const getCodeAgentEffortOptions = (runtime: string): ReadonlyArray<CodeAgentEffortOption> =>
  runtime === 'claude_code' ? CLAUDE_CODE_EFFORT_OPTIONS : CODEX_EFFORT_OPTIONS

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
