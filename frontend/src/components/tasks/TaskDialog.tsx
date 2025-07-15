import React, { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  TextField,  
  IconButton,
  Alert,
  CircularProgress,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DarkDialog from '../dialog/DarkDialog';
import TriggerCron from '../app/TriggerCron';
import AgentSelector from './AgentSelector';
import { IApp } from '../../types'
import { TypesTrigger, TypesTriggerConfiguration, TypesTriggerType, TypesOwnerType } from '../../api/api'

import { useCreateAppTrigger, useUpdateAppTrigger } from '../../services/appService';
import useAccount from '../../hooks/useAccount';
import useSnackbar from '../../hooks/useSnackbar';

interface TaskDialogProps {
  open: boolean;
  onClose: () => void;
  task?: TypesTriggerConfiguration;
  apps: IApp[]; // Agents
}

const TaskDialog: React.FC<TaskDialogProps> = ({ open, onClose, task, apps }) => {
  const account = useAccount();
  const snackbar = useSnackbar();
  
  const [selectedAgent, setSelectedAgent] = useState<IApp | undefined>(undefined);
  const [triggers, setTriggers] = useState<TypesTrigger[]>([]);
  const [taskName, setTaskName] = useState(task?.name || '');
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Initialize selected agent when apps are available
  useEffect(() => {
    if (apps.length > 0 && !selectedAgent) {
      setSelectedAgent(apps[0]);
    }
  }, [apps, selectedAgent]);

  // Update task name when task prop changes
  useEffect(() => {
    setTaskName(task?.name || '');
    setError(null);
  }, [task]);

  // Initialize triggers from existing task
  useEffect(() => {
    if (task?.trigger) {
      setTriggers([task.trigger]);
    } else {
      setTriggers([]);
    }
  }, [task]);

  const handleTriggersUpdate = (newTriggers: TypesTrigger[]) => {
    setTriggers(newTriggers);
  };

  const createTriggerMutation = useCreateAppTrigger(selectedAgent?.id || '', account.organizationTools.organization?.id || '');
  const updateTriggerMutation = useUpdateAppTrigger(selectedAgent?.id || '', task?.id || '', account.organizationTools.organization?.id || '');

  const handleSaveTask = async () => {
    if (!taskName.trim()) {
      setError('Task name is required');
      return;
    }

    if (!selectedAgent) {
      setError('Please select an agent');
      return;
    }

    // Find the cron trigger from the triggers array
    const cronTrigger = triggers.find(t => t.cron);
    if (!cronTrigger?.cron?.enabled) {
      setError('Please configure a schedule for the task');
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const triggerConfig: TypesTriggerConfiguration = {
        name: taskName.trim(),
        app_id: selectedAgent.id,
        organization_id: account.organizationTools.organization?.id || '',
        owner: account.user?.id || '',
        owner_type: account.organizationTools.organization ? TypesOwnerType.OwnerTypeSystem : TypesOwnerType.OwnerTypeUser,
        enabled: true,
        trigger_type: TypesTriggerType.TriggerTypeCron,
        trigger: cronTrigger,
        description: `Scheduled task: ${taskName.trim()}`,
      };

      if (task?.id) {
        // Update existing task
        await updateTriggerMutation.mutateAsync(triggerConfig);
        snackbar.success('Task updated successfully');
      } else {
        // Create new task
        await createTriggerMutation.mutateAsync(triggerConfig);
        snackbar.success('Task created successfully');
      }

      onClose();
    } catch (err) {
      console.error('Error saving task:', err);
      setError(err instanceof Error ? err.message : 'Failed to save task');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    if (!isSubmitting) {
      setError(null);
      onClose();
    }
  };

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          maxHeight: '90vh',
        },
      }}
    >
      <DialogTitle sx={{ 
        m: 0, 
        p: 2, 
        ml: 1,
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'center',
        
      }}>
        <TextField
          value={taskName}
          onChange={(e) => setTaskName(e.target.value)}
          placeholder="Enter task name"
          variant="standard"
          disabled={isSubmitting}
          sx={{
            '& .MuiInputBase-root': {
              fontSize: '1.25rem',
              fontWeight: 500,
              color: '#F1F1F1'
            },
            '& .MuiInputBase-input': {
              padding: 0
            },
            '& .MuiInput-underline:before': {
              borderBottom: 'none'
            },
            '& .MuiInput-underline:after': {
              borderBottom: 'none'
            },
            '& .MuiInput-underline:hover:not(.Mui-disabled):before': {
              borderBottom: 'none'
            }
          }}
        />
        <IconButton
          aria-label="close"
          onClick={handleClose}
          disabled={isSubmitting}
          sx={{ color: '#A0AEC0' }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3 }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {/* Error Alert */}
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}

          {/* Schedule Section - Using TriggerCron component */}
          <TriggerCron
            triggers={triggers}
            onUpdate={handleTriggersUpdate}
            readOnly={isSubmitting}
            showTitle={false}
            showToggle={false}
            defaultEnabled={true}
          />          

          {/* TODO: Task Limit Display */}
          {/* <Box sx={{ 
            display: 'flex', 
            alignItems: 'center', 
            gap: 1,
            p: 2,
            backgroundColor: '#23262F',
            borderRadius: 1,
          }}>
            <Box sx={{ 
              width: 8, 
              height: 8, 
              borderRadius: '50%', 
              backgroundColor: '#10B981' 
            }} />
            <Box>
              <Typography variant="body2" sx={{ color: '#F1F1F1' }}>
                2 daily tasks remaining
              </Typography>
              <Typography variant="caption" sx={{ color: '#A0AEC0' }}>
                Current: 0/2 daily tasks active
              </Typography>
            </Box>
          </Box> */}
        </Box>
      </DialogContent>

      <DialogActions sx={{ 
        p: 3,         
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }}>
        <Box sx={{ display: 'flex', gap: 1 }}>
          {/* Agent Selector */}
          <AgentSelector
            apps={apps}
            selectedAgent={selectedAgent}
            onAgentSelect={setSelectedAgent}
          />  
        </Box>
        <Button
          variant="outlined"
          onClick={handleSaveTask}
          color="secondary"
          disabled={isSubmitting || !taskName.trim() || !selectedAgent || !triggers[0].cron?.input}
          startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
        >
          {isSubmitting ? 'Saving...' : (task ? 'Update Task' : 'Create Task')}
        </Button>
      </DialogActions>
    </DarkDialog>
  );
};

export default TaskDialog; 