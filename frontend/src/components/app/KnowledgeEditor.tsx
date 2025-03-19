import React, { FC, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import IconButton from '@mui/material/IconButton';
import Alert from '@mui/material/Alert';
import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';
import CircularProgress from '@mui/material/CircularProgress';
import Grid from '@mui/material/Grid';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import LinkIcon from '@mui/icons-material/Link';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CloseIcon from '@mui/icons-material/Close';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';

import CrawledUrlsDialog from './CrawledUrlsDialog';
import AddKnowledgeDialog from './AddKnowledgeDialog';
import FileUpload from '../widgets/FileUpload';
import KnowledgeSourceInputs from './KnowledgeSourceInputs';

import { IFileStoreItem, IKnowledgeSource } from '../../types';
import { prettyBytes } from '../../utils/format';

import useSnackbar from '../../hooks/useSnackbar';
import useAccount from '../../hooks/useAccount';
import useKnowledge from '../../hooks/useKnowledge';

interface KnowledgeEditorProps {
  appId: string;
  disabled: boolean;
  saveKnowledgeToApp: (updatedKnowledge: IKnowledgeSource[]) => Promise<void>; 
  onSaveApp: () => Promise<any>;
}

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
}

const KnowledgeEditor: FC<KnowledgeEditorProps> = ({
  appId,
  disabled,
  saveKnowledgeToApp,
  onSaveApp,
}) => {

  const [urlDialogOpen, setUrlDialogOpen] = useState(false);
  const [selectedKnowledge, setSelectedKnowledge] = useState<IKnowledgeSource | undefined>();
  const [addDialogOpen, setAddDialogOpen] = useState(false);

  const snackbar = useSnackbar()
  const account = useAccount()
  const knowledgeHelpers = useKnowledge({
    appId,
    saveKnowledgeToApp,
    onSaveApp,
  })
    
  const getSourcePreview = (source: IKnowledgeSource): string => {
    // Prioritize using the source name if available
    if (source.name && source.name.trim() !== '') {
      return source.name;
    }
    
    // Fall back to URL or path if name is not available
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
  }

  const renderKnowledgeState = (knowledge: IKnowledgeSource | undefined) => {
    if (!knowledge) return null;
    
    let color: "default" | "primary" | "secondary" | "error" | "info" | "success" | "warning" = "default";
    switch (knowledge.state.toLowerCase()) {
      case 'ready':
        color = 'success';
        break;
      case 'preparing':
        color = 'warning';
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


  const renderSourceInput = (knowledge: IKnowledgeSource) => {
    const sourceType = knowledge.source.filestore ? 'filestore' : 'web';

    return (
      <>
        {/* Component to handle all text field inputs with local state */}
        <KnowledgeSourceInputs 
          knowledge={knowledge}
          updateKnowledge={knowledgeHelpers.updateSingleKnowledge}
          disabled={disabled}
          errors={knowledgeHelpers.errors}
          onCompletePreparation={knowledgeHelpers.handleCompleteKnowledgePreparation}
        />

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
              {knowledgeHelpers.localUploadProgress ? (
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
                      Uploading {knowledgeHelpers.uploadingFileCount} {knowledgeHelpers.uploadingFileCount === 1 ? 'File' : 'Files'}
                    </Typography>
                    
                    <Button 
                      variant="outlined" 
                      color="error" 
                      size="small" 
                      onClick={knowledgeHelpers.handleCancelUpload}
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
                      {knowledgeHelpers.localUploadProgress.percent}% Complete
                    </Typography>
                    <Typography variant="body2" color="rgba(255, 255, 255, 0.7)">
                      {prettyBytes(knowledgeHelpers.localUploadProgress.uploadedBytes)} of {prettyBytes(knowledgeHelpers.localUploadProgress.totalBytes)}
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
                        width: `${knowledgeHelpers.localUploadProgress.percent}%`, 
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
                          {knowledgeHelpers.uploadEta || "Calculating..."}
                        </Typography>
                      </Box>
                    </Grid>
                    <Grid item xs={6}>
                      <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Typography variant="caption" color="rgba(255, 255, 255, 0.7)">
                          UPLOAD SPEED
                        </Typography>
                        <Typography variant="body2" color="common.white" fontWeight="medium">
                          {knowledgeHelpers.currentSpeed ? formatSpeed(knowledgeHelpers.currentSpeed) : "Calculating..."}
                        </Typography>
                      </Box>
                    </Grid>
                  </Grid>
                </Box>
              ) : (
                <>
                  <FileUpload onUpload={(files) => knowledgeHelpers.handleFileUpload(knowledge.id, files)}>
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

            {knowledgeHelpers.directoryFiles[knowledge.id]?.length > 0 && !knowledgeHelpers.localUploadProgress && (
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
                  {knowledgeHelpers.directoryFiles[knowledge.id].map((file: any, fileIndex: number) => {
                    const fileId = `${knowledge.id}-${file.name}`;
                    const isDeleting = knowledgeHelpers.deletingFiles[fileId] === true;
                    
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
                                openFileInNewTab(file, knowledge.source.filestore?.path || '');
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
                                  knowledgeHelpers.handleDeleteFile(knowledge.id, file.name);
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
                {knowledge.source.filestore?.path && (
                  <Box sx={{ display: 'flex', mt: 1 }}>
                    <Button
                      size="small"
                      startIcon={<RefreshIcon />}
                      onClick={() => knowledgeHelpers.loadDirectoryContents(knowledge.source.filestore?.path || '', knowledge.id)}
                    >
                      Refresh Files
                    </Button>
                    <Button
                      size="small"
                      startIcon={<FolderOpenIcon />}
                      onClick={() => openInFilestore(knowledge.source.filestore?.path || '')}
                      sx={{ ml: 1 }}
                    >
                      Open in Filestore
                    </Button>
                  </Box>
                )}
              </>
            )}
            
            {knowledgeHelpers.directoryFiles[knowledge.id]?.length === 0 && !knowledgeHelpers.localUploadProgress && (
              <Typography variant="caption" sx={{ color: '#999', textAlign: 'center', mt: 2, display: 'block' }}>
                {knowledge.source.filestore?.path 
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
      snackbar.error('Must be logged in to view files');
      return;
    }

    // Ensure the path is properly scoped to the app directory
    let basePath = sourcePath;
    if (!basePath.startsWith(`apps/${appId}/`)) {
      basePath = `apps/${appId}/${basePath}`;
    }

    // Construct the full URL to the file - token will be read from cookies
    const fileUrl = file.url;
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
      {knowledgeHelpers.knowledge.map((knowledge, index) => {
        const serverKnowledge = knowledgeHelpers.serverKnowledge.find((k: IKnowledgeSource) => k.id === knowledge.id) || knowledge
        return (
          <Accordion
            key={index}
            expanded={knowledgeHelpers.expanded === `panel${knowledge.id}`}
            onChange={() => {
              if(knowledgeHelpers.expanded === `panel${knowledge.id}`) {
                knowledgeHelpers.setExpanded('')
              } else {
                knowledgeHelpers.setExpanded(`panel${knowledge.id}`)
              }              
            }}
          >
            <AccordionSummary 
              expandIcon={<ExpandMoreIcon />}
              sx={{ display: 'flex', alignItems: 'center' }}
            >
              <Box sx={{ flexGrow: 1 }}>
                <Typography component="div" sx={{ display: 'flex', alignItems: 'center' }}>
                  Knowledge Source ({getSourcePreview(knowledge)})
                  {renderKnowledgeState(serverKnowledge)}
                </Typography>
                {serverKnowledge.state === 'indexing' && (
                  <>
                    {serverKnowledge.progress?.step && serverKnowledge.progress?.step !== '' ? (
                      <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                        {serverKnowledge.progress.step} {serverKnowledge.progress.progress ? `| ${serverKnowledge.progress.progress}%` : ''} {serverKnowledge.progress.message ? `| ${serverKnowledge.progress.message}` : ''} {serverKnowledge.progress.started_at ? `| elapsed: ${Math.round((Date.now() - new Date(serverKnowledge.progress.started_at).getTime()) / 1000)}s` : ''}
                      </Typography>
                    ) : (
                      <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                        Pending...
                      </Typography>
                    )}
                  </>
                )}
                <Typography variant="caption" sx={{ display: 'block', mt: 0.5 }}>
                  Version: {serverKnowledge?.version || 'N/A'}
                </Typography>
              </Box>
              {knowledge.source.web && (
                <Tooltip title="View crawled URLs">
                  <IconButton
                    onClick={(e) => {
                      e.stopPropagation();
                      setSelectedKnowledge(serverKnowledge);
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
                    knowledgeHelpers.handleRefreshKnowledge(knowledge.id)
                  }}
                  disabled={disabled}
                  sx={{ mr: 1 }}
                >
                  <RefreshIcon />
                </IconButton>
              </Tooltip>
              {serverKnowledge && serverKnowledge.state === 'preparing' && (
                <Tooltip title="Complete preparation and start indexing">
                  <IconButton
                    onClick={(e) => {
                      e.stopPropagation();
                      knowledgeHelpers.handleCompleteKnowledgePreparation(knowledge.id)
                    }}
                    disabled={disabled}
                    sx={{ mr: 1 }}
                    color="warning"
                  >
                    <PlayArrowIcon />
                  </IconButton>
                </Tooltip>
              )}
              <Tooltip title="Delete this knowledge source">
                <IconButton
                  onClick={(e) => {
                    e.stopPropagation();
                    knowledgeHelpers.handleDeleteSource(knowledge.id)
                  }}
                  disabled={disabled}
                  sx={{ mr: 1 }}
                >
                  <DeleteIcon />
                </IconButton>
              </Tooltip>
            </AccordionSummary>
            <AccordionDetails>
              {renderSourceInput(knowledge)}
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
        onAdd={knowledgeHelpers.handleAddSource}
        appId={appId}
      />
      {Object.keys(knowledgeHelpers.errors).length > 0 && (
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



// {appTools.knowledgeErrors && appTools.showErrors && (
//   <Alert severity="error" sx={{ mt: 2 }}>
//     Please specify at least one URL for each knowledge source.
//   </Alert>
// )}