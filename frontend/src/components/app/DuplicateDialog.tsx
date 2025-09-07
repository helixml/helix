import React, { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  Typography,
  IconButton,
  CircularProgress,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';

import DarkDialog from '../dialog/DarkDialog';
import { IApp } from '../../types';
import { useDuplicateApp } from '../../services/appService';
import useSnackbar from '../../hooks/useSnackbar';
import { getAppName } from '../../utils/apps';

interface DuplicateDialogProps {
  open: boolean;
  onClose: () => void;
  app: IApp | null;
  orgId: string;
}

const DuplicateDialog: React.FC<DuplicateDialogProps> = ({
  open,
  onClose,
  app,
  orgId,
}) => {
  const [newName, setNewName] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const snackbar = useSnackbar();
  const duplicateApp = useDuplicateApp(orgId);

  // Reset form when dialog opens/closes
  useEffect(() => {
    if (open && app) {
      const appName = getAppName(app) || 'Untitled Agent';
      setNewName(`${appName} (Copy)`);
    } else {
      setNewName('');
    }
  }, [open, app]);

  const handleClose = () => {
    if (!isSubmitting) {
      onClose();
    }
  };

  const handleSubmit = async () => {
    if (!newName.trim()) {
      snackbar.error('Please enter a name for the duplicated agent');
      return;
    }

    if (!app) {
      snackbar.error('Please select an agent to duplicate');
      return;
    }

    setIsSubmitting(true);
    try {
      await duplicateApp.mutateAsync({
        appId: app.id,
        name: newName.trim(),
      });
      
      snackbar.success('App duplicated successfully');
      onClose();
    } catch (error) {
      console.error('Failed to duplicate app:', error);
      snackbar.error('Failed to duplicate app. Please try again.');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleKeyPress = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter' && !isSubmitting) {
      handleSubmit();
    }
  };

  console.log('dialog', app)

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
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
        alignItems: 'center' 
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="h6" component="div">
            Duplicate Agent
          </Typography>
        </Box>
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
        <Box sx={{ mb: 2 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Duplicating: <strong>{app ? getAppName(app) || 'Untitled Agent' : 'Untitled Agent'}</strong>
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Enter a new name for the duplicated agent.
          </Typography>
        </Box>

        <TextField
          fullWidth
          label="New App Name"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          onKeyPress={handleKeyPress}
          disabled={isSubmitting}
          variant="outlined"
          required
          autoFocus
          sx={{
            '& .MuiOutlinedInput-root': {
              '& fieldset': {
                borderColor: '#555',
              },
              '&:hover fieldset': {
                borderColor: '#777',
              },
              '&.Mui-focused fieldset': {
                borderColor: '#00c8ff',
              },
            },
            '& .MuiInputLabel-root': {
              color: '#A0AEC0',
            },
            '& .MuiInputLabel-root.Mui-focused': {
              color: '#00c8ff',
            },
          }}
        />
      </DialogContent>

      <DialogActions sx={{ p: 2, gap: 1 }}>
        <Button
          onClick={handleClose}
          disabled={isSubmitting}
          sx={{
            color: '#A0AEC0',
            '&:hover': {
              backgroundColor: 'rgba(160, 174, 192, 0.1)',
            },
          }}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={!newName.trim() || isSubmitting}
          variant="contained"
          startIcon={isSubmitting ? <CircularProgress size={16} /> : <></>}          
          color="secondary"
          // sx={{
          //   backgroundColor: '#00c8ff',
          //   color: '#000',
          //   fontWeight: 600,
          //   '&:hover': {
          //     backgroundColor: '#00b4e6',
          //   },
          //   '&:disabled': {
          //     backgroundColor: '#555',
          //     color: '#888',
          //   },
          // }}
        >
          {isSubmitting ? 'Duplicating...' : 'Duplicate'}
        </Button>
      </DialogActions>
    </DarkDialog>
  );
};

export default DuplicateDialog;
