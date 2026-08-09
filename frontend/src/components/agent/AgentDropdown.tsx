import { FC, useMemo } from 'react'
import {
  Box,
  FormControl,
  FormHelperText,
  InputLabel,
  Select,
  MenuItem,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material'
import { SxProps, Theme } from '@mui/material/styles'
import { Pencil } from 'lucide-react'
import { IApp } from '../../types'
import useAccount from '../../hooks/useAccount'
import { selectCodingAgents } from '../../utils/apps'
import AgentHarness, { getAgentHarnessLabel, getAgentHarnessRuntime } from './AgentHarness'

interface AgentDropdownProps {
  /** Currently selected agent ID */
  value: string
  /** Callback when agent selection changes */
  onChange: (agentId: string) => void
  /** Agents to choose from. Non-coding agents are filtered out here, so callers
   * can pass a raw agent list without repeating the kind check. */
  agents: IApp[]
  /** Label for the dropdown */
  label?: string
  /** Whether the dropdown is disabled */
  disabled?: boolean
  /** Size variant */
  size?: 'small' | 'medium'
  /** Helper text displayed below the dropdown */
  helperText?: string
  /** Style overrides for pages with their own palette (e.g. Onboarding) */
  selectSx?: SxProps<Theme>
  labelSx?: SxProps<Theme>
  menuPaperSx?: SxProps<Theme>
}

const agentName = (app: IApp): string => app.config?.helix?.name || 'Unnamed Agent'

/**
 * The single control for picking a coding agent.
 *
 * Owns the three things every caller previously reimplemented: filtering to
 * coding agents, rendering the harness mark (closed state included — a bare
 * name gives no clue which harness an agent runs), and the per-agent edit
 * shortcut. Used by ProjectSettings, CreateProjectDialog, Onboarding and
 * NewSpecTaskForm; add new agent pickers here rather than hand-rolling a Select.
 */
const AgentDropdown: FC<AgentDropdownProps> = ({
  value,
  onChange,
  agents,
  label = 'Agent',
  disabled = false,
  size = 'small',
  helperText,
  selectSx,
  labelSx,
  menuPaperSx,
}) => {
  const account = useAccount()
  const codingAgents = useMemo(() => selectCodingAgents(agents), [agents])

  return (
    <FormControl fullWidth size={size}>
      <InputLabel sx={labelSx}>{label}</InputLabel>
      <Select
        value={value}
        label={label}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        sx={selectSx}
        MenuProps={menuPaperSx ? { PaperProps: { sx: menuPaperSx } } : undefined}
        renderValue={(selectedValue) => {
          const app = codingAgents.find((a) => a.id === selectedValue)
          if (!app) return 'Select Agent'
          return (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
              <AgentHarness runtime={getAgentHarnessRuntime(app)} variant="short" size={18} />
              <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {agentName(app)}
              </Box>
            </Box>
          )
        }}
      >
        {codingAgents.map((app) => {
          const runtime = getAgentHarnessRuntime(app)
          const harnessLabel = getAgentHarnessLabel(runtime)
          const name = agentName(app)
          return (
            <MenuItem key={app.id} value={app.id}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%', minWidth: 0 }}>
                <AgentHarness runtime={runtime} variant="short" size={18} />
                <Box component="span" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {name}
                </Box>
                {/* Agent names are free text, so the harness is not otherwise
                    readable from the row when the name doesn't mention it. */}
                {name.toLowerCase() !== harnessLabel.toLowerCase() && (
                  <Typography component="span" variant="caption" color="text.secondary" noWrap>
                    {harnessLabel}
                  </Typography>
                )}
                <Tooltip title="Edit agent">
                  <IconButton
                    size="small"
                    aria-label={`Edit ${name}`}
                    onClick={(e) => {
                      e.stopPropagation()
                      account.orgNavigate('agent', { app_id: app.id })
                    }}
                  >
                    <Pencil size={18} />
                  </IconButton>
                </Tooltip>
              </Box>
            </MenuItem>
          )
        })}
        {codingAgents.length === 0 && (
          <MenuItem disabled value="">
            No agents available
          </MenuItem>
        )}
      </Select>
      {helperText && <FormHelperText>{helperText}</FormHelperText>}
    </FormControl>
  )
}

export default AgentDropdown
