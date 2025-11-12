import React, { FC } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  CardActions,
  Grid,
  Typography,
  IconButton,
  Alert,
  TextField,
  InputAdornment,
  Pagination,
} from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SettingsIcon from '@mui/icons-material/Settings'
import SearchIcon from '@mui/icons-material/Search'
import { Kanban } from 'lucide-react'

import CreateProjectButton from './CreateProjectButton'
import { TypesProject } from '../../services'
import type { ServerSampleProject } from '../../api/api'

interface ProjectsListViewProps {
  projects: TypesProject[]
  error: Error | null
  searchQuery: string
  onSearchChange: (query: string) => void
  page: number
  onPageChange: (page: number) => void
  filteredProjects: TypesProject[]
  paginatedProjects: TypesProject[]
  totalPages: number
  onViewProject: (project: TypesProject) => void
  onMenuOpen: (event: React.MouseEvent<HTMLElement>, project: TypesProject) => void
  onNavigateToSettings: (projectId: string) => void
  onCreateEmpty: () => void
  onCreateFromSample: (sampleId: string, sampleName: string) => Promise<void>
  sampleProjects: ServerSampleProject[]
  isCreating: boolean
}

const ProjectsListView: FC<ProjectsListViewProps> = ({
  projects,
  error,
  searchQuery,
  onSearchChange,
  page,
  onPageChange,
  filteredProjects,
  paginatedProjects,
  totalPages,
  onViewProject,
  onMenuOpen,
  onNavigateToSettings,
  onCreateEmpty,
  onCreateFromSample,
  sampleProjects,
  isCreating,
}) => {
  return (
    <>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error instanceof Error ? error.message : 'Failed to load projects'}
        </Alert>
      )}

      {/* Search bar */}
      {projects.length > 0 && (
        <Box sx={{ mb: 3 }}>
          <TextField
            placeholder="Search projects..."
            size="small"
            value={searchQuery}
            onChange={(e) => {
              onSearchChange(e.target.value)
              onPageChange(0) // Reset to first page on search
            }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon />
                </InputAdornment>
              ),
            }}
            sx={{ maxWidth: 400 }}
          />
          {searchQuery && (
            <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
              {filteredProjects.length} of {projects.length} projects
            </Typography>
          )}
        </Box>
      )}

      {filteredProjects.length === 0 && searchQuery ? (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No projects found
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Try adjusting your search query
          </Typography>
          <Button
            variant="outlined"
            onClick={() => onSearchChange('')}
          >
            Clear Search
          </Button>
        </Box>
      ) : projects.length === 0 ? (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <Box sx={{ color: 'text.disabled', mb: 2 }}>
            <Kanban size={80} />
          </Box>
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No projects yet
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Create your first project to get started
          </Typography>
          <CreateProjectButton
            onCreateEmpty={onCreateEmpty}
            onCreateFromSample={onCreateFromSample}
            sampleProjects={sampleProjects}
            isCreating={isCreating}
            variant="contained"
            color="primary"
          />
        </Box>
      ) : (
        <>
        <Grid container spacing={3}>
          {paginatedProjects.map((project) => (
            <Grid item xs={12} sm={6} md={4} key={project.id}>
              <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                <CardContent sx={{ flexGrow: 1, cursor: 'pointer' }} onClick={() => onViewProject(project)}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                    <Kanban size={40} style={{ color: '#1976d2' }} />
                    <IconButton
                      size="small"
                      onClick={(e) => {
                        e.stopPropagation()
                        onMenuOpen(e, project)
                      }}
                    >
                      <MoreVertIcon />
                    </IconButton>
                  </Box>
                  <Typography variant="h6" gutterBottom>
                    {project.name}
                  </Typography>
                  {project.description && (
                    <Typography variant="body2" color="text.secondary" sx={{
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      display: '-webkit-box',
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: 'vertical',
                    }}>
                      {project.description}
                    </Typography>
                  )}
                </CardContent>
                <CardActions>
                  <Button size="small" onClick={() => onViewProject(project)}>
                    Open
                  </Button>
                  <Button
                    size="small"
                    startIcon={<SettingsIcon />}
                    onClick={(e) => {
                      e.stopPropagation()
                      onNavigateToSettings(project.id)
                    }}
                  >
                    Settings
                  </Button>
                </CardActions>
              </Card>
            </Grid>
          ))}
        </Grid>

        {/* Pagination */}
        {totalPages > 1 && (
          <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
            <Pagination
              count={totalPages}
              page={page + 1}
              onChange={(_, newPage) => onPageChange(newPage - 1)}
              color="primary"
              showFirstButton
              showLastButton
            />
          </Box>
        )}
        </>
      )}
    </>
  )
}

export default ProjectsListView
