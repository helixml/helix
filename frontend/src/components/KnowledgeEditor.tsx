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
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { IKnowledgeSource } from '../types';

interface KnowledgeEditorProps {
  knowledgeSources: IKnowledgeSource[];
  onUpdate: (updatedKnowledge: IKnowledgeSource[]) => void;
  disabled: boolean;
}

const KnowledgeEditor: FC<KnowledgeEditorProps> = ({ knowledgeSources, onUpdate, disabled }) => {
  const [expanded, setExpanded] = useState<string | false>(false);
  const [errors, setErrors] = useState<{ [key: number]: string }>({});

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

    newSources[index] = newSource;
    onUpdate(newSources);
  };

  const addNewSource = () => {
    const newSource: IKnowledgeSource = {
      source: { web: { urls: [], crawler: { enabled: false } } },
      refresh_schedule: '', // Empty string for one-off
    };
    onUpdate([...knowledgeSources, newSource]);
    setExpanded(`panel${knowledgeSources.length}`);
  };

  const deleteSource = (index: number) => {
    const newSources = knowledgeSources.filter((_, i) => i !== index);
    onUpdate(newSources);
  };

  const validateSources = () => {
    const newErrors: { [key: number]: string } = {};
    knowledgeSources.forEach((source, index) => {
      if (!source.source.web?.urls || source.source.web.urls.length === 0) {
        newErrors[index] = "At least one URL must be specified.";
      }
    });
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  useEffect(() => {
    validateSources();
  }, [knowledgeSources]);

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
            <Typography sx={{ flexGrow: 1 }}>Knowledge Source {index + 1}</Typography>
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
            <TextField
              fullWidth
              label="URLs (comma-separated)"
              value={source.source.web?.urls?.join(', ') || ''}
              onChange={(e) => {
                handleSourceUpdate(index, { 
                  source: { 
                    ...source.source, 
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