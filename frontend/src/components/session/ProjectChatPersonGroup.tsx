import { FC, MouseEvent } from 'react'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import { ChevronDown, ChevronRight } from 'lucide-react'

import type { TypesOrganizationMembership, TypesPinnedChat, TypesProject, TypesUser } from '../../api/api'
import useIsPhone from '../../hooks/useIsPhone'
import useLightTheme from '../../hooks/useLightTheme'
import { useListSessions } from '../../services/sessionService'
import { useSpecTasks } from '../../services/specTaskService'
import { getUserInitials } from '../../utils/user'
import PresenceDot from '../widgets/PresenceDot'
import ProjectChatItemRow from './ProjectChatItemRow'
import ProjectChatShowMore from './ProjectChatShowMore'
import {
  buildPersonChatItems,
  filterProjectChatGroups,
  pinnedAtByItemKeyFrom,
  sidebarMemberMatchesQuery,
} from './ProjectChatSidebar.logic'
import type { SidebarItem, SidebarMember, SidebarThreadSortOrder } from './ProjectChatSidebar.logic'
import { useSidebarItemPagination, windowSidebarItems } from './useSidebarItemPagination'

export const sidebarMemberLabel = (member: SidebarMember): string => {
  const name = member.user.full_name || member.user.username || member.user.email || member.userId
  return member.isViewer ? `${name} (you)` : name
}

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
  pinnedChats?: TypesPinnedChat[]
  archivingItemId: string | null
  onToggle: () => void
  onOpenItem: (item: SidebarItem) => void
  onOpenItemContextMenu: (event: MouseEvent<HTMLElement>, item: SidebarItem) => void
  onArchiveItem: (item: SidebarItem) => void
}

// One org member in the People section: their presence, and when expanded,
// what they are working on across every project the viewer can see. A search
// opens every group so their work can match, and hides the group when neither
// the person nor any of their work does.
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
  pinnedChats = [],
  archivingItemId,
  onToggle,
  onOpenItem,
  onOpenItemContextMenu,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const isPhone = useIsPhone()
  const searching = !!query.trim()
  const open = expanded || searching
  const queriesEnabled = enabled && open && !!orgId
  const label = sidebarMemberLabel(member)
  const pagination = useSidebarItemPagination(visibleThreadCount)

  const sessionsQuery = useListSessions(
    orgId,
    undefined,
    undefined,
    0,
    pagination.requestCount,
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
    limit: pagination.requestCount,
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
  const items = buildPersonChatItems(projects, tasks, sessions, threadSortOrder, pinnedAtByItemKeyFrom(pinnedChats))
  const filteredItems = filterProjectChatGroups([{ id: member.userId, name: label, items }], query)[0]?.items || []
  const renderedItems = windowSidebarItems(filteredItems, activeItemId, pagination.visibleCount)
  const hasMore = filteredItems.length > pagination.visibleCount
    || (sessionsPage?.totalCount || 0) > sessions.length
    || tasks.length >= pagination.requestCount
  const isLoading = queriesEnabled && (sessionsQuery.isLoading || tasksQuery.isLoading)
  const isFetchingMore = sessionsQuery.isFetching || tasksQuery.isFetching
  const hasError = sessionsQuery.isError || tasksQuery.isError

  if (searching && !isLoading && !hasError && filteredItems.length === 0 && !sidebarMemberMatchesQuery(member, query)) {
    return null
  }

  return (
    <Box sx={{ mb: 0.25 }}>
      <Box
        role="button"
        tabIndex={0}
        aria-label={`${open ? 'Hide' : 'Show'} ${label}'s work`}
        aria-expanded={open}
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
            opacity: open ? 1 : 0.7,
          }}
        >
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
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

      {open && (
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
              {searching ? 'No matching work' : 'Nothing you can see yet'}
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
          <ProjectChatShowMore pagination={pagination} hasMore={hasMore} fetching={isFetchingMore} />
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatPersonGroup
