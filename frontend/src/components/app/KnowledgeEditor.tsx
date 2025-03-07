import React, { FC, useState, useEffect, useRef, useCallback } from 'react';
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
  FormControlLabel,
  Chip,
  Tooltip,
  Switch,
  CircularProgress,
  Grid,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import LinkIcon from '@mui/icons-material/Link';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CloseIcon from '@mui/icons-material/Close';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import debounce from 'lodash/debounce';
import useAccount from '../../hooks/useAccount';

import { IFileStoreItem, IKnowledgeSource } from '../../types';
import useSnackbar from '../../hooks/useSnackbar';
import useApi from '../../hooks/useApi';
import CrawledUrlsDialog from './CrawledUrlsDialog';
import AddKnowledgeDialog from './AddKnowledgeDialog';
import FileUpload from '../widgets/FileUpload';
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
  onRequestSave?: () => Promise<any>;
}

const KnowledgeEditor: FC<KnowledgeEditorProps> = ({ knowledgeSources, onUpdate, onRefresh, onUpload, loadFiles, uploadProgress, disabled, knowledgeList, appId, onRequestSave }) => {
  const [expanded, setExpanded] = useState<string | false>(false);
  const [errors, setErrors] = useState<Record<string, string[]>>({});
  const { error: snackbarError, info: snackbarInfo, success: snackbarSuccess } = useSnackbar();
  const api = useApi();
  const [urlDialogOpen, setUrlDialogOpen] = useState(false);
  const [selectedKnowledge, setSelectedKnowledge] = useState<IKnowledgeSource | undefined>();
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [directoryFiles, setDirectoryFiles] = useState<Record<number, IFileStoreItem[]>>({});
  const [deletingFiles, setDeletingFiles] = useState<Record<string, boolean>>({});
  const [localUploadProgress, setLocalUploadProgress] = useState<any>(null);
  const uploadStartTimeRef = useRef<number | null>(null);
  const [uploadEta, setUploadEta] = useState<string | null>(null);
  const cancelTokenRef = useRef<AbortController | null>(null);
  const uploadCancelledRef = useRef<boolean>(false);
  const uploadSpeedRef = useRef<number[]>([]);
  const [currentSpeed, setCurrentSpeed] = useState<number | null>(null);
  const [uploadingFileCount, setUploadingFileCount] = useState<number>(0);
  const account = useAccount();

  const debouncedUpdate = useCallback(
    debounce((updatedSources: IKnowledgeSource[]) => {
      console.log('[KnowledgeEditor] Debounced update triggered with sources:', updatedSources);
      onUpdate(updatedSources);
    }, 300),
    [onUpdate]
  );

  useEffect(() => {
    console.log('[KnowledgeEditor] Component mounted or updated with props:', {
      knowledgeSources,
      disabled,
      knowledgeList,
      appId
    });

    return () => {
      console.log('[KnowledgeEditor] Component will unmount');
    };
  }, [knowledgeSources, disabled, knowledgeList, appId]);

  useEffect(() => {
    console.log('[KnowledgeEditor] Knowledge sources changed:', knowledgeSources);
  }, [knowledgeSources]);

  useEffect(() => {
    console.log('[KnowledgeEditor] Knowledge list (backend data) changed:', knowledgeList);
  }, [knowledgeList]);

  useEffect(() => {
    console.log('KnowledgeEditor uploadProgress:', uploadProgress);
  }, [uploadProgress]);

  const default_max_depth = 1;
  const default_max_pages = 5;

  const handleChange = (panel: string) => (event: React.SyntheticEvent, isExpanded: boolean) => {
    setExpanded(isExpanded ? panel : false);
  };

  const handleSourceUpdate = (index: number, updatedSource: Partial<IKnowledgeSource>) => {
    console.log('[KnowledgeEditor] handleSourceUpdate - Original source:', knowledgeSources[index]);
    console.log('[KnowledgeEditor] handleSourceUpdate - Updated fields:', updatedSource);
    
    const newSources = [...knowledgeSources];
    const existingSource = newSources[index];
    
    let newSource = { ...existingSource, ...updatedSource };    
    
    console.log('[KnowledgeEditor] handleSourceUpdate - After merge:', newSource);

    if (newSource.refresh_schedule === 'custom') {
      newSource.refresh_schedule = '0 0 * * *';
    } else if (newSource.refresh_schedule === 'One off') {
      newSource.refresh_schedule = '';
    }

    if (newSource.source.web && newSource.source.web.crawler) {
      newSource.source.web.crawler.enabled = true;
    }

    if (newSource.source.web?.crawler) {
      newSource.source.web.crawler.max_depth = newSource.source.web.crawler.max_depth || default_max_depth;
      newSource.source.web.crawler.max_pages = newSource.source.web.crawler.max_pages || default_max_pages;
    }

    newSources[index] = newSource;
    console.log('[KnowledgeEditor] handleSourceUpdate - Final sources array being sent to parent:', newSources);
    
    if (updatedSource.name || updatedSource.description || 
        (updatedSource.source?.web?.urls && !updatedSource.source.filestore)) {
      debouncedUpdate(newSources);
    } else {
      onUpdate(newSources);
    }
  };

  useEffect(() => {
    return () => {
      debouncedUpdate.cancel();
    };
  }, [debouncedUpdate]);

  const handleAddSource = (newSource: IKnowledgeSource) => {
    console.log('[KnowledgeEditor] handleAddSource - Adding new source:', newSource);
    let knowledges = [...knowledgeSources, newSource];
    console.log('[KnowledgeEditor] handleAddSource - Updated knowledge array:', knowledges);
    onUpdate(knowledges);
    
    setExpanded(`panel${knowledgeSources.length}`);
    
    if (newSource.source.filestore) {
      snackbarInfo(`Knowledge source "${newSource.name}" created. You can now upload files.`);
    }
  };

  const deleteSource = (index: number) => {
    console.log('[KnowledgeEditor] deleteSource - Deleting source at index:', index);
    const newSources = knowledgeSources.filter((_, i) => i !== index);
    console.log('[KnowledgeEditor] deleteSource - Remaining sources:', newSources);
    onUpdate(newSources);
  };

  const refreshSource = (index: number) => {
    const knowledge = knowledgeList.find(k => k.name === knowledgeSources[index].name);
    if (knowledge) {
      onRefresh(knowledge.id);
      snackbarSuccess('Knowledge refresh initiated. This may take a few minutes.');
    }
  };

  const validateSources = () => {
    const newErrors: Record<string, string[]> = {};
    knowledgeSources.forEach((source, index) => {      
      if ((!source.source.web?.urls || source.source.web.urls.length === 0) && !source.source.filestore?.path) {
        newErrors[`${index}`] = ["At least one URL or a filestore path must be specified."];
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
    if (source.source.web?.urls && source.source.web.urls.length > 0) {
      return source.source.web.urls[0];
    } else if (source.source.filestore?.path) {
      const path = source.source.filestore.path;
      
      if (path.startsWith(`apps/${appId}/`)) {
        return path.substring(`apps/${appId}/`.length);
      }
      
      const appIdPattern = /^apps\/app_[a-zA-Z0-9]+\//;
      if (appIdPattern.test(path)) {
        return path.replace(appIdPattern, '');
      }
      
      return path;
    }
    return 'Unknown source';
  };

  const getKnowledge = (source: IKnowledgeSource): IKnowledgeSource | undefined => {
    if (source.id) {
      const byId = knowledgeList.find(k => k.id === source.id);
      if (byId) return byId;
    }
    
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

  const formatTimeRemaining = (seconds: number): string => {
    if (seconds < 60) {
      return `${Math.round(seconds)}s`;
    } else if (seconds < 3600) {
      return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`;
    } else {
      return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
    }
  };

  const formatSpeed = (bytesPerSecond: number): string => {
    if (bytesPerSecond < 1024) {
      return `${bytesPerSecond.toFixed(1)} B/s`;
    } else if (bytesPerSecond < 1024 * 1024) {
      return `${(bytesPerSecond / 1024).toFixed(1)} KB/s`;
    } else if (bytesPerSecond < 1024 * 1024 * 1024) {
      return `${(bytesPerSecond / (1024 * 1024)).toFixed(1)} MB/s`;
    } else {
      return `${(bytesPerSecond / (1024 * 1024 * 1024)).toFixed(1)} GB/s`;
    }
  };

  const calculateEta = (loaded: number, total: number, startTime: number) => {
    const elapsedSeconds = (Date.now() - startTime) / 1000;
    
    if (elapsedSeconds < 0.1) {
      const percentComplete = loaded / total;
      if (percentComplete > 0) {
        return formatTimeRemaining(Math.ceil((total / loaded) * elapsedSeconds));
      }
      return "Calculating...";
    }
    
    const currentSpeedValue = loaded / elapsedSeconds;
    
    uploadSpeedRef.current.push(currentSpeedValue);
    if (uploadSpeedRef.current.length > 5) {
      uploadSpeedRef.current.shift();
    }
    
    const smoothedSpeed = uploadSpeedRef.current.reduce((sum, speed) => sum + speed, 0) / 
                         uploadSpeedRef.current.length;
    
    setCurrentSpeed(smoothedSpeed);
    
    if (smoothedSpeed > 0) {
      const remainingBytes = total - loaded;
      const remainingSeconds = remainingBytes / smoothedSpeed;
      
      if (remainingSeconds < 1 && remainingSeconds > 0) {
        return "< 1s";
      }
      
      return formatTimeRemaining(remainingSeconds);
    }
    
    return "Calculating...";
  };

  const handleFileUpload = async (index: number, files: File[]) => {    
    const source = knowledgeSources[index];
    if (!source.source.filestore?.path) {
      snackbarError('No filestore path specified');
      return;
    }

    let uploadPath = source.source.filestore.path;
    if (!uploadPath.startsWith(`apps/${appId}/`)) {
      uploadPath = `apps/${appId}/${uploadPath}`;
    }

    uploadCancelledRef.current = false;
    
    cancelTokenRef.current = new AbortController();
    
    try {
      uploadSpeedRef.current = [];
      setCurrentSpeed(null);
      setUploadingFileCount(files.length);
      
      uploadStartTimeRef.current = Date.now();
      setLocalUploadProgress({
        percent: 0,
        uploadedBytes: 0,
        totalBytes: files.reduce((total, file) => total + file.size, 0)
      });
      setUploadEta("Calculating..."); 

      const formData = new FormData();
      files.forEach((file) => {
        formData.append("files", file);
      });

      try {
        await api.post('/api/v1/filestore/upload', formData, {
          params: {
            path: uploadPath,
          },
          signal: cancelTokenRef.current.signal,
          onUploadProgress: (progressEvent) => {
            if (uploadCancelledRef.current) return;
            
            const percent = progressEvent.total && progressEvent.total > 0 ?
              Math.round((progressEvent.loaded * 100) / progressEvent.total) : 0;
            
            setLocalUploadProgress({
              percent,
              uploadedBytes: progressEvent.loaded || 0,
              totalBytes: progressEvent.total || 0,
            });
            
            if (uploadStartTimeRef.current && progressEvent.total && progressEvent.loaded > 0) {
              const eta = calculateEta(progressEvent.loaded, progressEvent.total, uploadStartTimeRef.current);
              setUploadEta(eta);
            }
          }
        });

        if (!uploadCancelledRef.current) {
          snackbarSuccess(`Successfully uploaded ${files.length} file${files.length !== 1 ? 's' : ''}`);

          const updatedFiles = await loadFiles(uploadPath);
          setDirectoryFiles(prev => ({
            ...prev,
            [index]: updatedFiles
          }));

          if (onRequestSave) {
            console.log('Auto-saving app after file upload to trigger indexing');
            await onRequestSave();
            
            const knowledge = getKnowledge(source);
            if (knowledge && knowledge.id) {
              console.log('Triggering re-indexing after file upload for knowledge source:', knowledge.id);
              onRefresh(knowledge.id);
              snackbarInfo('Re-indexing started for newly uploaded files. This may take a few minutes.');
            }
          }
        }
      } catch (uploadError: unknown) {
        if (
          typeof uploadError === 'object' && 
          uploadError !== null && 
          ('name' in uploadError) && 
          (uploadError.name === 'AbortError' || uploadError.name === 'CanceledError')
        ) {
          console.log('Upload was cancelled by user');
          return;
        }
        
        if (!uploadCancelledRef.current) {
          console.error('Direct upload failed, falling back to onUpload method:', uploadError);
          
          try {
            await onUpload(uploadPath, files);
            
            if (!uploadCancelledRef.current) {
              snackbarSuccess(`Successfully uploaded ${files.length} file${files.length !== 1 ? 's' : ''}`);
              
              const fallbackFiles = await loadFiles(uploadPath);
              setDirectoryFiles(prev => ({
                ...prev,
                [index]: fallbackFiles
              }));

              if (onRequestSave) {
                console.log('Auto-saving app after file upload to trigger indexing');
                await onRequestSave();
                
                const knowledge = getKnowledge(source);
                if (knowledge && knowledge.id) {
                  console.log('Triggering re-indexing after fallback file upload for knowledge source:', knowledge.id);
                  onRefresh(knowledge.id);
                  snackbarInfo('Re-indexing started for newly uploaded files. This may take a few minutes.');
                }
              }
            }
          } catch (fallbackError) {
            if (!uploadCancelledRef.current) {
              console.error('Error in fallback upload:', fallbackError);
              snackbarError('Failed to upload files. Please try again.');
            }
          }
        }
      }
    } catch (error: unknown) {
      if (!uploadCancelledRef.current) {
        console.error('Error uploading files:', error);
        snackbarError('Failed to upload files. Please try again.');
      }
    } finally {
      if (uploadCancelledRef.current) {
        setLocalUploadProgress(null);
        uploadStartTimeRef.current = null;
        setUploadEta(null);
        setUploadingFileCount(0);
        cancelTokenRef.current = null;
      } else {
        setTimeout(() => {
          setLocalUploadProgress(null);
          uploadStartTimeRef.current = null;
          setUploadEta(null);
          setUploadingFileCount(0);
          cancelTokenRef.current = null;
        }, 1000);
      }
      
      uploadCancelledRef.current = false;
    }
  };

  const handleCancelUpload = () => {
    if (cancelTokenRef.current) {
      uploadCancelledRef.current = true;
      
      snackbarInfo('Upload cancelled');
      
      cancelTokenRef.current.abort();
      
      setLocalUploadProgress(null);
      uploadStartTimeRef.current = null;
      setUploadEta(null);
      setUploadingFileCount(0);
      cancelTokenRef.current = null;
    }
  };

  const loadDirectoryContents = async (path: string, index: number) => {
    if (!path) return;
    try {
      let loadPath = path;
      if (!loadPath.startsWith(`apps/${appId}/`)) {
        loadPath = `apps/${appId}/${loadPath}`;
      }
      const files = await loadFiles(loadPath);
      setDirectoryFiles(prev => ({
        ...prev,
        [index]: files
      }));
    } catch (error) {
      console.error('Error loading directory contents:', error);
    }
  };

  const handleDeleteFile = async (index: number, fileName: string) => {
    const source = knowledgeSources[index];
    if (!source.source.filestore?.path) {
      snackbarError('No filestore path specified');
      return;
    }
    
    try {
      const fileId = `${index}-${fileName}`;
      setDeletingFiles(prev => ({
        ...prev,
        [fileId]: true
      }));
      
      let basePath = source.source.filestore.path;
      if (!basePath.startsWith(`apps/${appId}/`)) {
        basePath = `apps/${appId}/${basePath}`;
      }
      
      const filePath = `${basePath}/${fileName}`;
      
      const response = await api.delete('/api/v1/filestore/delete', {
        params: {
          path: filePath,
        }
      });
      
      if (response) {
        snackbarSuccess(`File "${fileName}" deleted successfully`);
        
        const files = await loadFiles(basePath);
        setDirectoryFiles(prev => ({
          ...prev,
          [index]: files
        }));
      } else {
        snackbarError(`Failed to delete file "${fileName}"`);
      }
    } catch (error) {
      console.error('Error deleting file:', error);
      snackbarError('An error occurred while deleting the file');
    } finally {
      const fileId = `${index}-${fileName}`;
      setDeletingFiles(prev => ({
        ...prev,
        [fileId]: false
      }));
    }
  };

  const renderSourceInput = (source: IKnowledgeSource, index: number) => {
    const sourceType = source.source.filestore ? 'filestore' : 'web';

    return (
      <>
        {sourceType === 'filestore' ? (
          null
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
            error={!!errors[`${index}`]}
            helperText={errors[`${index}`]?.join(', ')}
          />
        )}

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
            value={source.rag_settings.chunk_size || ''}
            onChange={(e) => {
              const value = e.target.value ? parseInt(e.target.value) : undefined;
              handleSourceUpdate(index, {
                rag_settings: {
                  ...source.rag_settings,
                  chunk_size: value ?? 0
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
            <Typography variant="subtitle2" sx={{ mb: 1 }}>
              Upload Files
            </Typography>

            <Box
              sx={{
                width: '100%',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
              }}
            >
              {localUploadProgress ? (
                <Box sx={{ 
                  border: '1px solid rgba(255, 255, 255, 0.2)', 
                  borderRadius: '8px', 
                  padding: 3, 
                  backgroundColor: 'rgba(0, 0, 0, 0.7)',
                  width: '100%', 
                  marginBottom: 2,
                  position: 'relative',
                  overflow: 'hidden'
                }}>
                  <Box sx={{ 
                    display: 'flex', 
                    justifyContent: 'space-between', 
                    alignItems: 'center', 
                    mb: 2
                  }}>
                    <Typography variant="h6" fontWeight="500" color="common.white">
                      Uploading {uploadingFileCount} {uploadingFileCount === 1 ? 'File' : 'Files'}
                    </Typography>
                    
                    <Button 
                      variant="outlined" 
                      color="error" 
                      size="small" 
                      onClick={handleCancelUpload}
                      startIcon={<CloseIcon />}
                      sx={{ 
                        borderRadius: '20px'
                      }}
                    >
                      Cancel
                    </Button>
                  </Box>
                  
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography variant="body1" color="common.white" fontWeight="medium">
                      {localUploadProgress.percent}% Complete
                    </Typography>
                    <Typography variant="body2" color="rgba(255, 255, 255, 0.7)">
                      {prettyBytes(localUploadProgress.uploadedBytes)} of {prettyBytes(localUploadProgress.totalBytes)}
                    </Typography>
                  </Box>
                  
                  <Box sx={{ 
                    width: '100%', 
                    height: '8px', 
                    backgroundColor: 'rgba(255,255,255,0.1)', 
                    borderRadius: '4px',
                    overflow: 'hidden',
                    mb: 2
                  }}>
                    <Box 
                      sx={{ 
                        height: '100%', 
                        width: `${localUploadProgress.percent}%`, 
                        background: 'linear-gradient(90deg, #2196f3 0%, #64b5f6 100%)',
                        transition: 'width 0.3s ease-in-out'
                      }} 
                    />
                  </Box>
                  
                  <Grid container spacing={2}>
                    <Grid item xs={6}>
                      <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Typography variant="caption" color="rgba(255, 255, 255, 0.7)">
                          ESTIMATED TIME REMAINING
                        </Typography>
                        <Typography variant="body2" color="common.white" fontWeight="medium">
                          {uploadEta || "Calculating..."}
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={6}>
                      <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Typography variant="caption" color="rgba(255, 255, 255, 0.7)">
                          UPLOAD SPEED
                        </Typography>
                        <Typography variant="body2" color="common.white" fontWeight="medium">
                          {currentSpeed ? formatSpeed(currentSpeed) : "Calculating..."}
                        </Typography>
                      </Box>
                    </Grid>
                  </Grid>
                </Box>
              ) : (
                <>
                  <FileUpload onUpload={(files) => handleFileUpload(index, files)}>
                    <Box
                      sx={{
                        border: '1px dashed #ccc',
                        borderRadius: 1,
                        p: 2,
                        mt: 1,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        minHeight: '100px',
                        width: '100%',
                        cursor: disabled ? 'not-allowed' : 'pointer',
                        opacity: disabled ? 0.5 : 1,
                        transition: 'all 0.2s ease',
                        '&:hover': {
                          backgroundColor: 'rgba(144, 202, 249, 0.08)',
                          borderColor: '#90caf9'
                        }
                      }}
                    >
                      <CloudUploadIcon sx={{ fontSize: 40, mb: 1, color: '#90caf9' }} />
                      <Typography align="center" variant="body2">
                        Drag and drop files here or click to upload
                      </Typography>
                      <Typography align="center" variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
                        Supported files: PDF, DOC, DOCX, TXT, CSV, JSON, and more
                      </Typography>
                    </Box>
                  </FileUpload>
                </>
              )}
            </Box>

            {directoryFiles[index]?.length > 0 && !localUploadProgress && (
              <>
                <Typography variant="caption" sx={{ mt: 2, mb: 1, display: 'block' }}>
                  Current files:
                </Typography>
                <Box sx={{ 
                  maxHeight: '200px', 
                  overflowY: 'auto',
                  border: '1px solid #303047',
                  borderRadius: 1,
                  p: 1,
                  width: '100%'
                }}>
                  {directoryFiles[index].map((file: any, fileIndex: number) => {
                    const fileId = `${index}-${file.name}`;
                    const isDeleting = deletingFiles[fileId] === true;
                    
                    return (
                      <Box 
                        key={fileIndex}
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          p: 0.5,
                          borderRadius: '4px',
                          opacity: isDeleting ? 0.6 : 1,
                          '&:hover': {
                            bgcolor: 'rgba(255, 255, 255, 0.05)'
                          }
                        }}
                      >
                        <Typography 
                          variant="caption" 
                          sx={{ 
                            flexGrow: 1, 
                            overflow: 'hidden', 
                            textOverflow: 'ellipsis',
                            '& > span': {
                              cursor: 'pointer',
                              color: 'primary.main',
                              textDecoration: 'none',
                              '&:hover': {
                                textDecoration: 'underline'
                              }
                            }
                          }}
                        >
                          <span
                            onClick={(e) => {
                              e.stopPropagation();
                              if (!file.directory) {
                                openFileInNewTab(file, source.source.filestore?.path || '');
                              }
                            }}
                            style={{ 
                              opacity: file.directory ? 0.5 : 1,
                              cursor: file.directory ? 'not-allowed' : 'pointer'
                            }}
                          >
                            {file.name}
                          </span>
                        </Typography>
                        <Typography variant="caption" sx={{ ml: 2, color: 'text.secondary', minWidth: '60px', textAlign: 'right' }}>
                          {prettyBytes(file.size || 0)}
                        </Typography>
                        
                        {/* Delete file button */}
                        <Tooltip title={isDeleting ? "Deleting..." : "Delete file"}>
                          <span>
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                if (!isDeleting && window.confirm(`Are you sure you want to delete "${file.name}"?`)) {
                                  handleDeleteFile(index, file.name);
                                }
                              }}
                              disabled={disabled || isDeleting}
                              sx={{ 
                                ml: 1,
                                color: 'error.main',
                                '&:hover': {
                                  bgcolor: 'rgba(244, 67, 54, 0.08)'
                                }
                              }}
                            >
                              {isDeleting ? (
                                <CircularProgress size={16} color="inherit" />
                              ) : (
                                <DeleteIcon fontSize="small" />
                              )}
                            </IconButton>
                          </span>
                        </Tooltip>
                      </Box>
                    );
                  })}
                </Box>
                {source.source.filestore?.path && (
                  <Box sx={{ display: 'flex', mt: 1 }}>
                    <Button
                      size="small"
                      startIcon={<RefreshIcon />}
                      onClick={() => loadDirectoryContents(source.source.filestore?.path || '', index)}
                    >
                      Refresh Files
                    </Button>
                    <Button
                      size="small"
                      startIcon={<FolderOpenIcon />}
                      onClick={() => openInFilestore(source.source.filestore?.path || '')}
                      sx={{ ml: 1 }}
                    >
                      Open in Filestore
                    </Button>
                  </Box>
                )}
              </>
            )}
            
            {directoryFiles[index]?.length === 0 && !localUploadProgress && (
              <Typography variant="caption" sx={{ color: '#999', textAlign: 'center', mt: 2, display: 'block' }}>
                {source.source.filestore?.path 
                  ? 'No files uploaded yet. Drag and drop files here to upload.'
                  : 'Specify a filestore path first'
                }
              </Typography>
            )}
          </Box>
        )}
      </>
    );
  };

  // Add functions to open files in a new tab and in the filestore
  const openFileInNewTab = (file: IFileStoreItem, sourcePath: string) => {
    if (!account.token) {
      snackbarError('Must be logged in to view files');
      return;
    }

    // Ensure the path is properly scoped to the app directory
    let basePath = sourcePath;
    if (!basePath.startsWith(`apps/${appId}/`)) {
      basePath = `apps/${appId}/${basePath}`;
    }

    // Construct the full URL to the file
    const fileUrl = `${file.url}?access_token=${account.tokenUrlEscaped}`;
    window.open(fileUrl, '_blank');
  };

  const openInFilestore = (sourcePath: string) => {
    // Ensure the path is properly scoped to the app directory
    let basePath = sourcePath;
    if (!basePath.startsWith(`apps/${appId}/`)) {
      basePath = `apps/${appId}/${basePath}`;
    }

    // Open the filestore page with the given path
    window.open(`/files?path=${encodeURIComponent(basePath)}`, '_blank');
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