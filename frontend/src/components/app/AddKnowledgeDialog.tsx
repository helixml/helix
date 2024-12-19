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
  const [error, setError] = useState('');

  const handleSubmit = () => {
    if (!name.trim()) {
      setError('Name is required');
      return;
    }

    const knowledgePath = `apps/${appId}/${name}`;

    const newSource: IKnowledgeSource = {
      id: '',
      name: name,
      source: sourceType === 'filestore'
        ? { filestore: { path: knowledgePath } }
        : {
            web: {
              urls: [],
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

    console.log(newSource);

    onAdd(newSource);
    handleClose();
  };

  const handleClose = () => {
    setName('');
    setError('');
    setSourceType('web');
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
        
        <TextField
          fullWidth
          label="Knowledge name"
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            setError('');
          }}
          error={!!error}
          helperText={error || (sourceType === 'filestore' ? `Files will be uploaded to: apps/${appId}/${name}` : '')}
          sx={{ mb: 2 }}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button onClick={handleSubmit} variant="contained">
          Add
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddKnowledgeDialog; 