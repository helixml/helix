import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import type { TypesOrganizationMembership, TypesProject, TypesUser } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import ProjectChatPersonGroup from './ProjectChatPersonGroup'
import { visibleSidebarMembers } from './ProjectChatSidebar.logic'
import type { SidebarItem, SidebarMember, SidebarThreadSortOrder } from './ProjectChatSidebar.logic'

type ProjectChatPeopleSectionProps = {
  orgId: string
  members: SidebarMember[]
  selectedUserIds: string[]
  onToggleMember: (userId: string) => void
  projects: TypesProject[]
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
  onOpenItem: (item: SidebarItem) => void
  onOpenItemContextMenu: (event: MouseEvent<HTMLElement>, item: SidebarItem) => void
  onArchiveItem: (item: SidebarItem) => void
}

// Who is in the org and whether they are here right now; expand anyone to see
// what they are working on. Online members always show, offline ones are
// capped behind "Show all" so a big org stays scannable.
const ProjectChatPeopleSection: FC<ProjectChatPeopleSectionProps> = ({
  orgId,
  members,
  selectedUserIds,
  onToggleMember,
  projects,
  query,
  activeItemId,
  relativeTimeNow,
  enabled,
  threadSortOrder,
  visibleThreadCount,
  archived,
  organizationMembers,
  currentUser,
  archivingItemId,
  onOpenItem,
  onOpenItemContextMenu,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const [showAll, setShowAll] = useState(false)
  const selected = new Set(selectedUserIds)
  const visible = visibleSidebarMembers(members, selected, query, showAll)
  const mutedColor = lightTheme.isLight ? 'rgba(113,113,122,0.8)' : 'rgba(163,163,163,0.65)'

  if (members.length === 0) {
    return (
      <Typography sx={{ px: 1.25, py: 0.75, fontSize: '12px', color: mutedColor }}>
        No one else in this organization yet
      </Typography>
    )
  }

  return (
    <Box>
      {visible.members.map((member) => (
        <ProjectChatPersonGroup
          key={member.userId}
          orgId={orgId}
          member={member}
          projects={projects}
          expanded={selected.has(member.userId)}
          query={query}
          activeItemId={activeItemId}
          relativeTimeNow={relativeTimeNow}
          enabled={enabled}
          threadSortOrder={threadSortOrder}
          visibleThreadCount={visibleThreadCount}
          archived={archived}
          organizationMembers={organizationMembers}
          currentUser={currentUser}
          archivingItemId={archivingItemId}
          onToggle={() => onToggleMember(member.userId)}
          onOpenItem={onOpenItem}
          onOpenItemContextMenu={onOpenItemContextMenu}
          onArchiveItem={onArchiveItem}
        />
      ))}
      {query && visible.members.length === 0 && (
        <Typography sx={{ px: 1.25, py: 0.75, fontSize: '12px', color: mutedColor }}>
          No members match
        </Typography>
      )}
      {(visible.hiddenCount > 0 || (showAll && !query)) && (
        <Box
          component="button"
          type="button"
          onClick={() => setShowAll((current) => !current)}
          sx={{
            appearance: 'none',
            border: 0,
            height: 30,
            px: 1,
            backgroundColor: 'transparent',
            color: mutedColor,
            cursor: 'pointer',
            font: 'inherit',
            fontSize: '12px',
            '&:hover': { color: lightTheme.isLight ? '#27272a' : '#f1f3f7' },
          }}
        >
          {showAll ? 'Show fewer' : `Show ${visible.hiddenCount} more offline`}
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatPeopleSection
