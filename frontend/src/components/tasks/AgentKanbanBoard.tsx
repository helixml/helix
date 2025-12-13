import React, { FC, useState, useEffect, useMemo, useCallback } from 'react'
import {
  Box,
  Typography,
  Card,
  CardContent,
  CardHeader,
  Chip,
  Button,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Tooltip,
  Alert,
  Avatar,
  Badge,
  Menu,
  ListItemIcon,
  ListItemText,
  Divider,
  Paper,
  Stack,
  Grid,
  LinearProgress
} from '@mui/material'
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
  closestCenter,
  useDroppable
} from '@dnd-kit/core'
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  Add as AddIcon,
  MoreVert as MoreVertIcon,
  Computer as ComputerIcon,
  PlayArrow as PlayArrowIcon,
  Pause as PauseIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  OpenInNew as OpenInNewIcon,
  Source as BranchIcon,
  Person as PersonIcon,
  Schedule as ScheduleIcon,
  PriorityHigh as PriorityIcon,
  Code as CodeIcon,
  BugReport as BugIcon,
  Star as FeatureIcon,
  Task as TaskIcon,
  AutoAwesome as AutoAwesomeIcon,
  ContentCopy as CopyIcon,
  Delete as DeleteIcon,
  Edit as EditIcon
} from '@mui/icons-material'
import { useTheme } from '@mui/material/styles'
// Removed date-fns dependency - using native JavaScript instead

import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import { IApp } from '../../types'

// Task types
type TaskPriority = 'low' | 'medium' | 'high' | 'critical'
type TaskType = 'feature' | 'bug' | 'task' | 'epic'
type TaskStatus = 'backlog' | 'ready' | 'in_progress' | 'review' | 'done'

interface AgentTask {
  id: string
  title: string
  description: string
  type: TaskType
  priority: TaskPriority
  status: TaskStatus
  assignedAgent?: string
  sessionId?: string
  branchName?: string
  estimatedHours?: number
  labels: string[]
  createdAt: string
  updatedAt: string
  createdBy: string
  dueDate?: string
  githubIssue?: {
    number: number
    url: string
  }
  pullRequest?: {
    number: number
    url: string
    status: 'draft' | 'open' | 'merged' | 'closed'
  }
  agentProgress?: {
    completedSteps: string[]
    currentStep: string
    blockers: string[]
  }
}

interface KanbanColumn {
  id: TaskStatus
  title: string
  color: string
  limit?: number
  tasks: AgentTask[]
}

interface AgentSession {
  id: string
  sessionId: string
  agentType: string
  status: string
  taskId?: string
  branchName?: string
  rdpUrl?: string
  workspaceDir?: string
  healthStatus: string
  lastActivity: string
}

interface SampleProject {
  id: string
  name: string
  description: string
  githubRepo: string
  defaultBranch: string
  technologies: string[]
  sampleTasks: Omit<AgentTask, 'id' | 'createdAt' | 'updatedAt' | 'createdBy'>[]
}

interface AgentKanbanBoardProps {
  projectId?: string
  apps: IApp[]
}

const SAMPLE_PROJECTS: SampleProject[] = [
  {
    id: 'todo-app',
    name: 'Modern Todo App',
    description: 'A full-stack todo application with React, Node.js, and PostgreSQL',
    githubRepo: 'helix-ai/sample-todo-app',
    defaultBranch: 'main',
    technologies: ['React', 'TypeScript', 'Node.js', 'PostgreSQL', 'Tailwind CSS'],
    sampleTasks: [
      {
        title: 'Add dark mode toggle',
        description: 'Implement a dark/light mode toggle with persistent user preference',
        type: 'feature',
        priority: 'medium',
        status: 'backlog',
        estimatedHours: 3,
        labels: ['frontend', 'ui/ux']
      },
      {
        title: 'Fix todo deletion bug',
        description: 'Todo items are not being properly deleted from the database',
        type: 'bug',
        priority: 'high',
        status: 'ready',
        estimatedHours: 1,
        labels: ['backend', 'database']
      },
      {
        title: 'Add todo categories',
        description: 'Allow users to organize todos into custom categories',
        type: 'feature',
        priority: 'low',
        status: 'backlog',
        estimatedHours: 5,
        labels: ['frontend', 'backend', 'database']
      },
      {
        title: 'Implement user authentication',
        description: 'Add JWT-based authentication with login/register pages',
        type: 'feature',
        priority: 'critical',
        status: 'ready',
        estimatedHours: 8,
        labels: ['backend', 'security', 'frontend']
      }
    ]
  },
  {
    id: 'ecommerce-api',
    name: 'E-commerce API',
    description: 'RESTful API for an e-commerce platform with order management',
    githubRepo: 'helix-ai/sample-ecommerce-api',
    defaultBranch: 'main',
    technologies: ['Node.js', 'Express', 'MongoDB', 'Redis', 'Docker'],
    sampleTasks: [
      {
        title: 'Add product search endpoint',
        description: 'Create API endpoint for full-text search across products',
        type: 'feature',
        priority: 'high',
        status: 'backlog',
        estimatedHours: 4,
        labels: ['api', 'search']
      },
      {
        title: 'Implement order cancellation',
        description: 'Add ability to cancel orders within 1 hour of placement',
        type: 'feature',
        priority: 'medium',
        status: 'backlog',
        estimatedHours: 3,
        labels: ['orders', 'business-logic']
      },
      {
        title: 'Fix inventory race condition',
        description: 'Prevent overselling when multiple users buy the same item',
        type: 'bug',
        priority: 'critical',
        status: 'ready',
        estimatedHours: 2,
        labels: ['concurrency', 'inventory']
      }
    ]
  }
]

