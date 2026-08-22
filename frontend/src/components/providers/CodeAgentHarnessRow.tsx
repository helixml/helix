import { FC, ReactNode, useState } from 'react'
import Box from '@mui/material/Box'
import Collapse from '@mui/material/Collapse'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'
import { ChevronDown } from 'lucide-react'

import AgentHarness, { getAgentHarnessLabel } from '../agent/AgentHarness'

export type HarnessHealth = 'ready' | 'attention' | 'unavailable'

export const HARNESS_SWITCH_SLOT_SX = {
  width: 58,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'flex-end',
  flexShrink: 0,
} as const

const HEALTH_COLOR: Record<HarnessHealth, string> = {
  ready: 'success.main',
  attention: 'warning.main',
  unavailable: 'action.disabled',
}

const CodeAgentHarnessRow: FC<{
  runtime: string
  health: HarnessHealth
  // A node, not a string: the identity line embeds a click-to-reveal email.
  status: ReactNode
  enabled: boolean
  disabled?: boolean
  children: ReactNode
  // Omitted in account scope: enabling a harness is an org-level decision, but
  // the row keeps the switch's width as an empty slot so both surfaces align.
  onToggle?: (enabled: boolean) => void
}> = ({ runtime, health, status, enabled, disabled = false, children, onToggle }) => {
  const [expanded, setExpanded] = useState(false)
  const label = getAgentHarnessLabel(runtime)
  const expandLabel = expanded ? `Hide ${label} settings` : `Show ${label} settings`

  return (
    <Box
      sx={{
        py: 1.5,
        borderBottom: '1px solid',
        borderColor: 'divider',
        '&:last-of-type': { borderBottom: 'none' },
      }}
    >
      <Stack direction="row" alignItems="center" spacing={0.25}>
        <Box
          component="button"
          type="button"
          aria-expanded={expanded}
          aria-label={expandLabel}
          onClick={() => setExpanded((open) => !open)}
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1.5,
            flex: 1,
            minWidth: 0,
            m: 0,
            p: 0,
            border: 0,
            background: 'transparent',
            color: 'inherit',
            cursor: 'pointer',
            textAlign: 'left',
            borderRadius: 1,
            '&:hover .harness-expand-chevron': { color: 'text.primary' },
            '&:focus-visible': {
              outline: '2px solid',
              outlineColor: 'secondary.main',
              outlineOffset: 2,
            },
          }}
        >
          <Box component="span" sx={{ position: 'relative', display: 'flex', flexShrink: 0 }}>
            <AgentHarness runtime={runtime} variant="short" size={22} showTooltip={false} />
            <Box
              component="span"
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

          <Box component="span" sx={{ minWidth: 0, flex: 1 }}>
            <Typography component="span" variant="body2" sx={{ display: 'block', fontWeight: 600 }}>
              {label}
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
              {status}
            </Typography>
          </Box>

          <Box
            component="span"
            className="harness-expand-chevron"
            aria-hidden="true"
            sx={{
              width: 30,
              height: 30,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
              color: 'text.secondary',
            }}
          >
            <ChevronDown
              size={18}
              style={{
                transform: expanded ? 'rotate(180deg)' : undefined,
                transition: 'transform 120ms ease',
              }}
            />
          </Box>
        </Box>

        <Box
          sx={HARNESS_SWITCH_SLOT_SX}
          onClick={(event) => event.stopPropagation()}
        >
          {onToggle && (
            <Switch
              color="secondary"
              checked={enabled}
              disabled={disabled}
              onChange={(event) => onToggle(event.target.checked)}
              inputProps={{ 'aria-label': `Enable ${label}` }}
            />
          )}
        </Box>
      </Stack>

      <Collapse in={expanded} unmountOnExit>
        <Box sx={{ pl: 4.75, pt: 1.5 }}>{children}</Box>
      </Collapse>
    </Box>
  )
}

export default CodeAgentHarnessRow
