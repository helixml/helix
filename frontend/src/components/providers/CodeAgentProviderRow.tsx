import { FC, ReactNode, useState } from 'react'
import Box from '@mui/material/Box'
import Collapse from '@mui/material/Collapse'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'
import { ChevronDown, Trash2 } from 'lucide-react'

import AgentHarness, { getAgentHarnessLabel } from '../agent/AgentHarness'

export type ProviderHealth = 'ready' | 'attention' | 'unavailable'

const HEALTH_COLOR: Record<ProviderHealth, string> = {
  ready: 'success.main',
  attention: 'warning.main',
  unavailable: 'error.main',
}

/**
 * One coding-agent provider in the settings list: status dot, harness mark and
 * name, a one-line status, an expand chevron, and the enable switch.
 *
 * The switch is the only always-visible control by design — enabling is the
 * common action, and configuration only matters once a provider is on.
 */
const CodeAgentProviderRow: FC<{
  runtime: string
  health: ProviderHealth
  status: string
  enabled: boolean
  disabled?: boolean
  badge?: ReactNode
  children?: ReactNode
  label?: string
  // Set for flavour rows, which the org added and can remove. Built-in harness
  // rows have no delete: the list must stay complete so an owner can always
  // find and enable a supported agent.
  onDelete?: () => void
  onToggle: (enabled: boolean) => void
}> = ({ runtime, health, status, enabled, disabled = false, badge, children, label: labelOverride, onDelete, onToggle }) => {
  const [expanded, setExpanded] = useState(false)
  const label = labelOverride || getAgentHarnessLabel(runtime)
  const hasDetail = !!children

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
          <Stack direction="row" alignItems="center" spacing={1}>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {label}
            </Typography>
            {badge}
          </Stack>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
            {status}
          </Typography>
        </Box>

        <Stack direction="row" alignItems="center" spacing={0.25} sx={{ flexShrink: 0 }}>
          {onDelete && (
            <IconButton
              aria-label={`Remove ${label}`}
              onClick={onDelete}
              sx={{ width: 30, height: 30, color: 'text.secondary', '&:hover': { color: 'error.main' } }}
            >
              <Trash2 size={16} />
            </IconButton>
          )}
          <IconButton
            aria-label={expanded ? `Hide ${label} settings` : `Show ${label} settings`}
            aria-expanded={expanded}
            onClick={() => setExpanded((open) => !open)}
            disabled={!hasDetail}
            sx={{
              width: 30,
              height: 30,
              color: 'text.secondary',
              visibility: hasDetail ? 'visible' : 'hidden',
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
          <Switch
            checked={enabled}
            disabled={disabled}
            onChange={(event) => onToggle(event.target.checked)}
            inputProps={{ 'aria-label': `Enable ${label}` }}
          />
        </Stack>
      </Stack>

      {hasDetail && (
        <Collapse in={expanded} unmountOnExit>
          <Box sx={{ pl: 4.75, pr: 1, pt: 1.5 }}>{children}</Box>
        </Collapse>
      )}
    </Box>
  )
}

export default CodeAgentProviderRow
