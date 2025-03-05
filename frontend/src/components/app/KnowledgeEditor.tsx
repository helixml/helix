import React, { FC, useState, useEffect, useRef } from 'react';
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
  CircularProgress,
  Grid,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import LinkIcon from '@mui/icons-material/Link';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import Link from '@mui/material/Link';
import CloseIcon from '@mui/icons-material/Close';

import { IFileStoreItem, IKnowledgeSource } from '../../types';
import MuiAlert, { AlertProps } from '@mui/material/Alert';
import useSnackbar from '../../hooks/useSnackbar'; // Import the useSnackbar hook
import useApi from '../../hooks/useApi'; // Add useApi hook
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
  const api = useApi(); // Use the API hook
  const [urlDialogOpen, setUrlDialogOpen] = useState(false);
  const [selectedKnowledge, setSelectedKnowledge] = useState<IKnowledgeSource | undefined>();
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [directoryFiles, setDirectoryFiles] = useState<Record<number, IFileStoreItem[]>>({});
  const [deletingFiles, setDeletingFiles] = useState<{[key: string]: boolean}>({});
  const [localUploadProgress, setLocalUploadProgress] = useState<IFilestoreUploadProgress | null>(null);
  const [addSourceDialogOpen, setAddSourceDialogOpen] = useState(false);
  const uploadStartTimeRef = useRef<number | null>(null);
  const [uploadEta, setUploadEta] = useState<string | null>(null);
  const cancelTokenRef = useRef<AbortController | null>(null);
  // Add a dedicated ref for tracking cancellation state
  const uploadCancelledRef = useRef<boolean>(false);
  // Add a ref to track upload speed for smoothing
  const uploadSpeedRef = useRef<number[]>([]);
  // Add state to store the current speed for display
  const [currentSpeed, setCurrentSpeed] = useState<number | null>(null);
  // Add a state to track file count
  const [uploadingFileCount, setUploadingFileCount] = useState<number>(0);

  // Debug: Log uploadProgress
  useEffect(() => {
    console.log('KnowledgeEditor uploadProgress:', uploadProgress);
  }, [uploadProgress]);

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
    
    // Expand the newly added knowledge source panel
    setExpanded(`panel${knowledgeSources.length}`);
    
    // If this is a filestore source, show a message to the user about uploading files
    if (newSource.source.filestore) {
      snackbar.info(`Knowledge source "${newSource.name}" created. You can now upload files.`);
    }
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

  // Improved time formatting function
  const formatTimeRemaining = (seconds: number): string => {
    if (seconds < 60) {
      return `${Math.round(seconds)}s`;
    } else if (seconds < 3600) {
      return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`;
    } else {
      return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
    }
  };

  // Format upload speed to human-readable format
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

  // Improved ETA calculator with smoothing
  const calculateEta = (loaded: number, total: number, startTime: number) => {
    const elapsedSeconds = (Date.now() - startTime) / 1000;
    
    // Return early guess for very small uploads
    if (elapsedSeconds < 0.1) {
      const percentComplete = loaded / total;
      if (percentComplete > 0) {
        // Make a rough initial guess
        return formatTimeRemaining(Math.ceil((total / loaded) * elapsedSeconds));
      }
      return "Calculating...";
    }
    
    // Calculate current speed
    const currentSpeedValue = loaded / elapsedSeconds; // bytes per second
    
    // Add to speed history (keep last 5 speed measurements)
    uploadSpeedRef.current.push(currentSpeedValue);
    if (uploadSpeedRef.current.length > 5) {
      uploadSpeedRef.current.shift();
    }
    
    // Calculate smoothed speed (average of last 5 measurements)
    const smoothedSpeed = uploadSpeedRef.current.reduce((sum, speed) => sum + speed, 0) / 
                         uploadSpeedRef.current.length;
    
    // Update the speed state for display
    setCurrentSpeed(smoothedSpeed);
    
    if (smoothedSpeed > 0) {
      const remainingBytes = total - loaded;
      const remainingSeconds = remainingBytes / smoothedSpeed;
      
      // For very small values, round up to at least 1 second
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
      snackbar.error('No filestore path specified');
      return;
    }

    // Reset cancellation state at the start of every upload
    uploadCancelledRef.current = false;
    
    // Create abort controller for cancellation
    cancelTokenRef.current = new AbortController();
    
    try {
      // Reset speed measurement history at the start of upload
      uploadSpeedRef.current = [];
      // Reset current speed
      setCurrentSpeed(null);
      // Set the count of files being uploaded
      setUploadingFileCount(files.length);
      
      // Set initial upload progress and start time
      uploadStartTimeRef.current = Date.now();
      setLocalUploadProgress({
        percent: 0,
        uploadedBytes: 0,
        totalBytes: files.reduce((total, file) => total + file.size, 0)
      });
      setUploadEta("Calculating..."); 

      // Create form data for file upload
      const formData = new FormData();
      files.forEach((file) => {
        formData.append("files", file);
      });

      try {
        // Try direct upload first
        await api.post('/api/v1/filestore/upload', formData, {
          params: {
            path: source.source.filestore.path,
          },
          signal: cancelTokenRef.current.signal,
          onUploadProgress: (progressEvent) => {
            // Skip updates if cancelled
            if (uploadCancelledRef.current) return;
            
            // Update progress directly
            const percent = progressEvent.total && progressEvent.total > 0 ?
              Math.round((progressEvent.loaded * 100) / progressEvent.total) : 0;
            
            setLocalUploadProgress({
              percent,
              uploadedBytes: progressEvent.loaded || 0,
              totalBytes: progressEvent.total || 0,
            });
            
            // Calculate and update ETA immediately with any progress data
            if (uploadStartTimeRef.current && progressEvent.total && progressEvent.loaded > 0) {
              const eta = calculateEta(progressEvent.loaded, progressEvent.total, uploadStartTimeRef.current);
              setUploadEta(eta);
            }
          }
        });

        // Only show success if we reach here without cancellation
        if (!uploadCancelledRef.current) {
          // Show success message
          snackbar.success(`Successfully uploaded ${files.length} file${files.length !== 1 ? 's' : ''}`);

          // Refresh the file list
          const dirFiles = await loadFiles(source.source.filestore.path);
          setDirectoryFiles(prev => ({
            ...prev,
            [index]: dirFiles
          }));
        }
      } catch (uploadError: unknown) {
        // Check if this was a cancellation
        if (
          typeof uploadError === 'object' && 
          uploadError !== null && 
          ('name' in uploadError) && 
          (uploadError.name === 'AbortError' || uploadError.name === 'CanceledError')
        ) {
          console.log('Upload was cancelled by user');
          return; // Skip any further processing
        }
        
        // Only proceed with fallback if not cancelled
        if (!uploadCancelledRef.current) {
          console.error('Direct upload failed, falling back to onUpload method:', uploadError);
          
          try {
            await onUpload(source.source.filestore.path, files);
            
            // Double-check cancellation state again before success
            if (!uploadCancelledRef.current) {
              snackbar.success(`Successfully uploaded ${files.length} file${files.length !== 1 ? 's' : ''}`);
              
              const dirFiles = await loadFiles(source.source.filestore.path);
              setDirectoryFiles(prev => ({
                ...prev,
                [index]: dirFiles
              }));
            }
          } catch (fallbackError) {
            if (!uploadCancelledRef.current) {
              console.error('Error in fallback upload:', fallbackError);
              snackbar.error('Failed to upload files. Please try again.');
            }
          }
        }
      }
    } catch (error: unknown) {
      // Only show errors if not cancelled
      if (!uploadCancelledRef.current) {
        console.error('Error uploading files:', error);
        snackbar.error('Failed to upload files. Please try again.');
      }
    } finally {
      // Clean up based on cancellation state
      if (uploadCancelledRef.current) {
        // Immediate cleanup for cancellation
        setLocalUploadProgress(null);
        uploadStartTimeRef.current = null;
        setUploadEta(null);
        setUploadingFileCount(0); // Reset file count
        cancelTokenRef.current = null;
      } else {
        // Delay cleanup for successful completion
        setTimeout(() => {
          setLocalUploadProgress(null);
          uploadStartTimeRef.current = null;
          setUploadEta(null);
          setUploadingFileCount(0); // Reset file count
          cancelTokenRef.current = null;
        }, 1000);
      }
      
      // Reset cancellation state
      uploadCancelledRef.current = false;
    }
  };

  // Rewrite cancel function to use the ref
  const handleCancelUpload = () => {
    if (cancelTokenRef.current) {
      // Set cancellation state first
      uploadCancelledRef.current = true;
      
      // Show cancellation message
      snackbar.info('Upload cancelled');
      
      // Then abort the request
      cancelTokenRef.current.abort();
      
      // Clean up immediately
      setLocalUploadProgress(null);
      uploadStartTimeRef.current = null;
      setUploadEta(null);
      setUploadingFileCount(0); // Reset file count
      cancelTokenRef.current = null;
    }
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

  const handleDeleteFile = async (index: number, fileName: string) => {
    const source = knowledgeSources[index];
    if (!source.source.filestore?.path) {
      snackbar.error('No filestore path specified');
      return;
    }
    
    try {
      // Set deleting state for this file
      const fileId = `${index}-${fileName}`;
      setDeletingFiles(prev => ({
        ...prev,
        [fileId]: true
      }));
      
      // Construct the full path to the file
      const filePath = `${source.source.filestore.path}/${fileName}`;
      
      // Call the API to delete the file
      const response = await api.delete('/api/v1/filestore/delete', {
        params: {
          path: filePath,
        }
      });
      
      if (response) {
        snackbar.success(`File "${fileName}" deleted successfully`);
        
        // Refresh the file list
        const dirFiles = await loadFiles(source.source.filestore.path);
        setDirectoryFiles(prev => ({
          ...prev,
          [index]: dirFiles
        }));
      } else {
        snackbar.error(`Failed to delete file "${fileName}"`);
      }
    } catch (error) {
      console.error('Error deleting file:', error);
      snackbar.error('An error occurred while deleting the file');
    } finally {
      // Clear deleting state for this file
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
                  {/* Upload status and cancel button */}
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
                  
                  {/* Progress percentage and size info */}
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography variant="body1" color="common.white" fontWeight="medium">
                      {localUploadProgress.percent}% Complete
                    </Typography>
                    <Typography variant="body2" color="rgba(255, 255, 255, 0.7)">
                      {prettyBytes(localUploadProgress.uploadedBytes)} of {prettyBytes(localUploadProgress.totalBytes)}
                    </Typography>
                  </Box>
                  
                  {/* Main progress bar */}
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
                  
                  {/* ETA and speed info */}
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

            {/* Display existing files */}
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
                        <Typography variant="caption" sx={{ flexGrow: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {file.name}
                        </Typography>
                        <Typography variant="caption" sx={{ ml: 2, color: 'text.secondary', minWidth: '60px', textAlign: 'right' }}>
                          {prettyBytes(file.size || 0)}
                        </Typography>
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
                  <Button
                    sx={{ mt: 1 }}
                    size="small"
                    startIcon={<RefreshIcon />}
                    onClick={() => loadDirectoryContents(source.source.filestore?.path || '', index)}
                  >
                    Refresh File List
                  </Button>
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