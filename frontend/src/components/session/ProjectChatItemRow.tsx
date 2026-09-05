import { FC, MouseEvent, ReactElement } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { keyframes } from '@mui/material/styles'
import { Archive, ArchiveRestore, GitBranch, GitPullRequest, Pin } from 'lucide-react'

import type { TypesOrganizationMembership, TypesUser } from '../../api/api'
import useApps from '../../hooks/useApps'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import AgentHarness from '../agent/AgentHarness'
import OrganizationUserAvatar, { resolveOrganizationUser } from '../widgets/OrganizationUserAvatar'
import ProjectChatItemTooltip from './ProjectChatItemTooltip'
import { getProjectChatItemDetails, resolveProjectChatItemBranch } from './projectChatItemDetails'
import { compactRelativeTime, getSidebarPullRequestIcon, getSidebarTaskStatus } from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'

const activeStatusDotPulse = keyframes`
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
`

export type ProjectChatItemRowProps = {
  item: SidebarItem
  active: boolean
  relativeTimeNow: number
  archived?: boolean
  archivingItemId: string | null
  organizationMembers: TypesOrganizationMembership[]
  currentUser?: TypesUser
  showTaskAvatars?: boolean
  repositoryName?: string
  defaultBranch?: string
  /** Shown in the tooltip when the row sits in a cross-project list. */
  projectName?: string
  onOpenItem: (item: SidebarItem) => void
  onOpenItemContextMenu: (event: MouseEvent<HTMLElement>, item: SidebarItem) => void
  onArchiveItem: (item: SidebarItem) => void
}

