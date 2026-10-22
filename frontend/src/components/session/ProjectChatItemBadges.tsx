import { FC } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { keyframes } from '@mui/material/styles'
import { GitPullRequest } from 'lucide-react'

import { TYPOGRAPHY } from '../../styles/typography'
import type { SidebarPullRequestIcon, SidebarStatus } from './ProjectChatSidebar.logic'

export const activeStatusDotPulse = keyframes`
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
`

// Workflow badges for a task row in the dense single-line layout: the PR
// glyph is always present (grey placeholder before a PR exists) and the
// status renders as dot + label.
export const TaskStatusIcons: FC<{
  status: SidebarStatus | null
  pullRequestIcon?: SidebarPullRequestIcon
  isAgentWorking: boolean
}> = ({ status, pullRequestIcon, isAgentWorking }) => (
  <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, flexShrink: 0 }}>
    <Tooltip title={pullRequestIcon?.tooltip || ''}>
      <Box
        component="a"
        href={pullRequestIcon?.url}
        target={pullRequestIcon?.url ? '_blank' : undefined}
        rel={pullRequestIcon?.url ? 'noopener noreferrer' : undefined}
        aria-label={pullRequestIcon?.tooltip}
        onMouseOver={(event) => event.stopPropagation()}
        onClick={(event) => {
          event.stopPropagation()
          if (!pullRequestIcon?.url) event.preventDefault()
        }}
        sx={{
          display: 'inline-flex',
          color: pullRequestIcon?.color || 'currentColor',
          cursor: pullRequestIcon?.url ? 'pointer' : 'default',
        }}
      >
        <GitPullRequest size={13} />
      </Box>
    </Tooltip>
    {status && (
      <Tooltip title={status.tooltip || ''} disableHoverListener={!status.tooltip}>
        <Box
          onMouseOver={(event) => event.stopPropagation()}
          sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.45 }}
        >
          <Box
            sx={{
              width: 5,
              height: 5,
              borderRadius: '50%',
              backgroundColor: status.color,
              animation: isAgentWorking
                ? `${activeStatusDotPulse} 2s ease-in-out infinite`
                : 'none',
              '@media (prefers-reduced-motion: reduce)': {
                animation: 'none',
              },
            }}
          />
          <Typography
            component="span"
            sx={{
              fontSize: TYPOGRAPHY.sidebar.statusFontSize,
              color: status.color,
              lineHeight: TYPOGRAPHY.sidebar.statusLineHeight,
            }}
          >
            {status.label}
          </Typography>
        </Box>
      </Tooltip>
    )}
  </Box>
)
