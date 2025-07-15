import React, { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  TextField,
  IconButton,
  InputAdornment,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import NotificationsIcon from '@mui/icons-material/Notifications';
import SearchIcon from '@mui/icons-material/Search';
import DarkDialog from '../dialog/DarkDialog';
import TriggerCron from '../app/TriggerCron';
import AgentSelector from './AgentSelector';
import { IApp } from '../../types'
import { TypesTrigger } from '../../api/api'

interface TaskDialogProps {
  open: boolean;
  onClose: () => void;
  task?: any; // Will be properly typed when we implement the full dialog
  apps: IApp[]; // Agents
}

const TaskDialog: React.FC<TaskDialogProps> = ({ open, onClose, task, apps }) => {
  const [selectedAgent, setSelectedAgent] = useState(apps[0]);
  const [prompt, setPrompt] = useState('');
  const [characterCount, setCharacterCount] = useState(0);
  const [triggers, setTriggers] = useState<TypesTrigger[]>([]);
  const [taskName, setTaskName] = useState(task?.name || '');
  const maxCharacters = 2000;

  // Update task name when task prop changes
  useEffect(() => {
    setTaskName(task?.name || '');
  }, [task]);

  const handleTriggersUpdate = (newTriggers: TypesTrigger[]) => {
    setTriggers(newTriggers);
  };

  const handleSaveTask = () => {
    if (!taskName.trim()) {
      // TODO: Show error message to user
      console.error('Task name is required');
      return;
    }
    
    // TODO: Implement task creation/update logic
    console.log(task ? 'Updating task:' : 'Creating task:', {
      name: taskName,
      triggers,
      agent: selectedAgent,
      prompt,
    });
    onClose();
  };

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
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
          onClick={onClose}
          sx={{ color: '#A0AEC0' }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3 }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {/* Schedule Section - Using TriggerCron component */}
          <TriggerCron
            triggers={triggers}
            onUpdate={handleTriggersUpdate}
            readOnly={false}
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
        >
          {task ? 'Update Task' : 'Create Task'}
        </Button>
      </DialogActions>
    </DarkDialog>
  );
};

export default TaskDialog; 