// One chat or task row of the sidebar. Shared by the project groups and the
// per-person groups so both surfaces render, hover, and archive identically.
const ProjectChatItemRow: FC<ProjectChatItemRowProps> = ({
  item,
  active,
  relativeTimeNow,
  archived = false,
  archivingItemId,
  organizationMembers,
  currentUser,
  showTaskAvatars = false,
  repositoryName,
  defaultBranch,
  projectName,
  onOpenItem,
  onOpenItemContextMenu,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const { apps } = useApps()
  // No hover on a phone, so the facts the tooltip carries have to live on the
  // row itself. That makes the row two lines, and taller.
  const isPhone = useIsPhone()
  const archiveVerb = archived ? 'Unarchive' : 'Archive'
  const status = item.kind === 'spec-task' ? getSidebarTaskStatus(item.task) : null
  const isAgentWorking = item.kind === 'spec-task' && item.task?.agent_work_state === 'working'
  const pullRequestIcon = item.kind === 'spec-task'
    ? getSidebarPullRequestIcon(item.task)
    : undefined
  const isArchiving = archivingItemId === item.id
  const taskPersonId = item.task?.assignee_id || item.task?.created_by || ''
  const taskPerson = resolveOrganizationUser(taskPersonId, organizationMembers, currentUser)
  const taskPersonRole = item.task?.assignee_id ? 'Assigned to' : 'Created by'
  const branch = resolveProjectChatItemBranch(item, defaultBranch)
  // Only resolved for the phone layout — on desktop the tooltip does
  // its own lookup when it actually opens.
  const details = isPhone
    ? getProjectChatItemDetails({ item, apps, repository: repositoryName, branch })
    : undefined
  const subLine = details
    ? [
        details.branch && { key: 'branch', icon: <GitBranch size={11} />, value: details.branch },
        // Icon only — the mark identifies the harness, the name just
        // ate horizontal space on a phone.
        details.harness && {
          key: 'harness',
          icon: <AgentHarness runtime={details.runtime || ''} variant="short" size={11} />,
        },
      ].filter(Boolean) as Array<{ key: string; icon: ReactElement; value?: string }>
    : []
  return (
    <ProjectChatItemTooltip
      item={item}
      repository={repositoryName}
      projectName={projectName}
      branch={branch}
      disabled={isPhone}
    >
    <Box
      className="project-chat-item"
      role="button"
      tabIndex={0}
      onClick={() => onOpenItem(item)}
      onContextMenu={(event) => onOpenItemContextMenu(event, item)}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onOpenItem(item)
        }
      }}
      sx={{
        width: '100%',
        minWidth: 0,
        // Two lines and a 48px touch target on a phone; the dense
        // single-line row everywhere else.
        ...(isPhone
          ? { minHeight: 52, py: 0.75, flexDirection: 'column', alignItems: 'stretch', gap: 0.25 }
          : { height: 32, flexDirection: 'row', alignItems: 'center', gap: 0.75 }),
        px: 1,
        borderRadius: '6px',
        display: 'flex',
        color: active
          ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
          : (lightTheme.isLight ? '#71717a' : 'rgba(163,163,163,0.80)'),
        backgroundColor: active
          ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
          : 'transparent',
        cursor: 'pointer',
        textAlign: 'left',
        outline: 'none',
        position: 'relative',
        '&:hover, &:focus-visible': {
          color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
          backgroundColor: active
            ? (lightTheme.isLight ? '#ffffff' : 'rgba(241,243,247,0.11)')
            : (lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)'),
        },
        '&:hover .sidebar-item-time, &:focus-within .sidebar-item-time': { opacity: 0 },
        '&:hover .sidebar-item-archive, &:focus-within .sidebar-item-archive': { opacity: 1 },
        // Without hover the archive button can never appear, so on a
        // coarse pointer it is always shown. On a phone it gets its
        // own column instead (below), so the time stays visible too.
        '@media (hover: none)': {
          '& .sidebar-item-time': { opacity: isPhone ? 1 : 0 },
          '& .sidebar-item-archive': { opacity: 1 },
        },
        ...(isPhone && {
          // Rows are tall enough on a phone that adjacent ones read as
          // one block without a divider between them.
          borderBottom: '1px solid',
          borderColor: lightTheme.isLight
            ? 'rgba(0,0,0,0.06)'
            : 'rgba(241,243,247,0.07)',
          borderRadius: 0,
          '&:last-of-type': { borderBottom: 'none' },
        }),
      }}
    >
      <Box
        sx={isPhone
          ? { display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, width: '100%' }
          : { display: 'contents' }}
      >
      {item.kind === 'spec-task' && (
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
                <Typography component="span" sx={{ fontSize: '0.66rem', color: status.color, lineHeight: 1 }}>
                  {status.label}
                </Typography>
              </Box>
            </Tooltip>
          )}
        </Box>
      )}
      <Typography
        component="span"
        sx={{
          minWidth: 0,
          flex: 1,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          fontSize: '14px',
          lineHeight: '20px',
          fontWeight: active ? 500 : 400,
        }}
      >
        {item.title}
      </Typography>
      {showTaskAvatars && item.kind === 'spec-task' && (
        <Tooltip title={`${taskPersonRole} ${taskPerson?.full_name || taskPerson?.username || taskPerson?.email || 'unknown user'}`}>
          <Box sx={{ width: 18, height: 18, flexShrink: 0, display: 'inline-flex' }}>
            <OrganizationUserAvatar
              userId={taskPersonId}
              members={organizationMembers}
              currentUser={currentUser}
              size={18}
              fontSize="0.55rem"
              iconSize={16}
            />
          </Box>
        </Tooltip>
      )}
      {item.pinnedAt && (
        <Tooltip title="Pinned">
          <Box
            component="span"
            className="sidebar-item-pin"
            aria-label="Pinned"
            onMouseOver={(event) => event.stopPropagation()}
            sx={{
              width: 14,
              height: 28,
              flexShrink: 0,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: active
                ? (lightTheme.isLight ? 'rgba(39,39,42,0.68)' : 'rgba(241,243,247,0.78)')
                : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.62)'),
              transition: 'opacity 100ms ease',
            }}
          >
            <Pin size={11} fill="currentColor" strokeWidth={1.7} />
          </Box>
        </Tooltip>
      )}
      <Box
        sx={isPhone
          ? { display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }
          : { width: 28, height: 28, flexShrink: 0, position: 'relative' }}
      >
        <Typography
          className="sidebar-item-time"
          component="span"
          title={item.updatedAt ? new Date(item.updatedAt).toLocaleString() : undefined}
          sx={{
            ...(isPhone
              ? { position: 'static' }
              : { position: 'absolute', inset: 0 }),
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            color: active
              ? (lightTheme.isLight ? 'rgba(39,39,42,0.58)' : 'rgba(241,243,247,0.72)')
              : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
            fontSize: '10px',
            lineHeight: 1,
            fontVariantNumeric: 'tabular-nums',
            transition: 'opacity 100ms ease',
          }}
        >
          {compactRelativeTime(item.updatedAt, relativeTimeNow)}
        </Typography>
        <Tooltip title={`${archiveVerb} ${item.kind === 'spec-task' ? 'task' : 'chat'}`}>
          <IconButton
            className="sidebar-item-archive"
            size="small"
            disabled={isArchiving}
            aria-label={`${archiveVerb} ${item.kind === 'spec-task' ? 'task' : 'chat'} ${item.title}`}
            onMouseOver={(event) => event.stopPropagation()}
            onClick={(event) => {
              event.stopPropagation()
              onArchiveItem(item)
            }}
            sx={{
              ...(isPhone
                ? { position: 'static', width: 28, height: 28 }
                : { position: 'absolute', top: 0, right: 0, bottom: 0, width: 20, height: 28 }),
              opacity: 0,
              color: 'inherit',
              transition: 'opacity 100ms ease',
            }}
          >
            {isArchiving
              ? <CircularProgress size={12} color="inherit" />
              : archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
          </IconButton>
        </Tooltip>
      </Box>
      </Box>
      {isPhone && subLine.length > 0 && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1.25,
            minWidth: 0,
            color: lightTheme.isLight
              ? 'rgba(113,113,122,0.85)'
              : 'rgba(163,163,163,0.72)',
          }}
        >
          {subLine.map((entry) => (
            <Box
              key={entry.key}
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.5,
                minWidth: 0,
                // The branch takes the slack; the other entries are
                // icon-sized and should stay whole.
                flexShrink: entry.key === 'branch' ? 1 : 0,
              }}
            >
              <Box sx={{ display: 'inline-flex', flexShrink: 0 }}>{entry.icon}</Box>
              {entry.value && (
                <Typography
                  component="span"
                  sx={{
                    minWidth: 0,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    fontSize: '11px',
                    lineHeight: '15px',
                  }}
                >
                  {entry.value}
                </Typography>
              )}
            </Box>
          ))}
        </Box>
      )}
    </Box>
    </ProjectChatItemTooltip>
  )
}

export default ProjectChatItemRow
