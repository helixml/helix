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
  Tooltip,
  Switch,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import LinkIcon from '@mui/icons-material/Link';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import Link from '@mui/material/Link';

import { IFileStoreItem, IKnowledgeSource } from '../../types';
import MuiAlert, { AlertProps } from '@mui/material/Alert';
import useSnackbar from '../../hooks/useSnackbar'; // Import the useSnackbar hook
import CrawledUrlsDialog from './CrawledUrlsDialog';
import AddKnowledgeDialog from './AddKnowledgeDialog';
import FileUpload from '../widgets/FileUpload';
import Progress from '../widgets/Progress';
import useFilestore from '../../hooks/useFilestore';
import { prettyBytes } from '../../utils/format';
import { IFilestoreUploadProgress } from '../../contexts/filestore';
interface KnowledgeEditorProps {
  knowledgeSources: IKnowledgeSource[];
  onUpdate: (updatedKnowledge: IKnowledgeSource[]) => void;  
  onRefresh: (id: string) => void;
  onUpload: (path: string, files: File[]) => Promise<void>;
  loadFiles: (path: string) => Promise<IFileStoreItem[]>;
  uploadProgress?: IFilestoreUploadProgress;
  disabled: boolean;
  knowledgeList: IKnowledgeSource[];
  appId: string;
}

