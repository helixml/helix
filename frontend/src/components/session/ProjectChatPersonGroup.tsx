import { FC, MouseEvent, useState } from 'react'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import { ChevronDown, ChevronRight } from 'lucide-react'

import type { TypesOrganizationMembership, TypesProject, TypesUser } from '../../api/api'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import { useListSessions } from '../../services/sessionService'
import { useSpecTasks } from '../../services/specTaskService'
import { getUserInitials } from '../../utils/user'
import PresenceDot from '../widgets/PresenceDot'
import ProjectChatItemRow from './ProjectChatItemRow'
import {
  buildPersonChatItems,
  filterProjectChatGroups,
} from './ProjectChatSidebar.logic'
import type { SidebarItem, SidebarMember, SidebarThreadSortOrder } from './ProjectChatSidebar.logic'

const SHOW_MORE_COUNT = 20

export const sidebarMemberLabel = (member: SidebarMember): string => (
  member.user.full_name || member.user.username || member.user.email || member.userId
)

type ProjectChatPersonGroupProps = {
  orgId: string
  member: SidebarMember
  projects: TypesProject[]
  expanded: boolean
  query: string
  activeItemId: string
  relativeTimeNow: number
  enabled: boolean
  threadSortOrder?: SidebarThreadSortOrder
  visibleThreadCount?: number
  archived?: boolean
  organizationMembers: TypesOrganizationMembership[]
  currentUser?: TypesUser
  archivingItemId: string | null
  onToggle: () => void
  onOpenItem: (item: SidebarItem) => void
  onOpenItemContextMenu: (event: MouseEvent<HTMLElement>, item: SidebarItem) => void
  onArchiveItem: (item: SidebarItem) => void
}

