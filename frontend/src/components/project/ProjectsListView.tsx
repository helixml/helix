import React, { FC, useMemo } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  Grid,
  Typography,
  IconButton,
  Alert,
  TextField,
  InputAdornment,
  Pagination,
  Skeleton,
} from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import SearchIcon from '@mui/icons-material/Search'
import TrendingUpIcon from '@mui/icons-material/TrendingUp'
import TrendingDownIcon from '@mui/icons-material/TrendingDown'
import { Kanban } from 'lucide-react'

import CreateProjectButton from './CreateProjectButton'
import { TypesProject } from '../../services'
import type { ServerSampleProject } from '../../api/api'
import { useGetProjectUsage } from '../../services/projectService'

interface ProjectsListViewProps {
  projects: TypesProject[]
  error: Error | null
  isLoading: boolean
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
  appNamesMap?: Record<string, string>
}

const MiniSparkline: FC<{ data: number[]; color: string }> = ({ data, color }) => {
  if (!data || data.length === 0) {
    return (
      <Box sx={{ height: 32, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Typography variant="caption" sx={{ color: 'text.disabled', fontSize: '0.65rem' }}>
          No usage data
        </Typography>
      </Box>
    )
  }

  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const width = 100
  const height = 32
  const padding = 2

  const points = data.map((value, index) => {
    const x = padding + (index / (data.length - 1 || 1)) * (width - padding * 2)
    const y = height - padding - ((value - min) / range) * (height - padding * 2)
    return `${x},${y}`
  }).join(' ')

  const areaPoints = `${padding},${height - padding} ${points} ${width - padding},${height - padding}`

  return (
    <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      <defs>
        <linearGradient id={`gradient-${color.replace('#', '')}`} x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" stopColor={color} stopOpacity="0.3" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon
        points={areaPoints}
        fill={`url(#gradient-${color.replace('#', '')})`}
      />
      <polyline
        points={points}
        fill="none"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

const StatRow: FC<{
  label: string
  value: string | number
}> = ({ label, value }) => (
  <Box sx={{ 
    display: 'flex', 
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: 0.25,
    minWidth: 0,
  }}>
    <Typography variant="caption" sx={{ 
      color: 'text.secondary',
      fontSize: '0.65rem',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      width: '100%',
    }}>
      {label}
    </Typography>
    <Typography variant="body2" sx={{ 
      fontWeight: 600, 
      color: 'text.primary',
      fontSize: '0.8rem',
      fontFamily: 'monospace',
    }}>
      {value}
    </Typography>
  </Box>
)

const formatDuration = (hours: number): string => {
  if (hours === 0) return '-'
  if (hours < 1) return `${Math.round(hours * 60)}m`
  if (hours < 24) return `${hours.toFixed(1)}h`
  return `${(hours / 24).toFixed(1)}d`
}

const ProjectCard: FC<{
  project: TypesProject
  onViewProject: (project: TypesProject) => void
  onMenuOpen: (event: React.MouseEvent<HTMLElement>, project: TypesProject) => void
  appNamesMap: Record<string, string>
}> = ({ project, onViewProject, onMenuOpen }) => {
  const sevenDaysAgo = useMemo(() => {
    const date = new Date()
    date.setDate(date.getDate() - 7)
    return date.toISOString()
  }, [])

  const { data: usageData, isLoading: usageLoading } = useGetProjectUsage(project.id || '', {
    enabled: !!project.id,
    aggregationLevel: 'daily',
    from: sevenDaysAgo,
  })

  const { totalTokens, tokenData, trend } = useMemo(() => {
    if (!usageData || usageData.length === 0) {
      return { totalTokens: 0, tokenData: [], trend: 0 }
    }

    const total = usageData.reduce((sum, m) => sum + (m.total_tokens || 0), 0)
    const data = usageData.map(m => m.total_tokens || 0)
    
    const halfLen = Math.floor(data.length / 2)
    const firstHalf = data.slice(0, halfLen).reduce((a, b) => a + b, 0)
    const secondHalf = data.slice(halfLen).reduce((a, b) => a + b, 0)
    const trendValue = firstHalf > 0 ? ((secondHalf - firstHalf) / firstHalf) * 100 : 0

    return { totalTokens: total, tokenData: data, trend: trendValue }
  }, [usageData])

  const formatTokens = (tokens: number) => {
    if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
    if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
    return tokens.toString()
  }

  const stats = project.stats || {
    total_tasks: 0,
    backlog_tasks: 0,
    pending_review_tasks: 0,
    in_progress_tasks: 0,
    planning_tasks: 0,
    active_agent_sessions: 0,
    average_task_completion_hours: 0,
  }

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: 'background.paper',
        border: '1px solid',
        borderColor: 'rgba(0, 0, 0, 0.08)',
        borderLeft: '3px solid #a78bfa',
        borderRadius: 1,
        boxShadow: 'none',
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          backgroundColor: 'rgba(0, 0, 0, 0.01)',
        },
      }}
    >
      <CardContent
        sx={{
          flexGrow: 1,
          cursor: 'pointer',
          p: 2,
          '&:last-child': { pb: 2 },
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={() => onViewProject(project)}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1.5 }}>
          <Typography
            variant="body2"
            sx={{
              fontWeight: 500,
              flex: 1,
              lineHeight: 1.4,
              color: 'text.primary',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {project.name}
          </Typography>
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              onMenuOpen(e, project)
            }}
            sx={{
              width: 24,
              height: 24,
              color: 'text.secondary',
              ml: 0.5,
              '&:hover': {
                color: 'text.primary',
                backgroundColor: 'rgba(0, 0, 0, 0.04)',
              },
            }}
          >
            <MoreVertIcon sx={{ fontSize: 16 }} />
          </IconButton>
        </Box>

        <Box sx={{
          background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
          borderRadius: 2,
          border: '1px solid rgba(255,255,255,0.06)',
          p: 1.5,
          mb: 1.5,
        }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.75 }}>
            <Typography variant="caption" sx={{ 
              color: 'text.secondary',
              fontSize: '0.65rem',
            }}>
              Token Usage (7d)
            </Typography>
            {trend !== 0 && (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {trend > 0 ? (
                  <TrendingUpIcon sx={{ fontSize: 12, color: '#10b981' }} />
                ) : (
                  <TrendingDownIcon sx={{ fontSize: 12, color: '#ef4444' }} />
                )}
                <Typography variant="caption" sx={{ 
                  color: trend > 0 ? '#10b981' : '#ef4444',
                  fontWeight: 600,
                  fontSize: '0.65rem',
                }}>
                  {Math.abs(trend).toFixed(0)}%
                </Typography>
              </Box>
            )}
          </Box>
          
          {usageLoading ? (
            <Skeleton variant="rectangular" height={32} sx={{ bgcolor: 'rgba(255,255,255,0.05)', borderRadius: 1 }} />
          ) : (
            <MiniSparkline data={tokenData} color="#10b981" />
          )}
          
          <Box sx={{ display: 'flex', alignItems: 'baseline', flexWrap: 'wrap', gap: 0.5, mt: 0.75 }}>
            <Typography sx={{ 
              fontWeight: 600, 
              color: 'text.primary',
              fontFamily: 'monospace',
              fontSize: '1rem',
            }}>
              {formatTokens(totalTokens)}
            </Typography>
            <Typography variant="caption" sx={{ 
              color: 'text.secondary',
              fontWeight: 400,
              fontFamily: 'monospace',
              fontSize: '0.7rem',
            }}>
              tokens
            </Typography>
          </Box>
        </Box>

        <Box sx={{
          pt: 1,
          borderTop: '1px solid rgba(0, 0, 0, 0.06)',
        }}>
          <Box sx={{ 
            display: 'grid', 
            gridTemplateColumns: 'repeat(3, 1fr)',
            gap: 1,
          }}>
            <StatRow label="Backlog" value={stats.backlog_tasks || 0} />
            <StatRow label="Review" value={stats.pending_review_tasks || 0} />
            <StatRow label="Running" value={stats.active_agent_sessions || 0} />
            <StatRow label="In Progress" value={(stats.in_progress_tasks || 0) + (stats.planning_tasks || 0)} />
            <StatRow label="Total" value={stats.total_tasks || 0} />
            <StatRow label="Avg Time" value={formatDuration(stats.average_task_completion_hours || 0)} />
          </Box>
        </Box>
      </CardContent>
    </Card>
  )
}

const ProjectsListView: FC<ProjectsListViewProps> = ({
  projects,
  error,
  isLoading,
  searchQuery,
  onSearchChange,
  page,
  onPageChange,
  filteredProjects,
  paginatedProjects,
  totalPages,
  onViewProject,
  onMenuOpen,
  onCreateEmpty,
  onCreateFromSample,
  sampleProjects,
  isCreating,
  appNamesMap = {},
}) => {
  return (
    <Box sx={{ 
      minHeight: '100%',
      pb: 4,
    }}>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error instanceof Error ? error.message : 'Failed to load projects'}
        </Alert>
      )}

      <Box sx={{ mb: 4 }}>
        <Typography variant="h4" sx={{ 
          fontWeight: 700, 
          mb: 1,
          color: 'rgba(255,255,255,0.95)',
          letterSpacing: '-0.02em',
        }}>
          Projects
        </Typography>
        <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.5)' }}>
          Each Project has a Swarm of Agents working in parallel and a Team Desktop for manual testing.
        </Typography>
      </Box>

      {projects.length > 0 && (
        <Box sx={{ mb: 3, display: 'flex', alignItems: 'center', gap: 2 }}>
          <TextField
            placeholder="Filter projects..."
            size="small"
            value={searchQuery}
            onChange={(e) => {
              onSearchChange(e.target.value)
              onPageChange(0)
            }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon sx={{ fontSize: 18, color: 'rgba(255,255,255,0.4)' }} />
                </InputAdornment>
              ),
            }}
            sx={{ 
              maxWidth: 300,
              '& .MuiOutlinedInput-root': {
                background: 'rgba(255,255,255,0.03)',
                '& fieldset': {
                  borderColor: 'rgba(255,255,255,0.08)',
                },
                '&:hover fieldset': {
                  borderColor: 'rgba(255,255,255,0.15)',
                },
                '&.Mui-focused fieldset': {
                  borderColor: 'rgba(167, 139, 250, 0.5)',
                },
              },
              '& .MuiInputBase-input': {
                color: 'rgba(255,255,255,0.9)',
                '&::placeholder': {
                  color: 'rgba(255,255,255,0.4)',
                  opacity: 1,
                },
              },
            }}
          />
          {searchQuery && (
            <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.5)' }}>
              {filteredProjects.length} of {projects.length} projects
            </Typography>
          )}
        </Box>
      )}

      {filteredProjects.length === 0 && searchQuery ? (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <Typography variant="h6" sx={{ color: 'rgba(255,255,255,0.6)' }} gutterBottom>
            No projects found
          </Typography>
          <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.4)', mb: 3 }}>
            Try adjusting your search query
          </Typography>
          <Button
            variant="outlined"
            onClick={() => onSearchChange('')}
            sx={{
              borderColor: 'rgba(255,255,255,0.2)',
              color: 'rgba(255,255,255,0.7)',
              '&:hover': {
                borderColor: 'rgba(255,255,255,0.3)',
                background: 'rgba(255,255,255,0.05)',
              },
            }}
          >
            Clear Search
          </Button>
        </Box>
      ) : projects.length === 0 && !isLoading ? (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <Box sx={{ color: 'rgba(255,255,255,0.2)', mb: 2 }}>
            <Kanban size={80} />
          </Box>
          <Typography variant="h6" sx={{ color: 'rgba(255,255,255,0.6)' }} gutterBottom>
            No projects yet
          </Typography>
          <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.4)', mb: 3 }}>
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
          <Grid container spacing={{ xs: 2, sm: 3 }}>
            {paginatedProjects.map((project) => (
              <Grid item xs={12} sm={6} lg={4} key={project.id}>
                <ProjectCard
                  project={project}
                  onViewProject={onViewProject}
                  onMenuOpen={onMenuOpen}
                  appNamesMap={appNamesMap}
                />
              </Grid>
            ))}
          </Grid>

          {totalPages > 1 && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4, mb: 4 }}>
              <Pagination
                count={totalPages}
                page={page + 1}
                onChange={(_, newPage) => onPageChange(newPage - 1)}
                sx={{
                  '& .MuiPaginationItem-root': {
                    color: 'rgba(255,255,255,0.7)',
                    borderColor: 'rgba(255,255,255,0.1)',
                    '&:hover': {
                      background: 'rgba(255,255,255,0.05)',
                    },
                    '&.Mui-selected': {
                      background: 'rgba(167, 139, 250, 0.2)',
                      color: '#a78bfa',
                      '&:hover': {
                        background: 'rgba(167, 139, 250, 0.3)',
                      },
                    },
                  },
                }}
                showFirstButton
                showLastButton
              />
            </Box>
          )}
        </>
      )}
    </Box>
  )
}

export default ProjectsListView
