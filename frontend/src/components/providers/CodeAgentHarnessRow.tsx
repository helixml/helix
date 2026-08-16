import { FC, ReactNode, useState } from 'react'
import Box from '@mui/material/Box'
import Collapse from '@mui/material/Collapse'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { ChevronDown } from 'lucide-react'

import AgentHarness, { getAgentHarnessLabel } from '../agent/AgentHarness'

export type HarnessHealth = 'ready' | 'attention' | 'unavailable'

const HEALTH_COLOR: Record<HarnessHealth, string> = {
  ready: 'success.main',
  attention: 'warning.main',
  unavailable: 'action.disabled',
}

const CodeAgentHarnessRow: FC<{
  runtime: string
  health: HarnessHealth
  status: string
  enabled: boolean
  disabled?: boolean
  children: ReactNode
  onToggle: (enabled: boolean) => void
}> = ({ runtime, health, status, enabled, disabled = false, children, onToggle }) => {
  const [expanded, setExpanded] = useState(false)
  const label = getAgentHarnessLabel(runtime)

  return (
    <Box
      sx={{
        py: 1.5,
        borderBottom: '1px solid',
        borderColor: 'divider',
        '&:last-of-type': { borderBottom: 'none' },
      }}
    >
      <Stack direction="row" alignItems="flex-start" spacing={1.5}>
        <Box sx={{ position: 'relative', display: 'flex', flexShrink: 0, mt: 0.25 }}>
          <AgentHarness runtime={runtime} variant="short" size={22} showTooltip={false} />
          <Box
            aria-hidden="true"
            sx={{
              position: 'absolute',
              top: -3,
              left: -4,
              width: 7,
              height: 7,
              borderRadius: '50%',
              bgcolor: HEALTH_COLOR[health],
            }}
          />
        </Box>

        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            {label}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
            {status}
          </Typography>
        </Box>

        <Stack direction="row" alignItems="center" spacing={0.25} sx={{ flexShrink: 0 }}>
          <Tooltip title={expanded ? `Hide ${label} settings` : `Show ${label} settings`}>
            <IconButton
              aria-label={expanded ? `Hide ${label} settings` : `Show ${label} settings`}
              aria-expanded={expanded}
              onClick={() => setExpanded((open) => !open)}
              sx={{
                width: 30,
                height: 30,
                color: 'text.secondary',
                '&:hover': { color: 'text.primary' },
              }}
            >
              <ChevronDown
                size={18}
                style={{
                  transform: expanded ? 'rotate(180deg)' : undefined,
                  transition: 'transform 120ms ease',
                }}
              />
            </IconButton>
          </Tooltip>
          <Switch
            checked={enabled}
            disabled={disabled}
            onChange={(event) => onToggle(event.target.checked)}
            inputProps={{ 'aria-label': `Enable ${label}` }}
          />
        </Stack>
      </Stack>

      <Collapse in={expanded} unmountOnExit>
        <Box sx={{ pl: 4.75, pr: 1, pt: 1.5 }}>{children}</Box>
      </Collapse>
    </Box>
  )
}

export default CodeAgentHarnessRow
