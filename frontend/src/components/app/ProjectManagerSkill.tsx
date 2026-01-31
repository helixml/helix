import React, { useState, useEffect } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Alert,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
} from '@mui/material';
import { IAppFlatState } from '../../types';
import { TypesAssistantProjectManager } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import { useListProjects } from '../../services/projectService';

interface ProjectManagerSkillProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

const NameTypography = styled(Typography)(({ theme }) => ({
  fontSize: '2rem',
  fontWeight: 700,
  color: '#F8FAFC',
  marginBottom: theme.spacing(1),
}));

const DescriptionTypography = styled(Typography)(({ theme }) => ({
  fontSize: '1.1rem',
  color: '#A0AEC0',
  marginBottom: theme.spacing(3),
}));

const SectionCard = styled(Box)(({ theme }) => ({
  background: '#23262F',
  borderRadius: 12,
  padding: theme.spacing(3),
  marginBottom: theme.spacing(3),
  boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
}));

const ProjectManagerSkill: React.FC<ProjectManagerSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [projectManagerConfig, setProjectManagerConfig] = useState<TypesAssistantProjectManager>({
    enabled: false,
    project_id: '',
  });
  const [selectedProjectId, setSelectedProjectId] = useState<string>('');
  const [isDirty, setIsDirty] = useState(false);

  const { data: projects, isLoading: projectsLoading } = useListProjects();

  useEffect(() => {
    if (app.projectManagerTool) {
      setProjectManagerConfig(app.projectManagerTool);
      setSelectedProjectId(app.projectManagerTool.project_id || '');
    } else {
      setProjectManagerConfig({
        enabled: false,
        project_id: '',
      });
      setSelectedProjectId('');
    }
    setIsDirty(false);
  }, [app.projectManagerTool]);

  const handleProjectChange = (projectId: string) => {
    setSelectedProjectId(projectId);
    setIsDirty(projectId !== (app.projectManagerTool?.project_id || ''));
  };

  const handleUpdate = async () => {
    if (!isDirty) {
      return;
    }

    try {
      setError(null);

      const appCopy = JSON.parse(JSON.stringify(app));

      appCopy.projectManagerTool.project_id = selectedProjectId;

      await onUpdate(appCopy);

      setProjectManagerConfig(appCopy.projectManagerTool);
      setIsDirty(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update project manager configuration');
    }
  };

  const handleEnable = async () => {
    try {
      setError(null);

      const appCopy = JSON.parse(JSON.stringify(app));

      const updatedConfig = {
        enabled: true,
        project_id: selectedProjectId,
      };

      appCopy.projectManagerTool = updatedConfig;

      await onUpdate(appCopy);

      setProjectManagerConfig(updatedConfig);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to enable project manager skill');
    }
  };

  const handleDisable = async () => {
    try {
      setError(null);
      const appCopy = JSON.parse(JSON.stringify(app));

      appCopy.projectManagerTool = {
        enabled: false,
        project_id: app.projectManagerTool?.project_id,
      };

      await onUpdate(appCopy);

      setProjectManagerConfig(appCopy.projectManagerTool);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable project manager skill');
    }
  };

  const handleClose = () => {
    onClose();
  };

  const selectedProject = projects?.find(p => p.id === selectedProjectId);

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      TransitionProps={{
        onExited: () => {
          if (app.projectManagerTool) {
            setProjectManagerConfig(app.projectManagerTool);
            setSelectedProjectId(app.projectManagerTool.project_id || '');
          } else {
            setProjectManagerConfig({
              enabled: false,
              project_id: '',
            });
            setSelectedProjectId('');
          }
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            Project Manager Skill
          </NameTypography>
          <DescriptionTypography>
            Enable the AI to manage Helix projects. The agent can create tasks, update project status, and coordinate work across your projects.
          </DescriptionTypography>

          <SectionCard>
            <Typography sx={{ color: '#F8FAFC', mb: 2 }}>
              Select Project (Optional)
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1, mb: 2 }}>
              Choose a specific project for the agent to manage. If no project is selected, the agent will determine the project from the conversation context.
            </Typography>
            
            <FormControl fullWidth variant="outlined">
              <InputLabel id="project-select-label" sx={{ color: '#A0AEC0' }}>
                Project
              </InputLabel>
              <Select
                labelId="project-select-label"
                value={selectedProjectId}
                onChange={(e) => handleProjectChange(e.target.value)}
                label="Project"
                disabled={projectsLoading}
                sx={{
                  color: '#F8FAFC',
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderColor: '#4A5568',
                  },
                  '&:hover .MuiOutlinedInput-notchedOutline': {
                    borderColor: '#6B7280',
                  },
                  '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
                    borderColor: '#6366F1',
                  },
                  '& .MuiSelect-icon': {
                    color: '#A0AEC0',
                  },
                }}
              >
                <MenuItem value="">
                  <em>None - Use context</em>
                </MenuItem>
                {projects?.map((project) => (
                  <MenuItem key={project.id} value={project.id}>
                    {project.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <Box sx={{ mt: 3 }}>
              {selectedProjectId ? (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Typography variant="body2" color="text.secondary">
                    Selected:
                  </Typography>
                  <Chip 
                    label={selectedProject?.name || selectedProjectId}
                    color="primary"
                    size="small"
                    sx={{ fontWeight: 500 }}
                  />
                </Box>
              ) : (
                <Alert severity="info" sx={{ bgcolor: 'rgba(99, 102, 241, 0.1)', border: '1px solid rgba(99, 102, 241, 0.3)' }}>
                  <Typography variant="body2">
                    No project selected. The agent will automatically determine the appropriate project based on the conversation context.
                  </Typography>
                </Alert>
              )}
            </Box>
          </SectionCard>
        </Box>
      </DialogContent>
      <DialogActions sx={{ background: '#181A20', borderTop: '1px solid #23262F', flexDirection: 'column', alignItems: 'stretch' }}>
        {error && (
          <Box sx={{ width: '100%', pl: 2, pr: 2, mb: 3 }}>
            <Alert variant="outlined" severity="error" sx={{ width: '100%' }}>
              {error}
            </Alert>
          </Box>
        )}
        <Box sx={{ display: 'flex', width: '100%' }}>
          <Button
            onClick={handleClose}
            size="small"
            variant="outlined"
            color="primary"
          >
            Cancel
          </Button>
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 1, mr: 2 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>
              {projectManagerConfig.enabled ? (
                <>
                  <Button
                    onClick={handleDisable}
                    size="small"
                    variant="outlined"
                    color="error"
                    sx={{ borderColor: '#EF4444', color: '#EF4444', '&:hover': { borderColor: '#DC2626', color: '#DC2626' } }}
                  >
                    Disable
                  </Button>
                  <Button
                    onClick={handleUpdate}
                    size="small"
                    color="secondary"
                    variant="outlined"
                    disabled={!isDirty}
                  >
                    Save
                  </Button>
                </>
              ) : (
                <Button
                  onClick={handleEnable}
                  size="small"
                  variant="outlined"
                  color="secondary"
                >
                  Enable
                </Button>
              )}
            </Box>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default ProjectManagerSkill;

