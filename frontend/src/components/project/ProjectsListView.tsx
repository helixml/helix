import React, { FC, useMemo, useState } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  Grid,
  Typography,
  IconButton,
  Alert,
  Pagination,
  Skeleton,
  Popper,
  Paper,
  Fade,
} from '@mui/material'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import TrendingUpIcon from '@mui/icons-material/TrendingUp'
import TrendingDownIcon from '@mui/icons-material/TrendingDown'
import { Kanban } from 'lucide-react'

import CreateProjectButton from './CreateProjectButton'
import { TypesProject } from '../../services'
import type { ServerSampleProject, TypesAggregatedUsageMetric } from '../../api/api'
import { useGetProjectUsage } from '../../services/projectService'

interface ProjectsListViewProps {
  projects: TypesProject[]
  error: Error | null
  isLoading: boolean
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

const formatNumber = (num: number) => {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`
  return num.toString()
}

const MiniSparkline: FC<{ data: TypesAggregatedUsageMetric[]; color: string }> = ({ data, color }) => {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const containerRef = React.useRef<HTMLDivElement>(null)

  if (!data || data.length === 0) {
    return (
      <Box sx={{ height: 32, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Typography variant="caption" sx={{ color: 'text.disabled', fontSize: '0.65rem' }}>
          No usage data
        </Typography>
      </Box>
    )
  }

  const tokenData = data.map(m => m.total_tokens || 0)
  const max = Math.max(...tokenData, 1)
  const min = Math.min(...tokenData, 0)
  const range = max - min || 1
  const width = 100
  const height = 32
  const padding = 2

  const points = tokenData.map((value, index) => {
    const x = padding + (index / (tokenData.length - 1 || 1)) * (width - padding * 2)
    const y = height - padding - ((value - min) / range) * (height - padding * 2)
    return `${x},${y}`
  }).join(' ')

  const areaPoints = `${padding},${height - padding} ${points} ${width - padding},${height - padding}`

  const handleMouseMove = (event: React.MouseEvent<SVGRectElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - rect.left
    const relativeX = x / rect.width
    const index = Math.round(relativeX * (data.length - 1))
    const clampedIndex = Math.max(0, Math.min(data.length - 1, index))
    setHoveredIndex(clampedIndex)
    setAnchorEl(containerRef.current)
  }

  const handleMouseLeave = () => {
    setHoveredIndex(null)
    setAnchorEl(null)
  }

  const hoveredData = hoveredIndex !== null ? data[hoveredIndex] : null
  const hoveredX = hoveredIndex !== null 
    ? padding + (hoveredIndex / (data.length - 1 || 1)) * (width - padding * 2)
    : 0

  return (
    <Box sx={{ position: 'relative' }} ref={containerRef}>
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
        {hoveredIndex !== null && (
          <line
            x1={hoveredX}
            y1={0}
            x2={hoveredX}
            y2={height}
            stroke="rgba(255,255,255,0.5)"
            strokeWidth="0.5"
            strokeDasharray="1,1"
          />
        )}
        <rect
          x={0}
          y={0}
          width={width}
          height={height}
          fill="transparent"
          style={{ cursor: 'crosshair' }}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
        />
      </svg>
      <Popper
        open={hoveredIndex !== null}
        anchorEl={anchorEl}
        placement="top"
        modifiers={[{ name: 'offset', options: { offset: [0, 8] } }]}
        sx={{ zIndex: 1500 }}
      >
        <Fade in={hoveredIndex !== null} timeout={150}>
          <Paper sx={{
            p: 1,
            backgroundColor: 'rgba(30, 30, 30, 0.95)',
            border: '1px solid rgba(255,255,255,0.1)',
            borderRadius: 1,
          }}>
            {hoveredData && (
              <Box sx={{ minWidth: 100 }}>
                <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 0.5 }}>
                  {new Date(hoveredData.date || '').toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })}
                </Typography>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>Tokens:</Typography>
                  <Typography variant="caption" sx={{ color: 'text.primary', fontWeight: 600, fontFamily: 'monospace' }}>
                    {formatNumber(hoveredData.total_tokens || 0)}
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>Requests:</Typography>
                  <Typography variant="caption" sx={{ color: 'text.primary', fontWeight: 600, fontFamily: 'monospace' }}>
                    {formatNumber(hoveredData.total_requests || 0)}
                  </Typography>
                </Box>
              </Box>
            )}
          </Paper>
        </Fade>
      </Popper>
    </Box>
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

  const { totalTokens, trend } = useMemo(() => {
    if (!usageData || usageData.length === 0) {
      return { totalTokens: 0, trend: 0 }
    }

    const total = usageData.reduce((sum, m) => sum + (m.total_tokens || 0), 0)
    const data = usageData.map(m => m.total_tokens || 0)
    
    const halfLen = Math.floor(data.length / 2)
    const firstHalf = data.slice(0, halfLen).reduce((a, b) => a + b, 0)
    const secondHalf = data.slice(halfLen).reduce((a, b) => a + b, 0)
    const trendValue = firstHalf > 0 ? ((secondHalf - firstHalf) / firstHalf) * 100 : 0

    return { totalTokens: total, trend: trendValue }
  }, [usageData])

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
        borderLeft: '3px solid transparent',
        borderRadius: 1,
        boxShadow: 'none',
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          borderLeftColor: 'secondary.main',
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
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography
              variant="body2"
              sx={{
                fontWeight: 500,
                lineHeight: 1.4,
                color: 'text.primary',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {project.name}
            </Typography>
            {project.description && (
              <Typography
                variant="caption"
                sx={{
                  color: 'text.secondary',
                  fontSize: '0.7rem',
                  display: 'block',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {project.description}
              </Typography>
            )}
          </Box>
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
              flexShrink: 0,
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
            <MiniSparkline data={usageData || []} color="#10b981" />
          )}
          
          <Box sx={{ display: 'flex', alignItems: 'baseline', flexWrap: 'wrap', gap: 0.5, mt: 0.75 }}>
            <Typography sx={{ 
              fontWeight: 600, 
              color: 'text.primary',
              fontFamily: 'monospace',
              fontSize: '1rem',
            }}>
              {formatNumber(totalTokens)}
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
            <StatRow label="Desktops" value={stats.active_agent_sessions || 0} />
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
          Each Project has a Team of Agents working in parallel to perform tasks, collaborate, and build software.
        </Typography>
      </Box>


      {projects.length === 0 && !isLoading ? (
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
