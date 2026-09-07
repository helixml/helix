import { FC, MouseEvent, ReactElement } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Archive, ArchiveRestore, GitBranch, Pin } from 'lucide-react'

import type { TypesOrganizationMembership, TypesUser } from '../../api/api'
import useApps from '../../hooks/useApps'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import AgentHarness from '../agent/AgentHarness'
import OrganizationUserAvatar, { resolveOrganizationUser } from '../widgets/OrganizationUserAvatar'
import ProjectChatItemTooltip from './ProjectChatItemTooltip'
import { activeStatusDotPulse, ProjectRowIcon, StackedTaskStatusIcons, TaskStatusIcons } from './ProjectChatItemBadges'
import { getProjectChatItemDetails, resolveProjectChatItemBranch } from './projectChatItemDetails'
import { compactRelativeTime, getSidebarPullRequestIcon, getSidebarTaskStatus, isActiveSidebarTask } from './ProjectChatSidebar.logic'
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
  /**
   * Set when the row sits in a cross-project list (a person's or bot's work).
   * Switches the row to the stacked layout with the project named above the
   * title, and feeds the tooltip.
   */
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
  // Cross-project lists stack the project name above the title on every
  // device, so the reader knows where each piece of work lives without
  // hovering.
  const stacked = !!projectName
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

  // Active tasks trade their timestamp for the live status label (t3-style):
  // "24 minutes ago" says nothing while the agent is mid-implementation, and
  // once the task goes quiet recency becomes the interesting fact again.
  const showStatusAsTime = stacked && !!status && isActiveSidebarTask(item.task)

  // Time + archive occupy the same slot: the time yields to the archive
  // button on hover. In the stacked layout the slot sits on the project
  // line (t3-style, time top-right); otherwise on the single content line.
  const timeAndArchive = (
    <Box
      sx={isPhone
        // No hover on a phone, so time and archive sit side by side and
        // stay visible — in the stacked layout too.
        ? { display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }
        : stacked
          // Static content sizes the slot so a status label wider than the
          // 28px timestamp column still fits; the archive button overlays it.
          // Without hover the overlay would hide the time forever, so
          // touch devices get the phone treatment: side by side.
          ? {
              minWidth: 28,
              height: 16,
              flexShrink: 0,
              position: 'relative',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'flex-end',
              '@media (hover: none)': { gap: 0.5 },
            }
          : { width: 28, height: 28, flexShrink: 0, position: 'relative' }}
    >
      <Typography
        className="sidebar-item-time"
        component="span"
        title={item.updatedAt ? new Date(item.updatedAt).toLocaleString() : undefined}
        sx={{
          ...(isPhone || stacked
            ? { position: 'static' }
            : { position: 'absolute', inset: 0 }),
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          whiteSpace: 'nowrap',
          color: showStatusAsTime
            ? status!.color
            : active
              ? (lightTheme.isLight ? 'rgba(39,39,42,0.58)' : 'rgba(241,243,247,0.72)')
              : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
          fontSize: '10px',
          lineHeight: 1,
          fontVariantNumeric: 'tabular-nums',
          transition: 'opacity 100ms ease',
          ...(showStatusAsTime && isAgentWorking && {
            animation: `${activeStatusDotPulse} 2s ease-in-out infinite`,
            '@media (prefers-reduced-motion: reduce)': { animation: 'none' },
          }),
        }}
      >
        {showStatusAsTime ? status!.label : compactRelativeTime(item.updatedAt, relativeTimeNow)}
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
              : { position: 'absolute', top: stacked ? -6 : 0, right: 0, bottom: stacked ? -6 : 0, width: 20, height: 28 }),
            ...(!isPhone && stacked && {
              '@media (hover: none)': { position: 'static', top: 'auto', bottom: 'auto', height: 20 },
            }),
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
  )

  const statusIcons = item.kind === 'spec-task' && (
    <TaskStatusIcons
      status={status}
      pullRequestIcon={pullRequestIcon}
      isAgentWorking={isAgentWorking}
    />
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
        fontSize: '14px',
        lineHeight: '20px',
        fontWeight: active ? 500 : 400,
        // The stacked row's hierarchy is carried by contrast: the title
        // reads brighter than the muted project line above it.
        ...(stacked && !active && {
          color: lightTheme.isLight ? '#52525b' : 'rgba(212,212,216,0.88)',
        }),
      }}
    >
      {item.title}
    </Typography>
  )

  const stackedStatusIcons = item.kind === 'spec-task' && (
    <StackedTaskStatusIcons
      status={status}
      pullRequestIcon={pullRequestIcon}
      showStatus={!showStatusAsTime}
      showLabel={isPhone}
    />
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

  const subLineNode = isPhone && subLine.length > 0 && (
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
          height: stacked ? 20 : 28,
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
        // Stacked (cross-project) and phone rows are two lines; project
        // groups on desktop keep the dense single-line row.
        ...(stacked
          ? { py: 0.75, flexDirection: 'column', alignItems: 'stretch', gap: 0.4 }
          : isPhone
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
        // coarse pointer it is always shown. Phone and stacked rows give
        // it its own column (the stacked slot drops its overlay under
        // hover: none), so the time/status stays visible alongside it;
        // only the dense single-line desktop row keeps the swap.
        '@media (hover: none)': {
          '& .sidebar-item-time': { opacity: isPhone || stacked ? 1 : 0 },
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
      {stacked ? (
        <>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.6, minWidth: 0, width: '100%' }}>
            <ProjectRowIcon projectId={item.projectId} />
            <Typography
              component="span"
              sx={{
                minWidth: 0,
                flex: 1,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                fontSize: '11px',
                lineHeight: '14px',
                fontWeight: 400,
                color: active
                  ? (lightTheme.isLight ? 'rgba(39,39,42,0.68)' : 'rgba(241,243,247,0.72)')
                  : (lightTheme.isLight ? 'rgba(113,113,122,0.8)' : 'rgba(200,200,206,0.8)'),
              }}
            >
              {projectName}
            </Typography>
            {timeAndArchive}
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, width: '100%' }}>
            {titleNode}
            {avatarNode}
            {pinNode}
            {stackedStatusIcons}
          </Box>
          {subLineNode}
        </>
      ) : (
        <>
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
        </>
      )}
    </Box>
    </ProjectChatItemTooltip>
  )
}

export default ProjectChatItemRow
