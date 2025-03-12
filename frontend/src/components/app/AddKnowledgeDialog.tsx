import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  RadioGroup,
  FormControlLabel,
  Radio,
  TextField,
  FormControl,
  FormHelperText,
  CircularProgress,
} from '@mui/material';
import { IKnowledgeSource } from '../../types';

interface AddKnowledgeDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (source: IKnowledgeSource) => void;
  appId: string;
}

const AddKnowledgeDialog: React.FC<AddKnowledgeDialogProps> = ({
  open,
  onClose,
  onAdd,
  appId,
}) => {
  const [sourceType, setSourceType] = useState<'web' | 'filestore'>('web');
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = () => {
    if (!name.trim()) {
      setError('Name is required');
      return;
    }

    if (sourceType === 'web' && !url.trim()) {
      setError('URL is required for web sources');
      return;
    }

    setIsLoading(true);

    const knowledgePath = sourceType === 'filestore' ? name : name;

    const newSource: IKnowledgeSource = {
      id: '',
      name: name,
      source: sourceType === 'filestore'
        ? { filestore: { path: knowledgePath } }
        : {
            web: {
              urls: [url],
              crawler: {
                enabled: true,
                max_depth: 1,
                max_pages: 5,
                readability: true
              }
            }
          },
      refresh_schedule: '',
      version: '',
      state: '',
      rag_settings: {
        results_count: 0,
        chunk_size: 0,
        chunk_overflow: 0,
      },
    };

    onAdd(newSource);
    
    // Adding a small delay to show the loading indicator
    // The parent component should handle closing this dialog after processing is complete
    setTimeout(() => {
      setIsLoading(false);
      handleClose();
    }, 500);
  };

  const handleClose = () => {
    setName('');
    setUrl('');
    setError('');
    setSourceType('web');
    setIsLoading(false);
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Add Knowledge Source</DialogTitle>
      <DialogContent>
        <FormControl component="fieldset" sx={{ mt: 2, mb: 2 }}>
          <RadioGroup
            row
            value={sourceType}
            onChange={(e) => setSourceType(e.target.value as 'web' | 'filestore')}
          >
            <FormControlLabel value="web" control={<Radio />} label="Web" />
            <FormControlLabel value="filestore" control={<Radio />} label="Files" />
          </RadioGroup>
        </FormControl>
        
        {sourceType === 'web' && (
          <TextField
            fullWidth
            label="URLs (comma-separated)"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              setError('');
            }}
            error={!!error && !url.trim()}
            helperText={error && !url.trim() ? 'URL is required' : ''}
            sx={{ mb: 2 }}
          />
        )}

        <TextField
          fullWidth
          label="Knowledge name"
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            setError('');
          }}
          error={!!error}
          helperText={error || (sourceType === 'filestore' ? `Files will be uploaded to the '${name}' folder in this app` : '')}
          sx={{ mb: 2 }}
        />

      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isLoading}>Cancel</Button>
        <Button 
          onClick={handleSubmit} 
          variant="contained" 
          disabled={isLoading}
          startIcon={isLoading ? <CircularProgress size={20} /> : null}
        >
          {isLoading ? 'Adding...' : 'Add'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddKnowledgeDialog; 