const AgentKanbanBoard: FC<AgentKanbanBoardProps> = ({ projectId, apps }) => {
  const theme = useTheme()
  const api = useApi()
  const account = useAccount()

  // State
  const [tasks, setTasks] = useState<AgentTask[]>([])
  const [sessions, setSessions] = useState<AgentSession[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  
  // Dialog states
  const [createTaskOpen, setCreateTaskOpen] = useState(false)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)
  const [selectedSampleProject, setSelectedSampleProject] = useState<SampleProject | null>(null)
  
  // Task form state
  const [newTask, setNewTask] = useState<Partial<AgentTask>>({
    type: 'task',
    priority: 'medium',
    status: 'backlog',
    labels: []
  })

  // Menu states
  const [taskMenuAnchor, setTaskMenuAnchor] = useState<null | HTMLElement>(null)
  const [selectedTask, setSelectedTask] = useState<AgentTask | null>(null)

  // WIP limits for kanban columns
  const WIP_LIMITS = {
    backlog: undefined,
    ready: 5,
    in_progress: 3, // Limit to avoid merge conflicts
    review: 5,
    done: undefined
  }

  // Kanban columns configuration
  const columns: KanbanColumn[] = useMemo(() => [
    {
      id: 'backlog',
      title: 'Backlog',
      color: theme.palette.grey[600],
      tasks: tasks.filter(t => t.status === 'backlog')
    },
    {
      id: 'ready',
      title: 'Ready',
      color: theme.palette.info.main,
      limit: WIP_LIMITS.ready,
      tasks: tasks.filter(t => t.status === 'ready')
    },
    {
      id: 'in_progress',
      title: 'In Progress',
      color: theme.palette.warning.main,
      limit: WIP_LIMITS.in_progress,
      tasks: tasks.filter(t => t.status === 'in_progress')
    },
    {
      id: 'review',
      title: 'Review',
      color: theme.palette.secondary.main,
      limit: WIP_LIMITS.review,
      tasks: tasks.filter(t => t.status === 'review')
    },
    {
      id: 'done',
      title: 'Done',
      color: theme.palette.success.main,
      tasks: tasks.filter(t => t.status === 'done')
    }
  ], [tasks, theme])

  // Load data
  useEffect(() => {
    loadData()
    const interval = setInterval(loadData, 30000) // Refresh every 30 seconds
    return () => clearInterval(interval)
  }, [projectId])

  const loadData = async () => {
    try {
      setLoading(true)
      
      // Load tasks
      const tasksResponse = await api.get(`/api/v1/projects/${projectId || 'default'}/tasks`)
      setTasks(tasksResponse.data.tasks || [])
      
      // Load active agent sessions
      const sessionsResponse = await api.get('/api/v1/agents/sessions?active_only=true')
      setSessions(sessionsResponse.data.sessions || [])
      
      setError(null)
    } catch (err: any) {
      setError(err.message || 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }



  // Create new task
  const createTask = async () => {
    try {
      const taskData = {
        ...newTask,
        id: `task-${Date.now()}`,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        createdBy: account.user?.id || 'unknown'
      }

      const response = await api.post(`/api/v1/projects/${projectId || 'default'}/tasks`, taskData)
      setTasks(prev => [...prev, response.data])
      setCreateTaskOpen(false)
      setNewTask({ type: 'task', priority: 'medium', status: 'backlog', labels: [] })
    } catch (err: any) {
      setError(err.message || 'Failed to create task')
    }
  }

  // Assign agent to task
  const assignAgentToTask = async (taskId: string) => {
    try {
      const response = await api.post(`/api/v1/agents/work`, {
        name: tasks.find(t => t.id === taskId)?.title || 'Unnamed Task',
        description: tasks.find(t => t.id === taskId)?.description || '',
        source: 'kanban',
        agent_type: 'zed',
        work_data: {
          task_id: taskId,
          project_id: projectId || 'default'
        },
        priority: getPriorityNumber(tasks.find(t => t.id === taskId)?.priority || 'medium')
      })

      // Update task with session info
      const sessionId = response.data.session_id
      if (sessionId) {
        await api.put(`/api/v1/projects/${projectId || 'default'}/tasks/${taskId}`, {
          sessionId,
          assignedAgent: 'zed'
        })
        loadData() // Refresh data
      }
    } catch (err: any) {
      setError(err.message || 'Failed to assign agent')
    }
  }

  // Create sample project
  const createSampleProject = async (sampleProject: SampleProject) => {
    try {
      setLoading(true)
      
      // Fork the repository
      const forkResponse = await api.post('/api/v1/projects/fork-sample', {
        sample_project_id: sampleProject.id,
        github_repo: sampleProject.githubRepo,
        name: `${sampleProject.name} - ${account.user?.name || 'User'}`
      })

      const newProjectId = forkResponse.data.project_id
      
      // Create sample tasks
      const createdTasks = []
      for (const taskData of sampleProject.sampleTasks) {
        const task = {
          ...taskData,
          id: `task-${Date.now()}-${Math.random()}`,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
          createdBy: account.user?.id || 'unknown'
        }
        
        const response = await api.post(`/api/v1/projects/${newProjectId}/tasks`, task)
        createdTasks.push(response.data)
      }

      setTasks(createdTasks)
      setCreateProjectOpen(false)
      setSelectedSampleProject(null)
      
      // Update URL or navigate to new project
      window.history.pushState({}, '', `/projects/${newProjectId}/tasks`)
      
    } catch (err: any) {
      setError(err.message || 'Failed to create sample project')
    } finally {
      setLoading(false)
    }
  }

  // Helper functions
  const getPriorityColor = (priority: TaskPriority) => {
    switch (priority) {
      case 'critical': return theme.palette.error.main
      case 'high': return theme.palette.warning.main
      case 'medium': return theme.palette.info.main
      case 'low': return theme.palette.success.main
      default: return theme.palette.grey[500]
    }
  }

  const getPriorityNumber = (priority: TaskPriority) => {
    switch (priority) {
      case 'critical': return 1
      case 'high': return 2
      case 'medium': return 3
      case 'low': return 4
      default: return 3
    }
  }

  const getTypeIcon = (type: TaskType) => {
    switch (type) {
      case 'feature': return <FeatureIcon />
      case 'bug': return <BugIcon />
      case 'task': return <TaskIcon />
      case 'epic': return <AutoAwesomeIcon />
      default: return <TaskIcon />
    }
  }

  const getSessionForTask = (taskId: string) => {
    return sessions.find(s => s.taskId === taskId)
  }

  // Drag and drop sensors
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

  // Handle drag end
  const onDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event
    
    if (!over) return
    
    const taskId = active.id as string
    const newStatus = over.id as TaskStatus
    
    // Update task status
    setTasks(prevTasks => 
      prevTasks.map(task => 
        task.id === taskId 
          ? { ...task, status: newStatus }
          : task
      )
    )
    
    // TODO: Call API to update task status
    console.log(`Moving task ${taskId} to ${newStatus}`)
  }, [setTasks])

  if (loading && tasks.length === 0) {
    return <LinearProgress />
  }

  return (
    <Box sx={{ p: 3, height: '100vh', overflow: 'hidden' }}>
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4" component="h1">
          Agent Task Board
        </Typography>
        <Stack direction="row" spacing={2}>
          <Button
            variant="outlined"
            startIcon={<AutoAwesomeIcon />}
            onClick={() => setCreateProjectOpen(true)}
          >
            Try Sample Project
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setCreateTaskOpen(true)}
          >
            Add Task
          </Button>
        </Stack>
      </Box>

      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

      {/* Kanban Board */}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={onDragEnd}
      >
        <Box sx={{ display: 'flex', gap: 2, height: 'calc(100vh - 200px)', overflow: 'auto' }}>
          {columns.map((column) => (
            <DroppableColumn key={column.id} column={column} sessions={sessions} />
          ))}
        </Box>
      </DndContext>

      {/* Create Task Dialog */}
      <Dialog open={createTaskOpen} onClose={() => setCreateTaskOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Create New Task</DialogTitle>
        <DialogContent>
          <Grid container spacing={2} sx={{ mt: 1 }}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Title"
                value={newTask.title || ''}
                onChange={(e) => setNewTask({ ...newTask, title: e.target.value })}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                multiline
                rows={3}
                label="Description"
                value={newTask.description || ''}
                onChange={(e) => setNewTask({ ...newTask, description: e.target.value })}
              />
            </Grid>
            <Grid item xs={6}>
              <FormControl fullWidth>
                <InputLabel>Type</InputLabel>
                <Select
                  value={newTask.type || 'task'}
                  label="Type"
                  onChange={(e) => setNewTask({ ...newTask, type: e.target.value as TaskType })}
                >
                  <MenuItem value="feature">Feature</MenuItem>
                  <MenuItem value="bug">Bug</MenuItem>
                  <MenuItem value="task">Task</MenuItem>
                  <MenuItem value="epic">Epic</MenuItem>
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={6}>
              <FormControl fullWidth>
                <InputLabel>Priority</InputLabel>
                <Select
                  value={newTask.priority || 'medium'}
                  label="Priority"
                  onChange={(e) => setNewTask({ ...newTask, priority: e.target.value as TaskPriority })}
                >
                  <MenuItem value="low">Low</MenuItem>
                  <MenuItem value="medium">Medium</MenuItem>
                  <MenuItem value="high">High</MenuItem>
                  <MenuItem value="critical">Critical</MenuItem>
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={6}>
              <TextField
                fullWidth
                type="number"
                label="Estimated Hours"
                value={newTask.estimatedHours || ''}
                onChange={(e) => setNewTask({ ...newTask, estimatedHours: parseInt(e.target.value) })}
              />
            </Grid>
            <Grid item xs={6}>
              <TextField
                fullWidth
                label="Labels (comma-separated)"
                value={newTask.labels?.join(', ') || ''}
                onChange={(e) => setNewTask({ 
                  ...newTask, 
                  labels: e.target.value.split(',').map(l => l.trim()).filter(l => l) 
                })}
              />
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateTaskOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={createTask}>Create Task</Button>
        </DialogActions>
      </Dialog>

      {/* Sample Project Dialog */}
      <Dialog open={createProjectOpen} onClose={() => setCreateProjectOpen(false)} maxWidth="lg" fullWidth>
        <DialogTitle>Try a Sample Project</DialogTitle>
        <DialogContent>
          <Typography variant="body1" sx={{ mb: 3 }}>
            Fork a sample project and watch AI agents work on different tasks in parallel branches!
          </Typography>
          <Grid container spacing={3}>
            {SAMPLE_PROJECTS.map((project) => (
              <Grid item xs={12} md={6} key={project.id}>
                <Card 
                  sx={{ 
                    cursor: 'pointer', 
                    border: selectedSampleProject?.id === project.id ? 2 : 1,
                    borderColor: selectedSampleProject?.id === project.id ? 'primary.main' : 'divider'
                  }}
                  onClick={() => setSelectedSampleProject(project)}
                >
                  <CardContent>
                    <Typography variant="h6" gutterBottom>{project.name}</Typography>
                    <Typography variant="body2" color="textSecondary" sx={{ mb: 2 }}>
                      {project.description}
                    </Typography>
                    <Box sx={{ mb: 2 }}>
                      <Typography variant="caption" color="textSecondary">Technologies:</Typography>
                      <Box sx={{ mt: 0.5 }}>
                        {project.technologies.map((tech) => (
                          <Chip key={tech} label={tech} size="small" sx={{ mr: 0.5, mb: 0.5 }} />
                        ))}
                      </Box>
                    </Box>
                    <Typography variant="caption" color="textSecondary">
                      {project.sampleTasks.length} sample tasks • GitHub: {project.githubRepo}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            ))}
          </Grid>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateProjectOpen(false)}>Cancel</Button>
          <Button 
            variant="contained" 
            onClick={() => selectedSampleProject && createSampleProject(selectedSampleProject)}
            disabled={!selectedSampleProject}
          >
            Fork & Create Project
          </Button>
        </DialogActions>
      </Dialog>

      {/* Task Menu */}
      <Menu
        anchorEl={taskMenuAnchor}
        open={Boolean(taskMenuAnchor)}
        onClose={() => setTaskMenuAnchor(null)}
      >
        <MenuItem onClick={() => {
          if (selectedTask) {
            assignAgentToTask(selectedTask.id)
          }
          setTaskMenuAnchor(null)
        }}>
          <ListItemIcon>
            <PlayArrowIcon />
          </ListItemIcon>
          <ListItemText>Assign External Agent</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => {
          if (selectedTask?.sessionId) {
            const session = sessions.find(s => s.sessionId === selectedTask.sessionId)
            if (session?.rdpUrl) {
              window.open(session.rdpUrl, '_blank')
            }
          }
          setTaskMenuAnchor(null)
        }}>
          <ListItemIcon>
            <OpenInNewIcon />
          </ListItemIcon>
          <ListItemText>Open RDP Session</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => {
          if (selectedTask) {
            navigator.clipboard.writeText(selectedTask.id)
          }
          setTaskMenuAnchor(null)
        }}>
          <ListItemIcon>
            <CopyIcon />
          </ListItemIcon>
          <ListItemText>Copy Task ID</ListItemText>
        </MenuItem>
        <Divider />
        <MenuItem onClick={() => {
          setTaskMenuAnchor(null)
        }}>
          <ListItemIcon>
            <EditIcon />
          </ListItemIcon>
          <ListItemText>Edit Task</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => {
          setTaskMenuAnchor(null)
        }}>
          <ListItemIcon>
            <DeleteIcon />
          </ListItemIcon>
          <ListItemText>Delete Task</ListItemText>
        </MenuItem>
      </Menu>
    </Box>
  )
}

