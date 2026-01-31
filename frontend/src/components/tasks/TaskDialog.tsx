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
import ExecutionsHistory from './ExecutionsHistory';
import { IApp } from '../../types'
import { TypesTrigger, TypesTriggerConfiguration, TypesTriggerType, TypesOwnerType } from '../../api/api'

import { useCreateAppTrigger, useUpdateAppTrigger, useExecuteAppTrigger } from '../../services/appService';
import useAccount from '../../hooks/useAccount';
import useSnackbar from '../../hooks/useSnackbar';

import { generateAmusingName } from '../../utils/names';

interface TaskDialogProps {
  open: boolean;
  onClose: () => void;
  task?: TypesTriggerConfiguration;
  apps: IApp[]; // Agents
  prepopulatedData?: {
    name: string;
    schedule: string;
    input: string;
  };
}

const TaskDialog: React.FC<TaskDialogProps> = ({ open, onClose, task, apps, prepopulatedData }) => {
  const account = useAccount();
  const snackbar = useSnackbar();
  
  const [selectedAgent, setSelectedAgent] = useState<IApp | undefined>(undefined);
  const [triggers, setTriggers] = useState<TypesTrigger[]>([]);
  const [taskName, setTaskName] = useState(task?.name || '');
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [createdTaskId, setCreatedTaskId] = useState<string | undefined>(task?.id);

  // Get the current task ID for fetching executions
  const currentTaskId = createdTaskId || task?.id;

  // Initialize selected agent when apps are available
  useEffect(() => {
    if (apps.length > 0 && !selectedAgent) {
      // If editing an existing task, find the associated agent
      if (task?.app_id) {
        const associatedAgent = apps.find(app => app.id === task.app_id);
        if (associatedAgent) {
          setSelectedAgent(associatedAgent);
        } else {
          // Fallback to first app if associated agent not found
          setSelectedAgent(apps[0]);
        }
      } else {
        // For new tasks, select the first agent
        setSelectedAgent(apps[0]);
      }
    }
  }, [apps, task?.app_id]);

  // Update selected agent when task changes (for editing existing tasks)
  useEffect(() => {
    if (task?.app_id && apps.length > 0) {
      const associatedAgent = apps.find(app => app.id === task.app_id);
      if (associatedAgent) {
        setSelectedAgent(associatedAgent);
      }
    }
  }, [task?.app_id, apps]);

  // Update task name when task prop changes
  useEffect(() => {
    setTaskName(task?.name || '');
    setError(null);
  }, [task]);

  // Initialize triggers from existing task or prepopulated data
  useEffect(() => {
    if (task?.trigger) {
      setTriggers([task.trigger]);
    } else if (prepopulatedData) {
      // Use prepopulated data for new tasks
      setTriggers([{
        cron: {
          enabled: true,
          schedule: prepopulatedData.schedule,
          input: prepopulatedData.input
        }
      }]);
      setTaskName(prepopulatedData.name);
    } else {
      // For new tasks without prepopulated data, create a default trigger structure
      // This ensures the TriggerCron component has a proper initial state
      const userTz = Intl.DateTimeFormat().resolvedOptions().timeZone;
      const defaultSchedule = `CRON_TZ=${userTz} 0 9 * * 1`; // Monday 9 AM
      setTriggers([{
        cron: {
          enabled: true,
          schedule: defaultSchedule,
          input: ''
        }
      }]);
      
      // Generate an amusing name for new tasks
      if (!task?.id && !taskName) {
        setTaskName(generateAmusingName());
      }
    }
  }, [task, prepopulatedData]);

  const handleTriggersUpdate = (newTriggers: TypesTrigger[]) => {
    setTriggers(newTriggers);
  };

  const createTriggerMutation = useCreateAppTrigger(account.organizationTools.organization?.id || '');
  const updateTriggerMutation = useUpdateAppTrigger(task?.id || '', account.organizationTools.organization?.id || '');
  const executeTriggerMutation = useExecuteAppTrigger(createdTaskId || task?.id || '');

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
      };

      if (task?.id) {
        // Update existing task
        const updatedTask = await updateTriggerMutation.mutateAsync(triggerConfig);
        setCreatedTaskId(updatedTask.data?.id);
        snackbar.success('Task updated successfully');
      } else {
        // Create new task
        const newTask = await createTriggerMutation.mutateAsync(triggerConfig);
        setCreatedTaskId(newTask.data?.id);
        snackbar.success('Task created successfully');
      }

      // Don't close the dialog - let user test the task
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

  // Execute task and view response. Can only run on triggers that are already created
  const handleExecuteTask = async () => {
    const taskId = createdTaskId || task?.id;
    if (!taskId) {
      setError('Task not found');
      return;
    }

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

    setIsTesting(true);
    setError(null);

    try {
      // Update the task before executing
      const triggerConfig: TypesTriggerConfiguration = {
        name: taskName.trim(),
        app_id: selectedAgent.id,
        organization_id: account.organizationTools.organization?.id || '',
        owner: account.user?.id || '',
        owner_type: account.organizationTools.organization ? TypesOwnerType.OwnerTypeSystem : TypesOwnerType.OwnerTypeUser,
        enabled: true,
        trigger_type: TypesTriggerType.TriggerTypeCron,
        trigger: cronTrigger,
      };

      // Only update if task exists (should always be the case for execute)
      if (task?.id) {
        await updateTriggerMutation.mutateAsync(triggerConfig);
      }

      await executeTriggerMutation.mutateAsync();
      snackbar.success('Task executed successfully');
    } catch (err) {
      console.error('Error updating or executing task:', err);
      setError(err instanceof Error ? err.message : 'Failed to update or execute task');
    } finally {
      setIsTesting(false);
    }
  }



  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth={currentTaskId ? "lg" : "md"}
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
            minWidth: '200px',
            width: 'auto',
            '& .MuiInputBase-root': {
              fontSize: '1.25rem',
              fontWeight: 500,
              color: '#F1F1F1'
            },
            '& .MuiInputBase-input': {
              padding: 0,
              width: 'auto',
              minWidth: '200px'
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
          inputProps={{
            style: {
              width: `${Math.max(200, taskName.length * 12)}px`
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
        <Box sx={{ display: 'flex', gap: 3 }}>
          {/* Left side - Task configuration */}
          <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 3 }}>
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
              showBorder={false}
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

          {/* Right side - Trigger executions history */}
          {currentTaskId && (
            <ExecutionsHistory 
              taskId={currentTaskId}
              taskName={taskName}
            />
          )}
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
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            variant="outlined"
            onClick={handleExecuteTask}
            color="primary"
            disabled={(!createdTaskId && !task?.id) || isTesting}
            startIcon={isTesting ? <CircularProgress size={16} /> : undefined}
          >
            {isTesting ? 'Testing...' : 'Test'}
          </Button>
          <Button
            variant="outlined"
            onClick={handleSaveTask}
            color="secondary"
            disabled={isSubmitting || !taskName.trim() || !selectedAgent || !triggers[0].cron?.input}
            startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
          >
            {isSubmitting ? 'Saving...' : (task ? 'Save' : 'Create')}
          </Button>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default TaskDialog; 