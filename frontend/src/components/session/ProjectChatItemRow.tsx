import { FC, MouseEvent } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Archive, ArchiveRestore, GitBranch, Pin, Trash2 } from 'lucide-react'

import type { TypesOrganizationMembership, TypesUser } from '../../api/api'
import useApps from '../../hooks/useApps'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import { getSidebarColors } from '../../styles/themeTokens'
import { TYPOGRAPHY } from '../../styles/typography'
import AgentHarness from '../agent/AgentHarness'
import OrganizationUserAvatar, { resolveOrganizationUser } from '../widgets/OrganizationUserAvatar'
import { PRESENCE_OFFLINE_COLOR, PRESENCE_ONLINE_COLOR } from '../widgets/PresenceDot'
import { deriveSandboxState } from '../external-agent/sandboxState'
import ProjectChatItemTooltip from './ProjectChatItemTooltip'
import { activeStatusDotPulse, TaskStatusIcons } from './ProjectChatItemBadges'
import { getProjectChatItemDetails, resolveProjectChatItemBranch } from './projectChatItemDetails'
import { compactRelativeTime, getSidebarPullRequestIcon, getSidebarTaskStatus } from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'

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
  /** Set when the row sits in a cross-project list (a person's or bot's work); shown in the tooltip. */
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
  const sidebarColors = getSidebarColors(lightTheme.isLight)
  const { apps } = useApps()
  // No hover on a phone, so the facts the tooltip carries have to live on the
  // row itself. That makes the row two lines, and taller.
  const isPhone = useIsPhone()
  const isBotInstance = !!item.botInstanceOf
  const archiveVerb = archived ? 'Unarchive' : 'Archive'
  const status = item.kind === 'spec-task' ? getSidebarTaskStatus(item.task) : null
  const isAgentWorking = item.kind === 'spec-task' && item.task?.agent_work_state === 'working'
  const pullRequestIcon = item.kind === 'spec-task'
    ? getSidebarPullRequestIcon(item.task)
    : undefined
  const isArchiving = archivingItemId === item.id
  const taskPersonId = item.task?.assignee_id || item.task?.created_by || item.session?.owner || ''
  const taskPerson = resolveOrganizationUser(taskPersonId, organizationMembers, currentUser)
  const taskPersonRole = item.kind === 'session' ? 'Started by' : item.task?.assignee_id ? 'Assigned to' : 'Created by'
  const branch = resolveProjectChatItemBranch(item, defaultBranch)
  // Phone rows expose stable context without requiring hover; desktop rows
  // keep the details in the tooltip.
  const details = isPhone
    ? getProjectChatItemDetails({ item, apps, repository: repositoryName, branch })
    : undefined
  const branchUnavailable = item.kind === 'spec-task' && !details?.branch
  const branchLabel = details?.branch || (branchUnavailable ? 'n/a' : undefined)
  const hasSubLineDetails = !!(branchLabel || details?.harness)

  // Time + archive occupy the same slot: the time yields to the archive
  // button on hover.
  const timeAndArchive = (
    <Box
      sx={isPhone
        // No hover on a phone, so time and archive sit side by side and
        // stay visible.
        ? { display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }
        : { width: 32, height: 28, flexShrink: 0, position: 'relative' }}
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
          whiteSpace: 'nowrap',
          color: sidebarColors.mutedForeground,
          fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
          lineHeight: TYPOGRAPHY.sidebar.metadataLineHeight,
          fontVariantNumeric: 'tabular-nums',
          transition: 'opacity 100ms ease',
        }}
      >
        {compactRelativeTime(item.updatedAt, relativeTimeNow)}
      </Typography>
      <Tooltip title={isBotInstance ? 'Delete instance' : `${archiveVerb} ${item.kind === 'spec-task' ? 'task' : 'chat'}`}>
        <IconButton
          className="sidebar-item-archive"
          size="small"
          disabled={isArchiving}
          aria-label={isBotInstance
            ? `Delete instance ${item.title}`
            : `${archiveVerb} ${item.kind === 'spec-task' ? 'task' : 'chat'} ${item.title}`}
          onMouseOver={(event) => event.stopPropagation()}
          onClick={(event) => {
            event.stopPropagation()
            onArchiveItem(item)
          }}
          sx={{
            ...(isPhone
              ? { position: 'static', width: 28, height: 28 }
              : { position: 'absolute', top: 0, right: 0, bottom: 0, width: 24, height: 28 }),
            opacity: 0,
            color: 'inherit',
            transition: 'opacity 100ms ease',
          }}
        >
          {isArchiving
            ? <CircularProgress size={12} color="inherit" />
            : isBotInstance ? <Trash2 size={14} /> : archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
        </IconButton>
      </Tooltip>
    </Box>
  )

  const instanceSandboxState = isBotInstance ? deriveSandboxState(item.session?.metadata).sandboxState : undefined
  const instanceStatusTitle = instanceSandboxState === 'running'
    ? 'Sandbox running'
    : instanceSandboxState === 'starting' ? 'Sandbox starting' : 'Sandbox stopped'
  const statusIcons = item.kind === 'spec-task' ? (
    <TaskStatusIcons
      status={status}
      pullRequestIcon={pullRequestIcon}
      isAgentWorking={isAgentWorking}
    />
  ) : isBotInstance && (
    <Tooltip title={instanceStatusTitle}>
      <Box
        component="span"
        data-instance-status={instanceSandboxState}
        onMouseOver={(event) => event.stopPropagation()}
        sx={{ width: 14, display: 'inline-flex', justifyContent: 'center', flexShrink: 0 }}
      >
        <Box
          component="span"
          sx={{
            width: 7,
            height: 7,
            borderRadius: '50%',
            backgroundColor: instanceSandboxState === 'absent' ? PRESENCE_OFFLINE_COLOR : PRESENCE_ONLINE_COLOR,
            ...(instanceSandboxState === 'starting' && {
              animation: `${activeStatusDotPulse} 2s ease-in-out infinite`,
              '@media (prefers-reduced-motion: reduce)': { animation: 'none' },
            }),
          }}
        />
      </Box>
    </Tooltip>
  )

  const titleNode = (
    <Typography
      component="span"
      sx={{
        minWidth: 0,
        flex: 1,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        color: active ? sidebarColors.foreground : sidebarColors.primaryLabel,
        fontSize: TYPOGRAPHY.sidebar.primaryFontSize,
        lineHeight: TYPOGRAPHY.sidebar.primaryLineHeight,
        fontWeight: 500,
      }}
    >
      {item.title}
    </Typography>
  )

  const avatarNode = showTaskAvatars && !!taskPersonId && (
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
  )

  const branchMetadataNode = branchLabel && (
    <Box
      data-testid="sidebar-item-branch"
      data-branch-state={branchUnavailable ? 'unavailable' : 'available'}
      onMouseOver={(event) => event.stopPropagation()}
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 0.5,
        minWidth: 0,
        flex: 1,
      }}
    >
      <Box sx={{ display: 'inline-flex', flexShrink: 0 }}>
        <GitBranch size={11} />
      </Box>
      <Typography
        component="span"
        sx={{
          minWidth: 0,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
          lineHeight: TYPOGRAPHY.sidebar.metadataLineHeight,
        }}
      >
        {branchLabel}
      </Typography>
    </Box>
  )

  const branchNode = branchUnavailable
    ? (
        <Tooltip title="Task is in planning mode; no branch yet." placement="bottom-start">
          {branchMetadataNode}
        </Tooltip>
      )
    : branchMetadataNode

  const subLineNode = isPhone && hasSubLineDetails && (
    <Box
      data-testid="sidebar-item-metadata"
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        minWidth: 0,
        width: '100%',
        color: sidebarColors.subtleForeground,
      }}
    >
      {branchNode}
      {details?.harness && (
        <Box
          data-testid="sidebar-item-harness"
          sx={{ ml: 'auto', display: 'inline-flex', alignItems: 'center', flexShrink: 0 }}
        >
          <AgentHarness runtime={details.runtime || ''} variant="short" size={11} />
        </Box>
      )}
    </Box>
  )

  const pinNode = item.pinnedAt && (
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
            ? sidebarColors.foreground
            : sidebarColors.subtleForeground,
          transition: 'opacity 100ms ease',
        }}
      >
        <Pin size={11} fill="currentColor" strokeWidth={1.7} />
      </Box>
    </Tooltip>
  )

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
      aria-current={active ? 'page' : undefined}
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
        ...(isPhone
          ? { minHeight: 52, py: 0.75, flexDirection: 'column', alignItems: 'stretch', gap: 0.25 }
          : { height: 32, flexDirection: 'row', alignItems: 'center', gap: 0.75 }),
        px: 1,
        borderRadius: '6px',
        display: 'flex',
        color: active ? sidebarColors.foreground : sidebarColors.primaryLabel,
        backgroundColor: active ? sidebarColors.rowSelected : 'transparent',
        boxShadow: active ? `inset 0 0 0 1px ${sidebarColors.border}` : 'none',
        cursor: 'pointer',
        textAlign: 'left',
        outline: 'none',
        position: 'relative',
        '&:hover, &:focus-visible': {
          color: sidebarColors.foreground,
          backgroundColor: active ? sidebarColors.rowSelected : sidebarColors.rowHover,
        },
        '&:hover .sidebar-item-time, &:focus-within .sidebar-item-time': { opacity: 0 },
        '&:hover .sidebar-item-archive, &:focus-within .sidebar-item-archive': { opacity: 1 },
        // Without hover the archive button can never appear, so on a
        // coarse pointer it is always shown. Phone rows give it its own
        // column, so the time stays visible alongside it; the single-line
        // desktop row keeps the swap.
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
        {statusIcons}
        {titleNode}
        {avatarNode}
        {pinNode}
        {timeAndArchive}
      </Box>
      {subLineNode}
    </Box>
    </ProjectChatItemTooltip>
  )
}

export default ProjectChatItemRow