// Sortable Task Card Component
interface SortableTaskCardProps {
  task: AgentTask
  sessions: AgentSession[]
}

const SortableTaskCard: FC<SortableTaskCardProps> = ({ task, sessions }) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: task.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  const [taskMenuAnchor, setTaskMenuAnchor] = useState<null | HTMLElement>(null)
  const session = sessions.find(s => s.taskId === task.id)

  const getTypeIcon = (type: TaskType) => {
    switch (type) {
      case 'feature': return <FeatureIcon sx={{ fontSize: 16, color: '#4caf50' }} />
      case 'bug': return <BugIcon sx={{ fontSize: 16, color: '#f44336' }} />
      case 'task': return <TaskIcon sx={{ fontSize: 16, color: '#2196f3' }} />
      case 'epic': return <PriorityIcon sx={{ fontSize: 16, color: '#9c27b0' }} />
    }
  }

  const getPriorityColor = (priority: TaskPriority) => {
    switch (priority) {
      case 'low': return '#4caf50'
      case 'medium': return '#ff9800'
      case 'high': return '#f44336'
      case 'critical': return '#9c27b0'
    }
  }

  return (
    <Card
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      sx={{
        mb: 1,
        backgroundColor: isDragging ? 'background.paper' : 'background.default',
        border: 1,
        borderColor: isDragging ? 'primary.main' : 'divider',
        boxShadow: isDragging ? 4 : 1,
        '&:hover': { boxShadow: 2 },
        cursor: 'grab',
        '&:active': { cursor: 'grabbing' }
      }}
    >
      <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
        {/* Task Header */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {getTypeIcon(task.type)}
            <Chip
              label={task.priority}
              size="small"
              sx={{
                backgroundColor: getPriorityColor(task.priority) + '20',
                color: getPriorityColor(task.priority),
                fontWeight: 'bold'
              }}
            />
          </Box>
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              setTaskMenuAnchor(e.currentTarget)
            }}
          >
            <MoreVertIcon />
          </IconButton>
        </Box>

        {/* Task Title */}
        <Typography variant="subtitle2" fontWeight="bold" gutterBottom>
          {task.title}
        </Typography>

        {/* Task Description */}
        <Typography variant="body2" color="textSecondary" sx={{ mb: 1 }}>
          {task.description.length > 100 
            ? task.description.slice(0, 100) + '...' 
            : task.description}
        </Typography>

        {/* Labels */}
        {task.labels.length > 0 && (
          <Box sx={{ mb: 1 }}>
            {task.labels.map((label) => (
              <Chip
                key={label}
                label={label}
                size="small"
                variant="outlined"
                sx={{ mr: 0.5, mb: 0.5, fontSize: '0.7rem' }}
              />
            ))}
          </Box>
        )}

        {/* Task Footer */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {task.estimatedHours && (
              <Chip
                label={`${task.estimatedHours}h`}
                size="small"
                icon={<ScheduleIcon />}
                variant="outlined"
              />
            )}
            {task.branchName && (
              <Chip
                label={task.branchName}
                size="small"
                icon={<BranchIcon />}
                variant="outlined"
              />
            )}
          </Box>
          
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {session && (
              <Tooltip title={`Agent: ${session.agentType} (${session.status})`}>
                <Avatar
                  sx={{
                    width: 24,
                    height: 24,
                    bgcolor: session.status === 'active' ? 'success.main' : 'warning.main'
                  }}
                >
                  <ComputerIcon sx={{ fontSize: 16 }} />
                </Avatar>
              </Tooltip>
            )}
            {session?.rdpUrl && (
              <Tooltip title="Open RDP Session">
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation()
                    window.open(session.rdpUrl, '_blank')
                  }}
                >
                  <OpenInNewIcon />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        </Box>
      </CardContent>

      {/* Task Menu */}
      <Menu
        anchorEl={taskMenuAnchor}
        open={Boolean(taskMenuAnchor)}
        onClose={() => setTaskMenuAnchor(null)}
      >
        <MenuItem onClick={() => setTaskMenuAnchor(null)}>
          <ListItemIcon>
            <PlayArrowIcon />
          </ListItemIcon>
          <ListItemText>Assign External Agent</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => setTaskMenuAnchor(null)}>
          <ListItemIcon>
            <OpenInNewIcon />
          </ListItemIcon>
          <ListItemText>Open RDP Session</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => setTaskMenuAnchor(null)}>
          <ListItemIcon>
            <CopyIcon />
          </ListItemIcon>
          <ListItemText>Copy Task ID</ListItemText>
        </MenuItem>
        <Divider />
        <MenuItem onClick={() => setTaskMenuAnchor(null)}>
          <ListItemIcon>
            <EditIcon />
          </ListItemIcon>
          <ListItemText>Edit Task</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => setTaskMenuAnchor(null)}>
          <ListItemIcon>
            <DeleteIcon />
          </ListItemIcon>
          <ListItemText>Delete Task</ListItemText>
        </MenuItem>
      </Menu>
    </Card>
  )
}

