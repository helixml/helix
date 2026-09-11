import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { keyframes } from '@mui/material/styles'
import { Folder, GitPullRequest } from 'lucide-react'

import { useGetProjectRepositories } from '../../services/projectService'
import { TYPOGRAPHY } from '../../styles/typography'
import { githubOrgAvatarUrl } from './ProjectChatSidebar.logic'
import type { SidebarPullRequestIcon, SidebarStatus } from './ProjectChatSidebar.logic'

export const activeStatusDotPulse = keyframes`
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
`

// Project glyph for the stacked (cross-project) row: the GitHub owner avatar
// when the project's repo lives on github.com, the generic folder otherwise —
// including when the avatar can't load (air-gapped deployments never reach
// github.com, so the broken-image state must degrade to the folder).
export const ProjectRowIcon: FC<{ projectId?: string }> = ({ projectId }) => {
  const repositoriesQuery = useGetProjectRepositories(projectId || '', !!projectId)
  // Track which URL failed rather than a boolean so a URL change (repos
  // resolving late, project reassignment) gets a fresh attempt.
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const avatarUrl = githubOrgAvatarUrl(repositoriesQuery.data || [])
  if (avatarUrl && avatarUrl !== failedUrl) {
    return (
      <Box
        component="img"
        key={avatarUrl}
        src={avatarUrl}
        alt=""
        onError={() => setFailedUrl(avatarUrl)}
        sx={{ width: 16, height: 16, borderRadius: '3px', flexShrink: 0, display: 'block' }}
      />
    )
  }
  return <Folder size={14} style={{ opacity: 0.72, flexShrink: 0 }} />
}

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

// The stacked row keeps its title line clean: the workflow status collapses
// to a colored dot at the right edge (label in the tooltip), and the PR icon
// only appears once a pull request actually exists — the grey "no PR yet"
// placeholder is noise repeated on every row.
export const StackedTaskStatusIcons: FC<{
  status: SidebarStatus | null
  pullRequestIcon?: SidebarPullRequestIcon
  /** False while the time slot carries the status label instead. */
  showStatus: boolean
  /** No hover on a phone, so the label the tooltip would carry has to live on the row itself. */
  showLabel: boolean
}> = ({ status, pullRequestIcon, showStatus, showLabel }) => (
  <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, flexShrink: 0 }}>
    {pullRequestIcon?.url && (
      <Tooltip title={pullRequestIcon.tooltip}>
        <Box
          component="a"
          href={pullRequestIcon.url}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={pullRequestIcon.tooltip}
          onMouseOver={(event) => event.stopPropagation()}
          onClick={(event) => event.stopPropagation()}
          sx={{ display: 'inline-flex', color: pullRequestIcon.color, cursor: 'pointer' }}
        >
          <GitPullRequest size={12} />
        </Box>
      </Tooltip>
    )}
    {status && showStatus && (
      <Tooltip title={status.tooltip || status.label}>
        <Box
          onMouseOver={(event) => event.stopPropagation()}
          sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.45, flexShrink: 0 }}
        >
          <Box
            sx={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              flexShrink: 0,
              backgroundColor: status.color,
            }}
          />
          {showLabel && (
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
      </Tooltip>
    )}
  </Box>
)
