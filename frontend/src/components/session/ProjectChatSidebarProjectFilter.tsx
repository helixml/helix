import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import { Check, ChevronDown } from 'lucide-react'

import type { TypesProject } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { ALL_PROJECTS_FILTER } from './ProjectChatSidebar.logic'

type ProjectChatSidebarProjectFilterProps = {
  projects: TypesProject[]
  selectedProjectId: string
  archived: boolean
  onChange: (projectId: string) => void
}

const ProjectChatSidebarProjectFilter: FC<ProjectChatSidebarProjectFilterProps> = ({
  projects,
  selectedProjectId,
  archived,
  onChange,
}) => {
  const lightTheme = useLightTheme()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const selectedProject = projects.find((project) => project.id === selectedProjectId)
  const selectedLabel = selectedProject?.name || 'All projects'
  const label = archived ? `Archived · ${selectedLabel}` : selectedLabel

  const selectProject = (projectId: string) => {
    setAnchorEl(null)
    onChange(projectId)
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
          color: lightTheme.isLight ? 'rgba(113,113,122,0.80)' : 'rgba(163,163,163,0.80)',
          fontFamily: 'inherit',
          fontSize: '12px',
          fontWeight: 500,
          lineHeight: 1,
          textTransform: 'none',
          '& .MuiButton-endIcon': { ml: 0.5, flexShrink: 0 },
          '&:hover': {
            color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
            backgroundColor: lightTheme.isLight ? 'rgba(39,39,42,0.04)' : 'rgba(241,243,247,0.08)',
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
              maxHeight: 360,
              backgroundImage: 'none',
            },
          },
        }}
      >
        <MenuItem
          selected={selectedProjectId === ALL_PROJECTS_FILTER}
          onClick={() => selectProject(ALL_PROJECTS_FILTER)}
          sx={{ gap: 1, fontSize: '13px' }}
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
              sx={{ gap: 1, fontSize: '13px' }}
            >
              <Box sx={{ width: 16, display: 'inline-flex' }}>
                {selectedProjectId === project.id && <Check size={14} />}
              </Box>
              <Typography
                component="span"
                sx={{ overflow: 'hidden', textOverflow: 'ellipsis', fontSize: '13px' }}
              >
                {project.name || 'Untitled project'}
              </Typography>
            </MenuItem>
          )] : [])}
      </Menu>
    </>
  )
}

export default ProjectChatSidebarProjectFilter
