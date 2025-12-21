import React, { useState, useMemo } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Checkbox,
  FormControlLabel,
  TextField,
  Divider,
  Chip,
  CircularProgress,
  Alert,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  Collapse,
  IconButton,
} from '@mui/material';
import { Copy, ChevronDown, ChevronRight, FolderGit2, Check, ExternalLink } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import useApi from '../../hooks/useApi';
import { useCloneTask, useReposWithoutProjects, CloneTaskResponse } from '../../services/specTaskService';
import { TypesProject, TypesGitRepository } from '../../api/api';
import CloneGroupProgressFull from './CloneGroupProgress';

interface CloneTaskDialogProps {
  open: boolean;
  onClose: () => void;
  taskId: string;
  taskName: string;
  sourceProjectId: string;
  onCloneComplete?: (result: CloneTaskResponse) => void;
}

const CloneTaskDialog: React.FC<CloneTaskDialogProps> = ({
  open,
  onClose,
  taskId,
  taskName,
  sourceProjectId,
  onCloneComplete,
}) => {
  const api = useApi();
  const cloneTaskMutation = useCloneTask();

  // Selected targets
  const [selectedProjects, setSelectedProjects] = useState<string[]>([]);
  const [selectedRepos, setSelectedRepos] = useState<{ repo_id: string; name?: string }[]>([]);
  const [autoStart, setAutoStart] = useState(true);

  // Expansion state
  const [projectsExpanded, setProjectsExpanded] = useState(true);
  const [reposExpanded, setReposExpanded] = useState(false);

  // Search filters
  const [projectSearch, setProjectSearch] = useState('');
  const [repoSearch, setRepoSearch] = useState('');

  // Result state
  const [cloneResult, setCloneResult] = useState<CloneTaskResponse | null>(null);
  const [cloneError, setCloneError] = useState<string | null>(null);
  const [showCloneProgress, setShowCloneProgress] = useState(false);

  // Fetch projects
  const { data: projectsData, isLoading: loadingProjects } = useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const response = await api.getApiClient().v1ProjectsList();
      return response.data;
    },
    enabled: open,
  });

  // Filter out source project
  // Note: v1ProjectsList returns TypesProject[] directly, not an object with .projects
  const availableProjects = useMemo(() => {
    const projects = projectsData || [];
    return projects.filter((p: TypesProject) => p.id !== sourceProjectId);
  }, [projectsData, sourceProjectId]);

  // Fetch repos without projects
  const { data: reposWithoutProjects, isLoading: loadingRepos } = useReposWithoutProjects();

  // Filtered projects based on search
  const filteredProjects = useMemo(() => {
    if (!projectSearch.trim()) return availableProjects;
    const search = projectSearch.toLowerCase();
    return availableProjects.filter((p: TypesProject) =>
      p.name?.toLowerCase().includes(search) ||
      p.description?.toLowerCase().includes(search)
    );
  }, [availableProjects, projectSearch]);

  // Filtered repos based on search
  const filteredRepos = useMemo(() => {
    if (!repoSearch.trim()) return reposWithoutProjects || [];
    const search = repoSearch.toLowerCase();
    return (reposWithoutProjects || []).filter((r: TypesGitRepository) =>
      r.name?.toLowerCase().includes(search) ||
      r.description?.toLowerCase().includes(search)
    );
  }, [reposWithoutProjects, repoSearch]);

  const handleProjectToggle = (projectId: string) => {
    setSelectedProjects(prev =>
      prev.includes(projectId)
        ? prev.filter(id => id !== projectId)
        : [...prev, projectId]
    );
  };

  const handleRepoToggle = (repoId: string, repoName: string) => {
    setSelectedRepos(prev => {
      const exists = prev.some(r => r.repo_id === repoId);
      if (exists) {
        return prev.filter(r => r.repo_id !== repoId);
      } else {
        return [...prev, { repo_id: repoId, name: repoName }];
      }
    });
  };

  const handleClone = async () => {
    setCloneError(null);
    try {
      const result = await cloneTaskMutation.mutateAsync({
        taskId,
        request: {
          target_project_ids: selectedProjects,
          create_projects: selectedRepos,
          auto_start: autoStart,
        },
      });
      setCloneResult(result);
      if (onCloneComplete) {
        onCloneComplete(result);
      }
    } catch (error: unknown) {
      console.error('Clone failed:', error);
      const errorMessage = error instanceof Error ? error.message :
        (error as { response?: { data?: string } })?.response?.data || 'Clone operation failed';
      setCloneError(errorMessage);
    }
  };

  const handleClose = () => {
    setSelectedProjects([]);
    setSelectedRepos([]);
    setCloneResult(null);
    setCloneError(null);
    setProjectSearch('');
    setRepoSearch('');
    setShowCloneProgress(false);
    onClose();
  };

  const totalTargets = selectedProjects.length + selectedRepos.length;

  if (cloneResult) {
    // Handle task click to navigate to the project's kanban board
    const handleTaskClick = (taskId: string, projectId: string) => {
      handleClose();
      // Navigate to the project's kanban board - the task will be visible there
      window.location.href = `/projects/${projectId}`;
    };

    // Show clone group progress view
    if (showCloneProgress && cloneResult.clone_group_id) {
      return (
        <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth onClick={(e) => e.stopPropagation()}>
          <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Copy size={24} />
            Clone Group Progress
          </DialogTitle>
          <DialogContent>
            <CloneGroupProgressFull
              groupId={cloneResult.clone_group_id}
              onTaskClick={handleTaskClick}
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setShowCloneProgress(false)}>
              Back
            </Button>
            <Button onClick={handleClose} variant="contained">
              Done
            </Button>
          </DialogActions>
        </Dialog>
      );
    }

    return (
      <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth onClick={(e) => e.stopPropagation()}>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Check size={24} color="green" />
          Clone Complete
        </DialogTitle>
        <DialogContent>
          <Box sx={{ py: 2 }}>
            <Typography variant="body1" gutterBottom>
              Cloned "{taskName}" to {cloneResult.total_cloned} project(s)
            </Typography>

            {cloneResult.cloned_tasks && cloneResult.cloned_tasks.length > 0 && (
              <Box sx={{ mt: 2 }}>
                <Typography variant="subtitle2" gutterBottom>
                  Created Tasks:
                </Typography>
                <List dense>
                  {cloneResult.cloned_tasks.map((task) => (
                    <ListItem
                      key={task.task_id}
                      onClick={() => handleTaskClick(task.task_id!, task.project_id!)}
                      sx={{
                        cursor: 'pointer',
                        borderRadius: 1,
                        '&:hover': { bgcolor: 'action.hover' },
                      }}
                    >
                      <ListItemIcon>
                        <Check size={16} color="green" />
                      </ListItemIcon>
                      <ListItemText
                        primary={task.project_name || task.project_id}
                        secondary={
                          <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Chip
                              label={task.status}
                              size="small"
                              color={task.status === 'started' ? 'primary' : 'default'}
                              sx={{ height: 20, fontSize: '0.7rem' }}
                            />
                          </Box>
                        }
                      />
                      <IconButton size="small">
                        <ExternalLink size={16} />
                      </IconButton>
                    </ListItem>
                  ))}
                </List>
              </Box>
            )}

            {cloneResult.errors && cloneResult.errors.length > 0 && (
              <Box sx={{ mt: 2 }}>
                <Alert severity="warning">
                  {cloneResult.total_failed} clone(s) failed
                </Alert>
                {cloneResult.errors.map((err, idx) => (
                  <Typography key={idx} variant="body2" color="error" sx={{ mt: 1 }}>
                    {err.project_id || err.repo_id}: {err.error}
                  </Typography>
                ))}
              </Box>
            )}

            {cloneResult.clone_group_id && (
              <Button
                variant="text"
                size="small"
                startIcon={<Copy size={14} />}
                onClick={() => setShowCloneProgress(true)}
                sx={{ mt: 2 }}
              >
                View Clone Group Progress
              </Button>
            )}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} variant="contained">
            Done
          </Button>
        </DialogActions>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth onClick={(e) => e.stopPropagation()}>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Copy size={24} />
        Clone Task to Projects
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Clone "{taskName}" with its specs and plan to other projects.
        </Typography>

        {/* Existing Projects Section */}
        <Box sx={{ mb: 2 }}>
          <Box
            onClick={() => setProjectsExpanded(!projectsExpanded)}
            sx={{
              display: 'flex',
              alignItems: 'center',
              cursor: 'pointer',
              py: 1,
            }}
          >
            <IconButton size="small">
              {projectsExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
            </IconButton>
            <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
              Existing Projects ({projectSearch ? `${filteredProjects.length} of ${availableProjects.length}` : availableProjects.length})
            </Typography>
          </Box>
          <Collapse in={projectsExpanded}>
            {loadingProjects ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
                <CircularProgress size={24} />
              </Box>
            ) : availableProjects.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ pl: 4 }}>
                No other projects available
              </Typography>
            ) : (
              <>
                <TextField
                  size="small"
                  placeholder="Search projects..."
                  value={projectSearch}
                  onChange={(e) => setProjectSearch(e.target.value)}
                  sx={{ ml: 4, mr: 2, mb: 1, width: 'calc(100% - 48px)' }}
                />
                <List dense sx={{ maxHeight: 200, overflow: 'auto' }}>
                  {filteredProjects.map((project: TypesProject) => (
                    <ListItem
                      key={project.id}
                      onClick={() => handleProjectToggle(project.id!)}
                      sx={{ cursor: 'pointer' }}
                    >
                      <Checkbox
                        checked={selectedProjects.includes(project.id!)}
                        size="small"
                      />
                      <ListItemText
                        primary={project.name}
                        secondary={project.description}
                      />
                    </ListItem>
                  ))}
                </List>
              </>
            )}
          </Collapse>
        </Box>

        <Divider />

        {/* Repos Without Projects Section */}
        <Box sx={{ mt: 2 }}>
          <Box
            onClick={() => setReposExpanded(!reposExpanded)}
            sx={{
              display: 'flex',
              alignItems: 'center',
              cursor: 'pointer',
              py: 1,
            }}
          >
            <IconButton size="small">
              {reposExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
            </IconButton>
            <FolderGit2 size={18} style={{ marginRight: 8 }} />
            <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
              Create New Projects ({repoSearch ? `${filteredRepos.length} of ${reposWithoutProjects?.length || 0}` : reposWithoutProjects?.length || 0})
            </Typography>
          </Box>
          <Collapse in={reposExpanded}>
            <Typography variant="caption" color="text.secondary" sx={{ pl: 4, display: 'block', mb: 1 }}>
              Repositories without projects - selecting will create a new project
            </Typography>
            {loadingRepos ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
                <CircularProgress size={24} />
              </Box>
            ) : !reposWithoutProjects || reposWithoutProjects.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ pl: 4 }}>
                All repositories have projects
              </Typography>
            ) : (
              <>
                <TextField
                  size="small"
                  placeholder="Search repositories..."
                  value={repoSearch}
                  onChange={(e) => setRepoSearch(e.target.value)}
                  sx={{ ml: 4, mr: 2, mb: 1, width: 'calc(100% - 48px)' }}
                />
                <List dense sx={{ maxHeight: 200, overflow: 'auto' }}>
                  {filteredRepos.map((repo: TypesGitRepository) => (
                    <ListItem
                      key={repo.id}
                      onClick={() => handleRepoToggle(repo.id!, repo.name!)}
                      sx={{ cursor: 'pointer' }}
                    >
                      <Checkbox
                        checked={selectedRepos.some(r => r.repo_id === repo.id)}
                        size="small"
                      />
                      <ListItemText
                        primary={repo.name}
                        secondary={repo.description}
                      />
                    </ListItem>
                  ))}
                </List>
              </>
            )}
          </Collapse>
        </Box>

        <Divider sx={{ my: 2 }} />

        {/* Options */}
        <FormControlLabel
          control={
            <Checkbox
              checked={autoStart}
              onChange={(e) => setAutoStart(e.target.checked)}
            />
          }
          label="Auto-start planning after clone"
        />

        {/* Error Alert */}
        {cloneError && (
          <Alert severity="error" sx={{ mt: 2 }} onClose={() => setCloneError(null)}>
            {cloneError}
          </Alert>
        )}

        {/* Selection Summary */}
        {totalTargets > 0 && (
          <Box sx={{ mt: 2, p: 2, bgcolor: 'action.hover', borderRadius: 1 }}>
            <Typography variant="body2">
              Will clone to <strong>{totalTargets}</strong> target(s):
            </Typography>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 1 }}>
              {selectedProjects.map(id => {
                const project = availableProjects.find((p: TypesProject) => p.id === id);
                return (
                  <Chip
                    key={id}
                    label={project?.name || id}
                    size="small"
                    onDelete={() => handleProjectToggle(id)}
                  />
                );
              })}
              {selectedRepos.map(repo => (
                <Chip
                  key={repo.repo_id}
                  label={`New: ${repo.name}`}
                  size="small"
                  color="primary"
                  variant="outlined"
                  onDelete={() => handleRepoToggle(repo.repo_id, repo.name || '')}
                />
              ))}
            </Box>
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button
          onClick={handleClone}
          variant="contained"
          disabled={totalTargets === 0 || cloneTaskMutation.isPending}
          startIcon={cloneTaskMutation.isPending ? <CircularProgress size={16} /> : <Copy size={16} />}
        >
          {cloneTaskMutation.isPending ? 'Cloning...' : `Clone to ${totalTargets} Target(s)`}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default CloneTaskDialog;
