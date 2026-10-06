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
import { Bot, Pencil, TriangleAlert } from 'lucide-react'
import { IApp } from '../../types'
import useAccount from '../../hooks/useAccount'
import { isHelixAgent, selectCodingAgents } from '../../utils/apps'
import AgentHarness, { getAgentHarnessLabel, getAgentHarnessRuntime } from './AgentHarness'

interface AgentDropdownProps {
  /** Currently selected agent ID */
  value: string
  /** Callback when agent selection changes */
  onChange: (agentId: string) => void
  /** Agents to choose from. Whichever kind `kind` names is selected here, so
   * callers can pass a raw agent list without repeating the check. */
  agents: IApp[]
  /**
   * Which agents this picker is for.
   *
   * `coding` runs work in a sandbox (spec tasks, task defaults). `helix` is a
   * conversational agent driven by a plain inference session — what the project
   * manager and pull request reviewer roles run on.
   */
  kind?: 'coding' | 'helix'

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
  kind = 'coding',
  label,
  disabled = false,
  size = 'small',
  helperText,
  selectSx,
  labelSx,
  menuPaperSx,
}) => {
  const account = useAccount()
  const selectableAgents = useMemo(
    () => (kind === 'helix' ? agents.filter(isHelixAgent) : selectCodingAgents(agents)),
    [agents, kind],
  )
  // A Helix agent has no harness, and getAgentHarnessRuntime falls back to
  // zed_agent when none is set — so showing the mark here would label every
  // conversational agent "Zed Agent".
  const showHarness = kind === 'coding'

  return (
    <FormControl fullWidth size={size}>
      {label && <InputLabel sx={labelSx}>{label}</InputLabel>}
      <Select
        value={value}
        label={label || undefined}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        sx={selectSx}
        MenuProps={menuPaperSx ? { PaperProps: { sx: menuPaperSx } } : undefined}
        // Without this MUI skips renderValue entirely for an empty value, so an
        // unset picker collapses to a blank box while a set-but-unresolvable one
        // shows the placeholder — two different-looking flavours of "nothing".
        displayEmpty
        renderValue={(selectedValue) => {
          if (!selectedValue) {
            return <Box sx={{ color: 'text.secondary' }}>Select Agent</Box>
          }
          const app = selectableAgents.find((a) => a.id === selectedValue)
          // A stored id that isn't in the list (agent deleted, or of the wrong
          // kind) is a real misconfiguration — say so rather than rendering the
          // placeholder, which would read as "nothing is configured".
          if (!app) {
            return (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0, color: 'warning.main' }}>
                <TriangleAlert size={16} />
                <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  Unavailable agent
                </Box>
              </Box>
            )
          }
          return (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
              {showHarness
                ? <AgentHarness runtime={getAgentHarnessRuntime(app)} variant="short" size={18} />
                : <Bot size={18} />}
              <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {agentName(app)}
              </Box>
            </Box>
          )
        }}
      >
        {selectableAgents.map((app) => {
          const runtime = getAgentHarnessRuntime(app)
          const harnessLabel = getAgentHarnessLabel(runtime)
          const name = agentName(app)
          return (
            <MenuItem key={app.id} value={app.id}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%', minWidth: 0 }}>
                {showHarness
                  ? <AgentHarness runtime={runtime} variant="short" size={18} />
                  : <Bot size={18} />}
                <Box component="span" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {name}
                </Box>
                {/* Agent names are free text, so the harness is not otherwise
                    readable from the row when the name doesn't mention it. */}
                {showHarness && name.toLowerCase() !== harnessLabel.toLowerCase() && (
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
        {selectableAgents.length === 0 && (
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
