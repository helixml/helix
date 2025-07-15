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
import ScheduleSelector, { ScheduleType } from './ScheduleSelector';
import AgentSelector from './AgentSelector';
import { IApp } from '../../types'

interface TaskDialogProps {
  open: boolean;
  onClose: () => void;
  task?: any; // Will be properly typed when we implement the full dialog
  apps: IApp[]; // Agents
}

const TaskDialog: React.FC<TaskDialogProps> = ({ open, onClose, task, apps }) => {
  const [selectedSchedule, setSelectedSchedule] = useState<ScheduleType>('daily');
  const [selectedAgent, setSelectedAgent] = useState(apps[0]);
  const [time, setTime] = useState('23:20'); // 11:20 PM in 24-hour format
  const [prompt, setPrompt] = useState('');
  const [characterCount, setCharacterCount] = useState(0);
  const maxCharacters = 2000;

  const handlePromptChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const value = event.target.value;
    setPrompt(value);
    setCharacterCount(value.length);
  };

  const handleTimeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setTime(event.target.value);
  };

  const handleCreateTask = () => {
    // TODO: Implement task creation logic
    console.log('Creating task:', {
      schedule: selectedSchedule,
      agent: selectedAgent,
      time,
      prompt,
    });
    onClose();
  };

  const formatTimeForDisplay = (time24: string) => {
    const [hours, minutes] = time24.split(':');
    const hour = parseInt(hours);
    const ampm = hour >= 12 ? 'PM' : 'AM';
    const displayHour = hour === 0 ? 12 : hour > 12 ? hour - 12 : hour;
    return `${displayHour}:${minutes} ${ampm}`;
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
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'center',
        borderBottom: '1px solid #23262F'
      }}>
        <Typography variant="h6" component="div">
          Name of task
        </Typography>
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
          {/* Schedule Section */}
          <ScheduleSelector
            selectedSchedule={selectedSchedule}
            onScheduleChange={setSelectedSchedule}
          />

          {/* Time Section */}
          <Box>
            <Typography 
              variant="body2" 
              sx={{ 
                color: '#A0AEC0', 
                mb: 1,
                fontWeight: 500 
              }}
            >
              Time
            </Typography>
            <TextField
              type="time"
              value={time}
              onChange={handleTimeChange}
              sx={{
                '& .MuiOutlinedInput-root': {
                  color: '#F1F1F1',
                  '& fieldset': {
                    borderColor: '#4A5568',
                  },
                  '&:hover fieldset': {
                    borderColor: '#718096',
                  },
                  '&.Mui-focused fieldset': {
                    borderColor: '#3182CE',
                  },
                },
                '& .MuiInputLabel-root': {
                  color: '#A0AEC0',
                  '&.Mui-focused': {
                    color: '#3182CE',
                  },
                },
                '& input': {
                  color: '#F1F1F1',
                },
              }}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <Typography variant="body2" sx={{ color: '#A0AEC0' }}>
                      {formatTimeForDisplay(time)}
                    </Typography>
                  </InputAdornment>
                ),
              }}
            />
          </Box>          

          {/* Prompt Input */}
          <Box>
            <Typography 
              variant="body2" 
              sx={{ 
                color: '#A0AEC0', 
                mb: 1,
                fontWeight: 500 
              }}
            >
              Prompt
            </Typography>
            <TextField
              multiline
              rows={4}
              value={prompt}
              onChange={handlePromptChange}
              placeholder="Enter prompt here."
              sx={{
                width: '100%',
                '& .MuiOutlinedInput-root': {
                  color: '#F1F1F1',
                  '& fieldset': {
                    borderColor: '#4A5568',
                  },
                  '&:hover fieldset': {
                    borderColor: '#718096',
                  },
                  '&.Mui-focused fieldset': {
                    borderColor: '#3182CE',
                  },
                },
                '& .MuiInputBase-input': {
                  color: '#F1F1F1',
                  '&::placeholder': {
                    color: '#A0AEC0',
                    opacity: 1,
                  },
                },
              }}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end" sx={{ alignSelf: 'flex-end', mb: 1 }}>
                    <Typography 
                      variant="caption" 
                      sx={{ 
                        color: characterCount > maxCharacters ? '#EF4444' : '#A0AEC0' 
                      }}
                    >
                      {characterCount} / {maxCharacters}
                    </Typography>
                  </InputAdornment>
                ),
              }}
            />
          </Box>

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
        borderTop: '1px solid #23262F',
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
          variant="contained"
          onClick={handleCreateTask}
          sx={{
            backgroundColor: '#3182CE',
            color: '#FFFFFF',
            textTransform: 'none',
            '&:hover': {
              backgroundColor: '#2B6CB0',
            },
          }}
        >
          Create Task
        </Button>
      </DialogActions>
    </DarkDialog>
  );
};

export default TaskDialog; 