import { FC, useState } from 'react'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Divider from '@mui/material/Divider'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import { Check, ChevronDown, Users } from 'lucide-react'

import type { TypesProject } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { getUserInitials } from '../../utils/user'
import { getSidebarColors } from '../../styles/themeTokens'
import { TYPOGRAPHY } from '../../styles/typography'
import { ALL_PROJECTS_FILTER, ALL_USERS_FILTER } from './ProjectChatSidebar.logic'
import type { SidebarMember } from './ProjectChatSidebar.logic'
import { sidebarMemberLabel } from './ProjectChatPersonGroup'

type ProjectChatSidebarProjectFilterProps = {
  projects: TypesProject[]
  selectedProjectId: string
  archived: boolean
  /** Org members the view can be filtered to; empty until members load. */
  members: SidebarMember[]
  selectedUserId: string
  /** Whether the user section is offered at all (off when grouping by person). */
  showUserFilter: boolean
  onChange: (projectId: string) => void
  onUserChange: (userId: string) => void
}

// The toolbar's project picker. The menu holds both dimensions of the
// projects-and-tasks view: which project (or all of them), and below a
// divider, whose work — everyone's, or one member's.
const ProjectChatSidebarProjectFilter: FC<ProjectChatSidebarProjectFilterProps> = ({
  projects,
  selectedProjectId,
  archived,
  members,
  selectedUserId,
  showUserFilter,
  onChange,
  onUserChange,
}) => {
  const lightTheme = useLightTheme()
  const sidebarColors = getSidebarColors(lightTheme.isLight)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const selectedProject = projects.find((project) => project.id === selectedProjectId)
  // When the user section is not offered the stored choice must not leak into
  // the label or the menu — the caller keeps it, but it is not in effect here.
  const effectiveUserId = showUserFilter ? selectedUserId : ALL_USERS_FILTER
  const selectedMember = members.find((member) => member.userId === effectiveUserId)
  const projectLabel = selectedProject?.name || 'All projects'
  const selectedLabel = selectedMember
    ? `${projectLabel} · ${sidebarMemberLabel(selectedMember)}`
    : projectLabel
  const label = (archived ? `Archived · ${selectedLabel}` : selectedLabel)
  const sectionLabelSx = {
    px: 2,
    pt: 0.75,
    pb: 0.4,
    color: sidebarColors.subtleForeground,
    fontSize: TYPOGRAPHY.sidebar.sectionFontSize,
    fontWeight: 600,
    letterSpacing: '0.08em',
    textTransform: 'uppercase',
    lineHeight: 1.4,
  }

  const selectProject = (projectId: string) => {
    setAnchorEl(null)
    onChange(projectId)
  }

  const selectUser = (userId: string) => {
    setAnchorEl(null)
    onUserChange(userId)
  }

  return (
    <>
      <Button
        variant="text"
        size="small"
        aria-label="Filter tasks by project"
        aria-haspopup="menu"
        aria-expanded={!!anchorEl || undefined}
        onClick={(event) => setAnchorEl(event.currentTarget)}
        endIcon={<ChevronDown size={13} strokeWidth={1.7} />}
        sx={{
          flex: 1,
          minWidth: 0,
          height: 30,
          px: 0.75,
          justifyContent: 'space-between',
          color: sidebarColors.mutedForeground,
          fontFamily: 'inherit',
          fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
          fontWeight: 500,
          lineHeight: 1,
          textTransform: 'none',
          '& .MuiButton-endIcon': { ml: 0.5, flexShrink: 0 },
          '&:hover': {
            color: sidebarColors.foreground,
            backgroundColor: sidebarColors.rowHover,
          },
        }}
      >
        <Box
          component="span"
          sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        >
          {label}
        </Box>
      </Button>
      <Menu
        anchorEl={anchorEl}
        open={!!anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        slotProps={{
          paper: {
            sx: {
              mt: 0.5,
              minWidth: 220,
              maxWidth: 300,
              maxHeight: 420,
              backgroundImage: 'none',
            },
          },
        }}
      >
        <MenuItem
          selected={selectedProjectId === ALL_PROJECTS_FILTER}
          onClick={() => selectProject(ALL_PROJECTS_FILTER)}
          sx={{ gap: 1, fontSize: TYPOGRAPHY.sidebar.controlFontSize }}
        >
          <Box sx={{ width: 16, display: 'inline-flex' }}>
            {selectedProjectId === ALL_PROJECTS_FILTER && <Check size={14} />}
          </Box>
          All projects
        </MenuItem>
        {[...projects]
          .sort((left, right) => (left.name || '').localeCompare(right.name || ''))
          .flatMap((project) => project.id ? [(
            <MenuItem
              key={project.id}
              selected={selectedProjectId === project.id}
              onClick={() => selectProject(project.id!)}
              sx={{ gap: 1, fontSize: TYPOGRAPHY.sidebar.controlFontSize }}
            >
              <Box sx={{ width: 16, display: 'inline-flex' }}>
                {selectedProjectId === project.id && <Check size={14} />}
              </Box>
              <Typography
                component="span"
                sx={{
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  fontSize: TYPOGRAPHY.sidebar.controlFontSize,
                }}
              >
                {project.name || 'Untitled project'}
              </Typography>
            </MenuItem>
          )] : [])}
        {showUserFilter && (
          <>
            <Divider sx={{ my: 0.5 }} />
            <Typography aria-hidden="true" sx={sectionLabelSx}>
              Filter by user
            </Typography>
            <MenuItem
              selected={effectiveUserId === ALL_USERS_FILTER}
              onClick={() => selectUser(ALL_USERS_FILTER)}
              sx={{ gap: 1, fontSize: TYPOGRAPHY.sidebar.controlFontSize }}
            >
              <Box sx={{ width: 18, display: 'inline-flex', justifyContent: 'center' }}>
                {effectiveUserId === ALL_USERS_FILTER ? <Check size={14} /> : <Users size={14} />}
              </Box>
              Everyone
            </MenuItem>
            {members.map((member) => (
              <MenuItem
                key={member.userId}
                selected={effectiveUserId === member.userId}
                onClick={() => selectUser(member.userId)}
                sx={{ gap: 1, fontSize: TYPOGRAPHY.sidebar.controlFontSize }}
              >
                <Box sx={{ width: 18, display: 'inline-flex', justifyContent: 'center' }}>
                  {effectiveUserId === member.userId ? <Check size={14} /> : (
                    <Avatar sx={{ width: 16, height: 16, fontSize: '0.5rem' }}>
                      {getUserInitials(member.user)}
                    </Avatar>
                  )}
                </Box>
                <Typography
                  component="span"
                  sx={{
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    fontSize: TYPOGRAPHY.sidebar.controlFontSize,
                  }}
                >
                  {sidebarMemberLabel(member)}
                </Typography>
              </MenuItem>
            ))}
          </>
        )}
      </Menu>
    </>
  )
}

export default ProjectChatSidebarProjectFilter
