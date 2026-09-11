import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import type { TypesOrganizationMembership, TypesPinnedChat, TypesProject, TypesUser } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { getSidebarColors } from '../../styles/themeTokens'
import { TYPOGRAPHY } from '../../styles/typography'
import ProjectChatPersonGroup from './ProjectChatPersonGroup'
import { visibleSidebarMembers } from './ProjectChatSidebar.logic'
import type { SidebarItem, SidebarMember, SidebarThreadSortOrder } from './ProjectChatSidebar.logic'

type ProjectChatPeopleSectionProps = {
  orgId: string
  members: SidebarMember[]
  selectedUserIds: string[]
  onToggleMember: (userId: string) => void
  projects: TypesProject[]
  /** Set in focus mode: show only each person's work in this project. */
  projectId?: string
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
  projectId,
  query,
  activeItemId,
  relativeTimeNow,
  enabled,
  threadSortOrder,
  visibleThreadCount,
  archived,
  organizationMembers,
  currentUser,
  pinnedChats,
  archivingItemId,
  onOpenItem,
  onOpenItemContextMenu,
  onArchiveItem,
}) => {
  const lightTheme = useLightTheme()
  const sidebarColors = getSidebarColors(lightTheme.isLight)
  const [showAll, setShowAll] = useState(false)
  const selected = new Set(selectedUserIds)
  const visible = visibleSidebarMembers(members, selected, query, showAll)

  if (members.length === 0) {
    return (
      <Typography sx={{ px: 1.25, py: 0.75, fontSize: TYPOGRAPHY.sidebar.metadataFontSize, color: sidebarColors.subtleForeground }}>
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
          projectId={projectId}
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
          pinnedChats={pinnedChats}
          archivingItemId={archivingItemId}
          onToggle={() => onToggleMember(member.userId)}
          onOpenItem={onOpenItem}
          onOpenItemContextMenu={onOpenItemContextMenu}
          onArchiveItem={onArchiveItem}
        />
      ))}
      {query && visible.members.length === 0 && (
        <Typography sx={{ px: 1.25, py: 0.75, fontSize: TYPOGRAPHY.sidebar.metadataFontSize, color: sidebarColors.subtleForeground }}>
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
            color: sidebarColors.subtleForeground,
            cursor: 'pointer',
            font: 'inherit',
            fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
            '&:hover': { color: sidebarColors.foreground },
          }}
        >
          {showAll ? 'Show fewer' : `Show ${visible.hiddenCount} more offline`}
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatPeopleSection
