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
  Tooltip,
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
      <Box sx={{ height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.3)' }}>
          No usage data
        </Typography>
      </Box>
    )
  }

  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const width = 100
  const height = 40
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
      color: 'rgba(255,255,255,0.5)',
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
      color: 'rgba(255,255,255,0.95)',
      fontSize: '0.85rem',
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
}> = ({ project, onViewProject, onMenuOpen, appNamesMap }) => {
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
        background: 'linear-gradient(145deg, rgba(38, 40, 48, 0.95) 0%, rgba(30, 32, 38, 0.98) 100%)',
        border: '1px solid rgba(255,255,255,0.06)',
        borderRadius: 2,
        overflow: 'hidden',
        transition: 'all 0.2s ease-in-out',
        '&:hover': {
          border: '1px solid rgba(255,255,255,0.12)',
          transform: 'translateY(-2px)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.3)',
        },
      }}
    >
      <CardContent
        sx={{
          flexGrow: 1,
          cursor: 'pointer',
          p: 2.5,
          pb: '16px !important',
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={() => onViewProject(project)}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flex: 1, minWidth: 0 }}>
            <Box sx={{
              p: 1,
              borderRadius: 1.5,
              background: 'linear-gradient(145deg, rgba(99, 102, 241, 0.15) 0%, rgba(139, 92, 246, 0.1) 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}>
              <Kanban size={18} style={{ color: '#a78bfa' }} />
            </Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: 600,
                  color: 'rgba(255,255,255,0.95)',
                  lineHeight: 1.3,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {project.name}
              </Typography>
              {project.default_helix_app_id && appNamesMap[project.default_helix_app_id] && (
                <Typography
                  variant="caption"
                  sx={{
                    color: 'rgba(167, 139, 250, 0.8)',
                    fontSize: '0.7rem',
                  }}
                >
                  {appNamesMap[project.default_helix_app_id]}
                </Typography>
              )}
            </Box>
          </Box>
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              onMenuOpen(e, project)
            }}
            sx={{
              p: 0.5,
              color: 'rgba(255,255,255,0.4)',
              '&:hover': { color: 'rgba(255,255,255,0.7)', background: 'rgba(255,255,255,0.05)' },
            }}
          >
            <MoreVertIcon sx={{ fontSize: 18 }} />
          </IconButton>
        </Box>

        <Box sx={{
          background: 'rgba(0,0,0,0.2)',
          borderRadius: 1.5,
          p: 2,
          mb: 2,
        }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
            <Typography variant="caption" sx={{ 
              color: 'rgba(255,255,255,0.5)',
              textTransform: 'uppercase',
              letterSpacing: '0.08em',
              fontSize: '0.65rem',
            }}>
              Token Usage (7d)
            </Typography>
            {trend !== 0 && (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {trend > 0 ? (
                  <TrendingUpIcon sx={{ fontSize: 14, color: '#4ade80' }} />
                ) : (
                  <TrendingDownIcon sx={{ fontSize: 14, color: '#f87171' }} />
                )}
                <Typography variant="caption" sx={{ 
                  color: trend > 0 ? '#4ade80' : '#f87171',
                  fontWeight: 600,
                  fontSize: '0.7rem',
                }}>
                  {Math.abs(trend).toFixed(0)}%
                </Typography>
              </Box>
            )}
          </Box>
          
          {usageLoading ? (
            <Skeleton variant="rectangular" height={40} sx={{ bgcolor: 'rgba(255,255,255,0.05)', borderRadius: 1 }} />
          ) : (
            <MiniSparkline data={tokenData} color="#4ade80" />
          )}
          
          <Box sx={{ display: 'flex', alignItems: 'baseline', flexWrap: 'wrap', gap: 0.5, mt: 1 }}>
            <Typography sx={{ 
              fontWeight: 700, 
              color: 'rgba(255,255,255,0.95)',
              fontFamily: 'monospace',
              fontSize: { xs: '1.25rem', sm: '1.5rem' },
            }}>
              {formatTokens(totalTokens)}
            </Typography>
            <Typography variant="caption" sx={{ 
              color: 'rgba(255,255,255,0.4)',
              fontWeight: 400,
              fontFamily: 'monospace',
            }}>
              tokens
            </Typography>
          </Box>
        </Box>

        <Box sx={{
          pt: 1.5,
          borderTop: '1px solid rgba(255,255,255,0.06)',
        }}>
          <Box sx={{ 
            display: 'grid', 
            gridTemplateColumns: 'repeat(3, 1fr)',
            gap: 1.5,
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
