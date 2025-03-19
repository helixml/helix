import React, { FC, useState, useEffect } from 'react';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import FormControlLabel from '@mui/material/FormControlLabel';
import Tooltip from '@mui/material/Tooltip';
import Switch from '@mui/material/Switch';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';

import { IKnowledgeSource } from '../../types';
import { default_max_depth } from '../../hooks/useKnowledge';

interface KnowledgeSourceInputsProps {
  knowledge: IKnowledgeSource;
  updateKnowledge: (id: string, updates: Partial<IKnowledgeSource>) => void;
  disabled: boolean;
  errors: {[key: string]: string[]};
  onCompletePreparation: (id: string) => void;
}

const KnowledgeSourceInputs: FC<KnowledgeSourceInputsProps> = ({
  knowledge,
  updateKnowledge,
  disabled,
  errors,
  onCompletePreparation
}) => {
  // Local state for all input fields
  const [urls, setUrls] = useState<string>('');
  const [description, setDescription] = useState<string>('');
  const [resultsCount, setResultsCount] = useState<number>(0);
  const [chunkSize, setChunkSize] = useState<number | undefined>(undefined);
  const [chunkOverflow, setChunkOverflow] = useState<number>(0);
  const [maxDepth, setMaxDepth] = useState<number>(default_max_depth);
  const [readability, setReadability] = useState<boolean>(true);
  const [refreshSchedule, setRefreshSchedule] = useState<string>('');
  const [customSchedule, setCustomSchedule] = useState<string>('');

  // Initialize local state from props
  useEffect(() => {
    setUrls(knowledge.source.web?.urls?.join(', ') || '');
    setDescription(knowledge.description || '');
    setResultsCount(knowledge.rag_settings.results_count);
    setChunkSize(knowledge.rag_settings.chunk_size || undefined);
    setChunkOverflow(knowledge.rag_settings.chunk_overflow);
    setMaxDepth(knowledge.source.web?.crawler?.max_depth || default_max_depth);
    setReadability(knowledge.source.web?.crawler?.readability ?? true);
    setRefreshSchedule(knowledge.refresh_schedule === '' ? 'One off' : 
                       (knowledge.refresh_schedule === '@hourly' || knowledge.refresh_schedule === '@daily' ? knowledge.refresh_schedule : 'custom'));
    
    // Handle the case where refresh_schedule might be undefined
    const schedule = knowledge.refresh_schedule || '';
    const isCustomSchedule = schedule !== '' && 
                       schedule !== '@hourly' && 
                       schedule !== '@daily';
    
    setCustomSchedule(isCustomSchedule ? schedule : '0 0 * * *');
  }, [knowledge]);

  // Source type determination
  const sourceType = knowledge.source.filestore ? 'filestore' : 'web';

  return (
    <>
      {sourceType === 'web' && (
        <TextField
          fullWidth
          label="URLs (comma-separated)"
          value={urls}
          onChange={(e) => setUrls(e.target.value)}
          onBlur={() => {
            updateKnowledge(knowledge.id, {
              source: { 
                web: { 
                  ...knowledge.source.web, 
                  urls: urls.split(',').map(url => url.trim()) 
                } 
              }
            });
          }}
          disabled={disabled}
          sx={{ mb: 2 }}
          error={!!errors[`${knowledge.id}`]}
          helperText={errors[`${knowledge.id}`]?.join(', ')}
        />
      )}

      <TextField
        fullWidth
        label="Description"
        multiline
        rows={2}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        onBlur={() => {
          updateKnowledge(knowledge.id, {
            description: description
          });
        }}
        disabled={disabled}
        sx={{ mb: 2 }}
        placeholder="Description for this knowledge source. This will be used by the agent to search for relevant information."
      />

      <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
        <TextField
          fullWidth
          label="Results Count (optional)"
          type="number"
          value={resultsCount}
          onChange={(e) => setResultsCount(parseInt(e.target.value) || 0)}
          onBlur={() => {
            updateKnowledge(knowledge.id, {
              rag_settings: {
                ...knowledge.rag_settings,
                results_count: resultsCount
              }
            });
          }}
          disabled={disabled}
        />
        <TextField
          fullWidth
          label="Chunk Size (optional)"
          type="number"              
          value={chunkSize === undefined ? '' : chunkSize}
          onChange={(e) => {
            const value = e.target.value ? parseInt(e.target.value) : undefined;
            setChunkSize(value);
          }}
          onBlur={() => {
            updateKnowledge(knowledge.id, {
              rag_settings: {
                ...knowledge.rag_settings,
                chunk_size: chunkSize ?? 0
              }
            });
          }}
          disabled={disabled}
        />
        <TextField
          fullWidth
          label="Chunk Overflow (optional)"
          type="number"
          value={chunkOverflow}
          onChange={(e) => setChunkOverflow(parseInt(e.target.value) || 0)}
          onBlur={() => {
            updateKnowledge(knowledge.id, {
              rag_settings: {
                ...knowledge.rag_settings,
                chunk_overflow: chunkOverflow
              }
            });
          }}
          disabled={disabled}
        />
      </Box>

      {sourceType === 'web' && (
        <>
          <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
            <TextField
              fullWidth
              label="Max crawling depth (pages to visit, max 100)"
              type="number"
              value={maxDepth}
              onChange={(e) => setMaxDepth(parseInt(e.target.value) || default_max_depth)}
              onBlur={() => {
                updateKnowledge(knowledge.id, {
                  source: {
                    web: {
                      ...knowledge.source.web,
                      crawler: {
                        enabled: true,
                        ...knowledge.source.web?.crawler,
                        max_depth: maxDepth
                      }
                    }
                  }
                });
              }}
              disabled={disabled}
            /> 
            <Tooltip title="If enabled, Helix will attempt to first extract content from the webpage. This is recommended for all documentation websites. If you are missing content, try disabling this.">
              <FormControlLabel
                control={
                  <Switch
                    checked={readability}
                    onChange={(e) => {
                      setReadability(e.target.checked);
                      // For switches, we update immediately since they don't have blur events
                      updateKnowledge(knowledge.id, {
                        source: {
                          web: {
                            ...knowledge.source.web,
                            crawler: {
                              enabled: true,
                              ...knowledge.source.web?.crawler,
                              readability: e.target.checked
                            }
                          }
                        }
                      });
                    }}
                    disabled={disabled}
                  />
                }
                label="Filter out headers, footers, etc."
                sx={{ mb: 2 }}
              />
            </Tooltip>               
          </Box>            
        </>
      )}

      <FormControl fullWidth sx={{ mb: 2 }}>
        <InputLabel>Scrape Interval</InputLabel>
        <Select
          value={refreshSchedule}
          onChange={(e) => {
            const newValue = e.target.value;
            setRefreshSchedule(newValue);
            
            // Update parent state immediately for selects
            let newSchedule = newValue;
            if (newSchedule === 'One off') newSchedule = '';
            if (newSchedule === 'custom') newSchedule = '0 0 * * *';
            
            updateKnowledge(knowledge.id, {
              refresh_schedule: newSchedule,
            });
          }}
          disabled={disabled}
        >
          <MenuItem value="One off">One off</MenuItem>
          <MenuItem value="@hourly">Hourly</MenuItem>
          <MenuItem value="@daily">Daily</MenuItem>
          <MenuItem value="custom">Custom (cron)</MenuItem>
        </Select>
      </FormControl>
      
      {refreshSchedule === 'custom' && (
        <TextField
          fullWidth
          label="Custom Cron Schedule"
          value={customSchedule}
          onChange={(e) => setCustomSchedule(e.target.value)}
          onBlur={() => {
            updateKnowledge(knowledge.id, {
              refresh_schedule: customSchedule,
            });
          }}
          disabled={disabled}
          sx={{ mb: 2 }}
          helperText="Enter a valid cron expression (default: daily at midnight)"
        />
      )}

      {knowledge && knowledge.state === 'preparing' && (
        <Alert 
          severity="warning" 
          sx={{ mt: 2, mb: 2 }}
          action={
            <Button
              color="inherit"
              size="small"
              onClick={() => onCompletePreparation(knowledge.id)}
              disabled={disabled}
            >
              Complete & Start Indexing
            </Button>
          }
        >
          This knowledge source is in preparation mode. Upload all your files, then click "Complete & Start Indexing" when you're ready.
        </Alert>
      )}
    </>
  );
};

export default KnowledgeSourceInputs; 