// Droppable Column Component
interface DroppableColumnProps {
  column: any
  sessions: AgentSession[]
}

const DroppableColumn: FC<DroppableColumnProps> = ({ column, sessions }) => {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  })

  const theme = useTheme()

  return (
    <Paper
      ref={setNodeRef}
      sx={{
        minWidth: 300,
        maxWidth: 300,
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: isOver ? column.color + '10' : theme.palette.background.default,
        border: isOver ? 2 : 1,
        borderColor: isOver ? column.color : 'divider'
      }}
    >
      {/* Column Header */}
      <Box
        sx={{
          p: 2,
          borderBottom: 1,
          borderColor: 'divider',
          backgroundColor: column.color + '20'
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="h6" sx={{ color: column.color, fontWeight: 'bold' }}>
            {column.title}
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Chip
              label={column.tasks.length}
              size="small"
              sx={{
                backgroundColor: column.color,
                color: 'white',
                fontWeight: 'bold'
              }}
            />
            {column.limit && column.tasks.length >= column.limit && (
              <Chip
                label="FULL"
                size="small"
                color="error"
                variant="outlined"
              />
            )}
          </Box>
        </Box>
        {column.limit && (
          <LinearProgress
            variant="determinate"
            value={(column.tasks.length / column.limit) * 100}
            sx={{
              mt: 1,
              height: 4,
              backgroundColor: column.color + '20',
              '& .MuiLinearProgress-bar': {
                backgroundColor: column.tasks.length >= column.limit ? theme.palette.error.main : column.color
              }
            }}
          />
        )}
      </Box>

      {/* Column Content */}
      <SortableContext items={column.tasks.map((t: any) => t.id)} strategy={verticalListSortingStrategy}>
        <Box
          sx={{
            flex: 1,
            p: 1,
            minHeight: 200,
            overflow: 'auto'
          }}
        >
          {column.tasks.map((task: any) => (
            <SortableTaskCard key={task.id} task={task} sessions={sessions} />
          ))}
        </Box>
      </SortableContext>
    </Paper>
  )
}

export default AgentKanbanBoard