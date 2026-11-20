import { FC } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { keyframes } from '@mui/material/styles'
import { GitPullRequest } from 'lucide-react'

import { TYPOGRAPHY } from '../../styles/typography'
import type { SidebarActivity, SidebarPullRequestIcon, SidebarStatus } from './ProjectChatSidebar.logic'

export const activeStatusDotPulse = keyframes`
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
`

export const ActivityStatusDot: FC<{
  activity: SidebarActivity | null
  restartRequired?: boolean
}> = ({ activity, restartRequired = false }) => activity ? (
  <Tooltip title={restartRequired ? `${activity.label} - restart required to apply changes` : activity.label}>
    <Box
      component="span"
      role="status"
      aria-label={activity.label}
      onMouseOver={(event) => event.stopPropagation()}
      sx={{
        width: 5,
        height: 5,
        borderRadius: '50%',
        flexShrink: 0,
        backgroundColor: activity.color,
        boxShadow: restartRequired ? '0 0 0 2px rgba(251,191,36,0.55)' : 'none',
        animation: activity.working
          ? `${activeStatusDotPulse} 2s ease-in-out infinite`
          : 'none',
        '@media (prefers-reduced-motion: reduce)': {
          animation: 'none',
        },
      }}
    />
  </Tooltip>
) : null

// Workflow badges for a task row in the dense single-line layout: the PR
// glyph is always present (grey placeholder before a PR exists) and the
// activity dot and workflow status stay independent.
export const TaskStatusIcons: FC<{
  status: SidebarStatus | null
  pullRequestIcon?: SidebarPullRequestIcon
  activity: SidebarActivity | null
}> = ({ status, pullRequestIcon, activity }) => (
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
    <Box
      onMouseOver={(event) => event.stopPropagation()}
      sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.45 }}
    >
      <ActivityStatusDot activity={activity} />
      {status && (
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
      )}
    </Box>
  </Box>
)
