import React, { FC, useState, useEffect } from 'react';
import {
  Box,
  Button,
  TextField,
  Typography,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  IconButton,
  Alert,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  RadioGroup,
  FormControlLabel,
  Radio,
  Chip,
  Snackbar,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import { IKnowledgeSource } from '../types';
import MuiAlert, { AlertProps } from '@mui/material/Alert';
import useSnackbar from '../hooks/useSnackbar'; // Import the useSnackbar hook

interface KnowledgeEditorProps {
  knowledgeSources: IKnowledgeSource[];
  onUpdate: (updatedKnowledge: IKnowledgeSource[]) => void;
  onRefresh: (id: string) => void;
  disabled: boolean;
  knowledgeList: IKnowledgeSource[];
}

const KnowledgeEditor: FC<KnowledgeEditorProps> = ({ knowledgeSources, onUpdate, onRefresh, disabled, knowledgeList }) => {
  const [expanded, setExpanded] = useState<string | false>(false);
  const [errors, setErrors] = useState<{ [key: number]: string }>({});
  const snackbar = useSnackbar(); // Use the snackbar hook

  const handleChange = (panel: string) => (event: React.SyntheticEvent, isExpanded: boolean) => {
    setExpanded(isExpanded ? panel : false);
  };

  const handleSourceUpdate = (index: number, updatedSource: Partial<IKnowledgeSource>) => {
    const newSources = [...knowledgeSources];
    let newSource = { ...newSources[index], ...updatedSource };

    // Ensure refresh_schedule is always a valid cron expression or empty string
    if (newSource.refresh_schedule === 'custom') {
      newSource.refresh_schedule = '0 0 * * *'; // Default to daily at midnight
    } else if (newSource.refresh_schedule === 'One off') {
      newSource.refresh_schedule = ''; // Empty string for one-off
    }

    // Update the name based on the source
    if (newSource.source.web?.urls && newSource.source.web.urls.length > 0) {
      newSource.name = newSource.source.web.urls.join(', ');
    } else if (newSource.source.filestore?.path) {
      newSource.name = newSource.source.filestore.path;
    } else {
      newSource.name = 'Unnamed Source';
    }

    newSources[index] = newSource;
    onUpdate(newSources);
  };

  const addNewSource = () => {
    const newSource: IKnowledgeSource = {
        id: '',
        source: { web: { urls: [], crawler: { enabled: true } } },
        refresh_schedule: '',
        name: '',
        state: '',
        rag_settings: {
            results_count: 0,
            chunk_size: 0
        },
    };
    onUpdate([...knowledgeSources, newSource]);
    setExpanded(`panel${knowledgeSources.length}`);
  };

  const deleteSource = (index: number) => {
    const newSources = knowledgeSources.filter((_, i) => i !== index);
    onUpdate(newSources);
  };

  const refreshSource = (index: number) => {
    // Find ID of knowledge source
    const knowledge = knowledgeList.find(k => k.name === knowledgeSources[index].name);
    if (knowledge) {
      onRefresh(knowledge.id);
      // Show success message using snackbar
      snackbar.success('Knowledge refresh initiated. This may take a few minutes.');
    }
  };

  const validateSources = () => {
    const newErrors: { [key: number]: string } = {};
    knowledgeSources.forEach((source, index) => {      
      if ((!source.source.web?.urls || source.source.web.urls.length === 0) && !source.source.filestore?.path) {
        newErrors[index] = "At least one URL or a filestore path must be specified.";
      }
    });
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  useEffect(() => {
    validateSources();
  }, [knowledgeSources]);

  const getSourcePreview = (source: IKnowledgeSource): string => {
    if (source.source.web?.urls && source.source.web.urls.length > 0) {
      try {
        const url = new URL(source.source.web.urls[0]);
        return url.hostname;
      } catch {
        return source.source.web.urls[0];
      }    
    } else if (source.source.filestore?.path) {
      return source.source.filestore.path;
    }
    return 'No source specified';
  };

  const getKnowledgeState = (source: IKnowledgeSource): string | undefined => {
    const knowledge = knowledgeList.find(k => k.name === source.name);
    return knowledge?.state;
  };

  const renderKnowledgeState = (state: string | undefined) => {
    if (!state) return null;
    
    let color: "default" | "primary" | "secondary" | "error" | "info" | "success" | "warning" = "default";
    switch (state.toLowerCase()) {
      case 'ready':
        color = 'success';
        break;
      case 'pending':
        color = 'info';
        break;
      case 'indexing':
        color = 'info';
        break;
      case 'error':
        color = 'error';
        break;
      // Add more cases as needed
    }

    return <Chip label={state} color={color} size="small" sx={{ ml: 1 }} />;
  };

  const renderSourceInput = (source: IKnowledgeSource, index: number) => {
    const sourceType = source.source.filestore ? 'filestore' : 'web';

    return (
      <>
        <FormControl component="fieldset" sx={{ mb: 2 }}>
          <RadioGroup
            row
            value={sourceType}
            onChange={(e) => {
              const newSourceType = e.target.value;
              let newSource: Partial<IKnowledgeSource> = {
                source: newSourceType === 'filestore'
                  ? { filestore: { path: '' } }
                  : { web: { urls: [], crawler: { enabled: false } } }
              };
              handleSourceUpdate(index, newSource);
            }}
          >
            <FormControlLabel value="filestore" control={<Radio />} label="Helix Filestore" />
            <FormControlLabel value="web" control={<Radio />} label="Web" />
          </RadioGroup>
        </FormControl>

        {sourceType === 'filestore' ? (
          <TextField
            fullWidth
            label="Filestore Path"
            value={source.source.filestore?.path || ''}
            onChange={(e) => {
              handleSourceUpdate(index, { 
                source: { 
                  filestore: { path: e.target.value } 
                } 
              });
            }}
            disabled={disabled}
            sx={{ mb: 2 }}
          />
        ) : (
          <TextField
            fullWidth
            label="URLs (comma-separated)"
            value={source.source.web?.urls?.join(', ') || ''}
            onChange={(e) => {
              handleSourceUpdate(index, { 
                source: { 
                  web: { 
                    ...source.source.web, 
                    urls: e.target.value.split(',').map(url => url.trim()) 
                  } 
                } 
              });
            }}
            disabled={disabled}
            sx={{ mb: 2 }}
            error={!!errors[index]}
            helperText={errors[index]}
          />
        )}
      </>
    );
  };

  return (
    <Box>
      {knowledgeSources.map((source, index) => (
        <Accordion
          key={index}
          expanded={expanded === `panel${index}`}
          onChange={handleChange(`panel${index}`)}
        >
          <AccordionSummary 
            expandIcon={<ExpandMoreIcon />}
            sx={{ display: 'flex', alignItems: 'center' }}
          >
            <Typography sx={{ flexGrow: 1 }}>
              Knowledge Source ({getSourcePreview(source)})
              {renderKnowledgeState(getKnowledgeState(source))}
            </Typography>
            <IconButton
              onClick={(e) => {
                e.stopPropagation();
                refreshSource(index);
              }}
              disabled={disabled}
              sx={{ mr: 1 }}
            >
              <RefreshIcon />
            </IconButton>
            <IconButton
              onClick={(e) => {
                e.stopPropagation();
                deleteSource(index);
              }}
              disabled={disabled}
              sx={{ mr: 1 }}
            >
              <DeleteIcon />
            </IconButton>
          </AccordionSummary>
          <AccordionDetails>
            {renderSourceInput(source, index)}
            <FormControl fullWidth sx={{ mb: 2 }}>
              <InputLabel>Scrape Interval</InputLabel>
              <Select
                value={source.refresh_schedule === '' ? 'One off' : 
                       (source.refresh_schedule === '@hourly' || source.refresh_schedule === '@daily' ? source.refresh_schedule : 'custom')}
                onChange={(e) => {
                  let newSchedule = e.target.value;
                  if (newSchedule === 'One off') newSchedule = '';
                  if (newSchedule === 'custom') newSchedule = '0 0 * * *';
                  handleSourceUpdate(index, { refresh_schedule: newSchedule });
                }}
                disabled={disabled}
              >
                <MenuItem value="One off">One off</MenuItem>
                <MenuItem value="@hourly">Hourly</MenuItem>
                <MenuItem value="@daily">Daily</MenuItem>
                <MenuItem value="custom">Custom (cron)</MenuItem>
              </Select>
            </FormControl>
            {source.refresh_schedule !== '' && source.refresh_schedule !== '@hourly' && source.refresh_schedule !== '@daily' && (
              <TextField
                fullWidth
                label="Custom Cron Schedule"
                value={source.refresh_schedule}
                onChange={(e) => handleSourceUpdate(index, { refresh_schedule: e.target.value })}
                disabled={disabled}
                sx={{ mb: 2 }}
                helperText="Enter a valid cron expression (default: daily at midnight)"
              />
            )}
          </AccordionDetails>
        </Accordion>
      ))}
      <Button
        variant="outlined"
        startIcon={<AddIcon />}
        onClick={addNewSource}
        disabled={disabled}
        sx={{ mt: 2 }}
      >
        Add Knowledge Source
      </Button>
      {Object.keys(errors).length > 0 && (
        <Alert severity="error" sx={{ mt: 2 }}>
          Please specify at least one URL for each knowledge source.
        </Alert>
      )}
    </Box>
  );
};

export default KnowledgeEditor;