const KnowledgeEditor: FC<KnowledgeEditorProps> = ({ knowledgeSources, onUpdate, onRefresh, onUpload, loadFiles, uploadProgress, disabled, knowledgeList, appId }) => {
  const [expanded, setExpanded] = useState<string | false>(false);
  const [errors, setErrors] = useState<{ [key: number]: string }>({});
  const snackbar = useSnackbar(); // Use the snackbar hook
  const [urlDialogOpen, setUrlDialogOpen] = useState(false);
  const [selectedKnowledge, setSelectedKnowledge] = useState<IKnowledgeSource | undefined>();
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [directoryFiles, setDirectoryFiles] = useState<{[key: string]: any[]}>({});

  const default_max_depth = 1;
  const default_max_pages = 5;
  const default_readability = true;

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

    // Only update the name based on the source if no custom name is set
    if (!updatedSource.name) {
      // if (newSource.source.web?.urls && newSource.source.web.urls.length > 0) {
      //   newSource.name = newSource.source.web.urls.join(', ');
      // } else if (newSource.source.filestore?.path) {
      //   newSource.name = newSource.source.filestore.path;
      // } else {
      newSource.name = 'UnnamedSource';
      // }
    }

    if (newSource.source.web && newSource.source.web.crawler) {
      newSource.source.web.crawler.enabled = true;
    }

    // Ensure default values for max_depth and max_pages
    if (newSource.source.web?.crawler) {
      newSource.source.web.crawler.max_depth = newSource.source.web.crawler.max_depth || default_max_depth;
      newSource.source.web.crawler.max_pages = newSource.source.web.crawler.max_pages || default_max_pages;
    }

    newSources[index] = newSource;
    onUpdate(newSources);
  };

  const handleAddSource = (newSource: IKnowledgeSource) => {
    let knowledges = [...knowledgeSources, newSource];
    onUpdate(knowledges);
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

  useEffect(() => {
    knowledgeSources.forEach((source, index) => {
      if (source.source.filestore?.path) {
        loadDirectoryContents(source.source.filestore.path, index);
      }
    });
  }, [knowledgeSources]);

  const getSourcePreview = (source: IKnowledgeSource): string => {
    if (source.name) {
      return source.name;
    }
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

  const getKnowledge = (source: IKnowledgeSource): IKnowledgeSource | undefined => {
    return knowledgeList.find(k => k.name === source.name);
  };

  const renderKnowledgeState = (knowledge: IKnowledgeSource | undefined) => {
    if (!knowledge) return null;
    
    let color: "default" | "primary" | "secondary" | "error" | "info" | "success" | "warning" = "default";
    switch (knowledge.state.toLowerCase()) {
      case 'ready':
        color = 'success';
        break;
      case 'pending':
      case 'indexing':
        color = 'info';
        break;
      case 'error':
        color = 'error';
        break;
      // Add more cases as needed
    }

    if (knowledge.message) {
      return (
        <Tooltip title={knowledge.message}>
          <Chip label={knowledge.state} color={color} size="small" sx={{ ml: 1 }} />
        </Tooltip>
      );
    }

    return <Chip label={knowledge.state} color={color} size="small" sx={{ ml: 1 }} />;
  };

  const handleFileUpload = async (index: number, files: File[]) => {    
    const source = knowledgeSources[index];
    if (!source.source.filestore?.path) {
      snackbar.error('No filestore path specified');
      return;
    }    

    await onUpload(source.source.filestore.path, files);

    const dirFiles = await loadFiles(source.source.filestore.path);
    setDirectoryFiles(prev => ({
      ...prev,
      [index]: dirFiles
    }));
  };

  const loadDirectoryContents = async (path: string, index: number) => {
    if (!path) return;
    try {
      const files = await loadFiles(path);
      setDirectoryFiles(prev => ({
        ...prev,
        [index]: files
      }));
    } catch (error) {
      console.error('Error loading directory contents:', error);
    }
  };

  const renderSourceInput = (source: IKnowledgeSource, index: number) => {
    const sourceType = source.source.filestore ? 'filestore' : 'web';

    return (
      <>
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
            disabled={true}
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

        <TextField
          fullWidth
          label="Name"
          value={source.name || ''}
          onChange={(e) => {
            handleSourceUpdate(index, { 
              name: e.target.value 
            });
          }}
          disabled={disabled}
          sx={{ mb: 2 }}
          placeholder="Give this knowledge source a name (optional)"
        />

        <TextField
          fullWidth
          label="Description"
          multiline
          rows={2}
          value={source.description || ''}
          onChange={(e) => {
            handleSourceUpdate(index, { 
              description: e.target.value 
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
            value={source.rag_settings.results_count}
            onChange={(e) => {
              const value = parseInt(e.target.value);
              handleSourceUpdate(index, {
                rag_settings: {
                  ...source.rag_settings,
                  results_count: value
                }
              });
            }}
            disabled={disabled}
          />
          <TextField
            fullWidth
            label="Chunk Size (optional)"
            type="number"              
            value={source.rag_settings.chunk_size}
            onChange={(e) => {
              const value = parseInt(e.target.value);
              handleSourceUpdate(index, {
                rag_settings: {
                  ...source.rag_settings,
                  chunk_size: value
                }
              });
            }}
            disabled={disabled}
          />
          <TextField
            fullWidth
            label="Chunk Overflow (optional)"
            type="number"
            value={source.rag_settings.chunk_overflow}
            onChange={(e) => {
              const value = parseInt(e.target.value);
              handleSourceUpdate(index, {
                rag_settings: {
                  ...source.rag_settings,
                  chunk_overflow: value
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
                value={source.source.web?.crawler?.max_depth || default_max_depth}
                onChange={(e) => {
                  const value = parseInt(e.target.value) || default_max_depth;
                  handleSourceUpdate(index, {
                    source: {
                      web: {
                        ...source.source.web,
                        crawler: {
                          enabled: true,
                          ...source.source.web?.crawler,
                          max_depth: value
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
                      checked={source.source.web?.crawler?.readability ?? true}
                      onChange={(e) => {
                        handleSourceUpdate(index, {
                          source: {
                            web: {
                              ...source.source.web,
                              crawler: {
                                enabled: true,
                                ...source.source.web?.crawler,
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

        {sourceType === 'filestore' && (
          <Box sx={{ mt: 2, mb: 2 }}>
            {uploadProgress ? (
              <Box sx={{ width: '100%', mb: 2 }}>
                <Typography variant="caption" sx={{ display: 'block', mb: 1 }}>
                  Uploaded {prettyBytes(uploadProgress.uploadedBytes)} of {prettyBytes(uploadProgress.totalBytes)}
                </Typography>
                <Progress progress={uploadProgress.percent} />
              </Box>
            ) : (
              <>
                <FileUpload onUpload={(files) => handleFileUpload(index, files)}>
                  {/* <Button
                    variant="contained"
                    color="secondary"
                    component="span"
                    startIcon={<CloudUploadIcon />}
                    disabled={disabled || !source.source.filestore?.path}
                    fullWidth
                  >
                    Upload Files
                  </Button> */}
                  <Box
                    sx={{
                      border: '1px dashed #ccc',
                      borderRadius: 1,
                      p: 2,
                      mt: 1,
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'stretch',
                      minHeight: '100px',
                      cursor: disabled ? 'not-allowed' : 'pointer',
                      opacity: disabled ? 0.5 : 1,
                    }}
                  >
                    {directoryFiles[index]?.length > 0 ? (
                      <>
                        <Typography variant="caption" sx={{ mb: 1 }}>
                          Current files:
                        </Typography>
                        <Box sx={{ 
                          maxHeight: '200px', 
                          overflowY: 'auto',
                          border: '1px solid #303047',
                          borderRadius: 1,
                          p: 1
                        }}>
                          {directoryFiles[index].map((file: any, fileIndex: number) => (
                            <Box 
                              key={fileIndex}
                              sx={{
                                display: 'flex',
                                alignItems: 'center',
                                p: 0.5,
                                '&:hover': {
                                  bgcolor: 'rgba(255, 255, 255, 0.05)'
                                }
                              }}
                            >
                              <Typography variant="caption" sx={{ flexGrow: 1 }}>
                                {file.name}
                              </Typography>
                              <Typography variant="caption" sx={{ ml: 2 }}>
                                {prettyBytes(file.size || 0)}
                              </Typography>
                            </Box>
                          ))}
                        </Box>
                        <Typography variant="caption" sx={{ color: '#999', mt: 1, textAlign: 'center' }}>
                          Drop files here to upload more. Manage existing files{' '}
                          <Link 
                            href={`/files?path=${encodeURIComponent(source.source.filestore?.path || '')}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            style={{ color: '#90caf9' }}
                            onClick={(e) => e.stopPropagation()}
                          >
                            here
                          </Link>.
                        </Typography>
                      </>
                    ) : (
                      <Typography variant="caption" sx={{ color: '#999', textAlign: 'center' }}>
                        {source.source.filestore?.path 
                          ? <>
                              No files yet - drop files here to upload them. Manage files{' '}
                              <Link 
                                href={`/files?path=${encodeURIComponent(source.source.filestore?.path)}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                style={{ color: '#90caf9' }}
                                onClick={(e) => e.stopPropagation()}
                              >
                                here
                              </Link>.
                            </>
                          : 'Specify a filestore path first'
                        }
                      </Typography>
                    )}
                  </Box>
                </FileUpload>
                {source.source.filestore?.path && (
                  <>
                  <Button
                    sx={{ mt: 1 }}
                    size="small"
                    startIcon={<RefreshIcon />}
                    onClick={() => loadDirectoryContents(source.source.filestore?.path || '', index)}
                  >
                    Refresh File List
                  </Button>
                  </>
                )}
              </>
            )}
          </Box>
        )}
      </>
    );
  };

  return (
    <Box>
      {knowledgeSources.map((source, index) => {
        const knowledge = getKnowledge(source);
        
        return (
          <Accordion
            key={index}
            expanded={expanded === `panel${index}`}
            onChange={handleChange(`panel${index}`)}
          >
            <AccordionSummary 
              expandIcon={<ExpandMoreIcon />}
              sx={{ display: 'flex', alignItems: 'center' }}
            >
              <Box sx={{ flexGrow: 1 }}>
                <Typography component="div" sx={{ display: 'flex', alignItems: 'center' }}>
                  Knowledge Source ({getSourcePreview(source)})
                  {renderKnowledgeState(knowledge)}
                </Typography>
                {knowledge?.state === 'indexing' && (
                  <>
                    {knowledge?.progress?.step && knowledge?.progress?.step !== '' ? (
                      <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                        {knowledge.progress.step} {knowledge.progress.progress ? `| ${knowledge.progress.progress}%` : ''} {knowledge.progress.message ? `| ${knowledge.progress.message}` : ''} {knowledge.progress.started_at ? `| elapsed: ${Math.round((Date.now() - new Date(knowledge.progress.started_at).getTime()) / 1000)}s` : ''}
                      </Typography>
                    ) : (
                      <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                        Pending...
                      </Typography>
                    )}
                  </>
                )}
                <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                  Version: {knowledge?.version || 'N/A'}
                </Typography>
              </Box>
              {source.source.web && (
                <Tooltip title="View crawled URLs">
                  <IconButton
                    onClick={(e) => {
                      e.stopPropagation();
                      setSelectedKnowledge(knowledge);
                      setUrlDialogOpen(true);
                    }}
                    disabled={disabled || !knowledge}
                    sx={{ mr: 1 }}
                  >
                    <LinkIcon />
                  </IconButton>
                </Tooltip>
              )}
              <Tooltip title="Refresh knowledge and reindex data">
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
              </Tooltip>
              <Tooltip title="Delete this knowledge source">
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
              </Tooltip>
            </AccordionSummary>
            <AccordionDetails>
              {renderSourceInput(source, index)}
            </AccordionDetails>
          </Accordion>
        );
      })}
      <Button
        variant="outlined"
        startIcon={<AddIcon />}
        onClick={() => setAddDialogOpen(true)}
        disabled={disabled}
        sx={{ mt: 2 }}
      >
        Add Knowledge Source
      </Button>
      <AddKnowledgeDialog
        open={addDialogOpen}
        onClose={() => setAddDialogOpen(false)}
        onAdd={handleAddSource}
        appId={appId}
      />
      {Object.keys(errors).length > 0 && (
        <Alert severity="error" sx={{ mt: 2 }}>
          Please specify at least one URL for each knowledge source.
        </Alert>
      )}
      <CrawledUrlsDialog
        open={urlDialogOpen}
        onClose={() => setUrlDialogOpen(false)}
        knowledge={selectedKnowledge}
      />
    </Box>
  );
};

export default KnowledgeEditor;