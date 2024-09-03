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

  const handleChange = (panel: string) => (event: React.SyntheticEvent, isExpanded: boolean) => {
    setExpanded(isExpanded ? panel : false);
  };

  const handleSourceUpdate = (index: number, updatedSource: Partial<IKnowledgeSource>) => {
    const newSources = [...knowledgeSources];
    newSources[index] = { ...newSources[index], ...updatedSource };
    onUpdate(newSources);
  };

  const addNewSource = () => {
    const newSource = {
      name: `New Source ${knowledgeSources.length + 1}`,
      rag_settings: { results_count: 5, chunk_size: 1024 },
      source: { web: { urls: [], crawler: { enabled: false } } },
    };
    onUpdate([...knowledgeSources, newSource]);
    setExpanded(`panel${knowledgeSources.length}`);
  };

  const deleteSource = (index: number) => {
    const newSources = knowledgeSources.filter((_, i) => i !== index);
    onUpdate(newSources);
  };

  useEffect(() => {
    if (knowledgeSources.length > 0) {
      setExpanded(`panel${knowledgeSources.length - 1}`);
    }
  }, [knowledgeSources.length]);

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
            <Typography sx={{ flexGrow: 1 }}>{source.name}</Typography>
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
              label="Name"
              value={source.name}

              onChange={(e) => handleSourceUpdate(index, { name: e.target.value })}
              disabled={disabled}
              sx={{ mb: 2 }}
            />
            <TextField
              fullWidth
              label="Results Count"
              type="number"
              value={source.rag_settings.results_count}
              onChange={(e) => handleSourceUpdate(index, { rag_settings: { ...source.rag_settings, results_count: parseInt(e.target.value) } })}
              disabled={disabled}
              sx={{ mb: 2 }}
            />
            <TextField
              fullWidth
              label="Chunk Size"
              type="number"
              value={source.rag_settings.chunk_size}
              onChange={(e) => handleSourceUpdate(index, { rag_settings: { ...source.rag_settings, chunk_size: parseInt(e.target.value) } })}
              disabled={disabled}
              sx={{ mb: 2 }}
            />
            <TextField
              fullWidth
              label="URLs (comma-separated)"
              value={source.source.web?.urls?.join(', ') || ''}
              onChange={(e) => handleSourceUpdate(index, { 
                source: { 
                  ...source.source, 
                  web: { 
                    ...source.source.web, 
                    urls: e.target.value.split(',').map(url => url.trim()) 
                  } 
                } 
              })}
              disabled={disabled}
              sx={{ mb: 2 }}
            />
            {/* Add more fields for other knowledge source settings */}
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
    </Box>
  );
};

export default KnowledgeEditor;