// One org member in the People section: their presence, and when expanded,
// what they are working on across every project the viewer can see.
const ProjectChatPersonGroup: FC<ProjectChatPersonGroupProps> = ({
  orgId,
  member,
  projects,
  expanded,
  query,
  activeItemId,
  relativeTimeNow,
  enabled,
  threadSortOrder = 'updated_at',
  visibleThreadCount = 6,
  archived = false,
  organizationMembers,
  currentUser,
  archivingItemId,
  onToggle,
  onOpenItem,
  onOpenItemContextMenu,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const isPhone = useIsPhone()
  const [additionalVisibleCount, setAdditionalVisibleCount] = useState(0)
  const visibleCount = visibleThreadCount + additionalVisibleCount
  const requestCount = visibleCount + 1
  const queriesEnabled = enabled && expanded && !!orgId

  const sessionsQuery = useListSessions(
    orgId,
    undefined,
    undefined,
    0,
    requestCount,
    {
      enabled: queriesEnabled,
      includeExternalAgents: true,
      sort: threadSortOrder === 'created_at' ? 'created' : 'last_message',
      archived,
      ownerId: member.userId,
    },
  )
  const tasksQuery = useSpecTasks({
    organizationId: orgId,
    limit: requestCount,
    offset: 0,
    sort: threadSortOrder === 'created_at' ? 'created' : 'last_message',
    archivedOnly: archived,
    participantIds: [member.userId],
    enabled: queriesEnabled,
    refetchInterval: archived ? false : 10000,
  })

  const sessionsPage = sessionsQuery.data?.data
  const sessions = sessionsPage?.sessions || []
  const tasks = tasksQuery.data || []
  const items = buildPersonChatItems(projects, tasks, sessions, threadSortOrder)
  const label = sidebarMemberLabel(member)
  const filteredItems = filterProjectChatGroups([{ id: member.userId, name: label, items }], query)[0]?.items || []
  const previewItems = filteredItems.slice(0, visibleCount)
  const activeHiddenItem = filteredItems.slice(visibleCount).find((item) => item.id === activeItemId)
  const renderedItems = activeHiddenItem ? [...previewItems, activeHiddenItem] : previewItems
  const sessionsHaveMore = (sessionsPage?.totalCount || 0) > sessions.length
  const tasksMayHaveMore = tasks.length === requestCount
  const hasMore = filteredItems.length > visibleCount || sessionsHaveMore || tasksMayHaveMore
  const canShowLess = additionalVisibleCount > 0
  const isLoading = queriesEnabled && (sessionsQuery.isLoading || tasksQuery.isLoading)
  const isFetchingMore = sessionsQuery.isFetching || tasksQuery.isFetching
  const hasError = sessionsQuery.isError || tasksQuery.isError
  const paginationButtonSx = {
    appearance: 'none',
    border: 0,
    height: 30,
    px: 1,
    backgroundColor: 'transparent',
    color: lightTheme.isLight ? 'rgba(113,113,122,0.75)' : 'rgba(163,163,163,0.75)',
    cursor: isFetchingMore ? 'default' : 'pointer',
    font: 'inherit',
    fontSize: '12px',
    '&:hover': {
      color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
      backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
    },
  }

  return (
    <Box sx={{ mb: 0.25 }}>
      <Box
        role="button"
        tabIndex={0}
        aria-label={`${expanded ? 'Hide' : 'Show'} ${label}'s work`}
        aria-expanded={expanded}
        data-member-id={member.userId}
        onClick={onToggle}
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onToggle()
          }
        }}
        sx={{
          width: '100%',
          height: 32,
          px: 0.75,
          display: 'flex',
          alignItems: 'center',
          gap: 0.65,
          borderRadius: '6px',
          color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
          cursor: 'pointer',
          outline: 'none',
          '&:hover, &:focus-visible': {
            backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
          },
        }}
      >
        <Box
          component="span"
          sx={{
            width: isPhone ? 28 : 16,
            height: isPhone ? 28 : 16,
            flexShrink: 0,
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            opacity: expanded ? 1 : 0.7,
          }}
        >
          {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </Box>
        <Box sx={{ position: 'relative', width: 18, height: 18, flexShrink: 0 }}>
          <Avatar sx={{ width: 18, height: 18, fontSize: '0.55rem' }}>
            {getUserInitials(member.user)}
          </Avatar>
          <Box sx={{ position: 'absolute', right: -2, bottom: -2, display: 'inline-flex' }}>
            <PresenceDot
              online={member.online}
              size={7}
              ringColor={lightTheme.isLight ? '#fafafa' : '#000000'}
            />
          </Box>
        </Box>
        <Typography
          component="span"
          sx={{
            minWidth: 0,
            flex: 1,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontFamily: 'inherit',
            fontSize: '14px',
            lineHeight: '20px',
            fontWeight: 500,
          }}
        >
          {label}
        </Typography>
        {isLoading && <CircularProgress size={11} color="inherit" />}
      </Box>

      {expanded && (
        <Box sx={{ pl: 1.15 }}>
          {hasError && (
            <Typography color="error" sx={{ px: 1, py: 0.75, fontSize: '0.7rem' }}>
              Failed to load their work
            </Typography>
          )}
          {!isLoading && !hasError && filteredItems.length === 0 && (
            <Typography
              sx={{
                px: 1,
                py: 0.75,
                fontSize: '12px',
                color: lightTheme.isLight ? 'rgba(113,113,122,0.8)' : 'rgba(163,163,163,0.65)',
              }}
            >
              {query ? 'No matching work' : 'Nothing you can see yet'}
            </Typography>
          )}
          {renderedItems.map((item) => (
            <ProjectChatItemRow
              key={`${item.kind}:${item.id}`}
              item={item}
              active={item.id === activeItemId}
              relativeTimeNow={relativeTimeNow}
              archived={archived}
              archivingItemId={archivingItemId}
              organizationMembers={organizationMembers}
              currentUser={currentUser}
              projectName={item.projectName}
              onOpenItem={onOpenItem}
              onOpenItemContextMenu={onOpenItemContextMenu}
              onArchiveItem={onArchiveItem}
            />
          ))}
          {(canShowLess || hasMore) && (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
              {canShowLess && (
                <Box
                  component="button"
                  type="button"
                  disabled={isFetchingMore}
                  onClick={() => setAdditionalVisibleCount(0)}
                  sx={paginationButtonSx}
                >
                  Show less
                </Box>
              )}
              {hasMore && (
                <Box
                  component="button"
                  type="button"
                  disabled={isFetchingMore}
                  onClick={() => setAdditionalVisibleCount((count) => count + SHOW_MORE_COUNT)}
                  sx={paginationButtonSx}
                >
                  {isFetchingMore ? 'Loading…' : 'Show more'}
                </Box>
              )}
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatPersonGroup
