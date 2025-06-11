import React, { useCallback, useRef, useState, useEffect } from 'react';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Avatar from '@mui/material/Avatar';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import IconButton from '@mui/material/IconButton';
import CloseIcon from '@mui/icons-material/Close';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import Link from '@mui/material/Link';
import Tooltip from '@mui/material/Tooltip';
import Container from '@mui/material/Container';
import Stack from '@mui/material/Stack';
import CreateIcon from '@mui/icons-material/Create';

import Interaction from '../session/Interaction';
import Session from '../../pages/Session';
import InteractionLiveStream from '../session/InteractionLiveStream';
import ConversationStarters from '../create/ConversationStarters';
import InferenceTextField from '../create/InferenceTextField';
import AppCreateHeader from '../appstore/CreateHeader';
import Cell from '../widgets/Cell';
import Row from '../widgets/Row';
import { useRouterContext } from '../../contexts/router';

import { 
  ISession, 
  ISessionRAGResult, 
  IKnowledgeSearchResult,   
  SESSION_TYPE_TEXT, 
  IApp,
  ICloneInteractionMode,
  INTERACTION_STATE_EDITING,
  INTERACTION_STATE_COMPLETE,
  INTERACTION_STATE_ERROR,  
} from '../../types';
import { TypesCreatorType } from '../../api/api';
import ContextMenuModal from '../widgets/ContextMenuModal';
import useApi from '../../hooks/useApi';
import useIsBigScreen from '../../hooks/useIsBigScreen';
import { useStreaming } from '../../contexts/streaming';
import {
  getAssistant,
  getAssistantAvatar,  
} from '../../utils/apps';

interface PreviewPanelProps {
  appId: string;
  loading: boolean;
  name: string;
  avatar: string;
  image: string;
  isSearchMode: boolean;
  setIsSearchMode: (isSearchMode: boolean) => void;
  inputValue: string;
  setInputValue: (inputValue: string) => void;
  onInference: () => void;
  onSearch: (query: string) => void;
  hasKnowledgeSources: boolean;
  searchResults: IKnowledgeSearchResult[];
  session: ISession | undefined;
  serverConfig: any;
  themeConfig: any;
  snackbar: any;
  conversationStarters?: string[];
  app?: IApp;
  activeAssistantID?: string;
  onSessionUpdate?: (session: ISession) => void;
}

const PreviewPanel: React.FC<PreviewPanelProps> = ({
  appId,
  loading,
  name,
  avatar,
  image,
  isSearchMode,
  setIsSearchMode,
  inputValue,
  setInputValue,
  onInference,
  onSearch,
  hasKnowledgeSources,
  searchResults,
  session,
  snackbar,
  conversationStarters = [],
  app,
  activeAssistantID = '0',
  onSessionUpdate,
}) => {
  const textFieldRef = useRef<HTMLTextAreaElement>();
  const scrollableRef = useRef<HTMLDivElement>(null);
  const [selectedChunk, setSelectedChunk] = useState<ISessionRAGResult | null>(null);
  const [attachedImages, setAttachedImages] = useState<File[]>([]);
  const [isInternalLoading, setIsInternalLoading] = useState(false);
  const api = useApi();
  const isBigScreen = useIsBigScreen();
  const { NewInference, setCurrentSessionId } = useStreaming();
  const [isStreaming, setIsStreaming] = useState(false);
  const router = useRouterContext();
  // const [showSession, setShowSession] = useState(false);

  const activeAssistant = app && getAssistant(app, activeAssistantID);
  const activeAssistantAvatar = activeAssistant && app ? getAssistantAvatar(app, activeAssistantID) : '';

  // Load session from URL query parameter if present
  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const sessionId = urlParams.get('sessionId') || router.params.session_id;
    
    if (sessionId && (!session || session.id !== sessionId)) {
      api.get<ISession>(`/api/v1/sessions/${sessionId}`).then((loadedSession) => {
        if (loadedSession && onSessionUpdate) {
          onSessionUpdate(loadedSession);
        }
      }).catch((error) => {
        console.error('Error loading session:', error);
        snackbar.error('Failed to load session');
      });
    }
  }, [api, session, onSessionUpdate, router.params.session_id, snackbar]);

  // Connect session to streaming system - crucial for live streaming to work
  useEffect(() => {
    if (session?.id) {
      setCurrentSessionId(session.id);
    }
  }, [session?.id, setCurrentSessionId]);

  // Handle streaming state
  useEffect(() => {
    if (!session?.interactions || session.interactions.length === 0) return;

    const lastInteraction = session.interactions[session.interactions.length - 1];
    const shouldBeStreaming = !lastInteraction.finished && 
                             lastInteraction.state !== INTERACTION_STATE_EDITING &&
                             lastInteraction.state !== INTERACTION_STATE_COMPLETE &&
                             lastInteraction.state !== INTERACTION_STATE_ERROR;

    setIsStreaming(shouldBeStreaming);
  }, [session?.interactions]);

  // Auto-scroll to bottom when session interactions change or during streaming
  const scrollToBottom = useCallback(() => {
    if (!scrollableRef.current) return;

    scrollableRef.current.scrollTo({
      top: scrollableRef.current.scrollHeight,
      behavior: 'auto' // Use 'auto' instead of 'smooth' to prevent jumpiness during streaming
    });
  }, []);

  // Auto-scroll when interactions change or content updates
  useEffect(() => {
    if (scrollableRef.current && session?.interactions) {
      scrollToBottom();
    }
  }, [session?.interactions, scrollToBottom]);

  // Auto-scroll during streaming
  useEffect(() => {
    if (isStreaming) {
      scrollToBottom();
    }
  }, [isStreaming, scrollToBottom]);

  // Add effect to handle final scroll when streaming ends
  useEffect(() => {
    if (isStreaming) return;

    // Wait for the bottom bar and final content to render
    const timer = setTimeout(() => {
      if (!scrollableRef.current) return;
      scrollableRef.current.scrollTo({
        top: scrollableRef.current.scrollHeight,
        behavior: 'auto'
      });
    }, 200);

    return () => clearTimeout(timer);
  }, [isStreaming]);

  // Handle inference - continue session if exists, otherwise create new
  const handleInference = useCallback(async () => {
    if (!inputValue.trim()) return;

    if (session && session.id) {
      // Continue existing session
      setIsInternalLoading(true);
      try {
        const messagePayloadContent = {
          content_type: "text",
          parts: [
            {
              type: "text", 
              text: inputValue,
            }
          ],
        };

        setInputValue('');
        
        const updatedSession = await NewInference({
          message: '',
          messages: [
            {
              role: TypesCreatorType.CreatorTypeUser,
              content: messagePayloadContent as any,
            }
          ],
          appId: appId,
          assistantId: activeAssistantID || undefined,
          ragSourceId: session.config?.rag_source_data_entity_id || '',
          provider: session.provider,
          modelName: session.model_name,
          loraDir: session.lora_dir,
          sessionId: session.id,
          type: SESSION_TYPE_TEXT,
        });

        // Notify parent component of session update
        if (onSessionUpdate) {
          onSessionUpdate(updatedSession);
        }
      } catch (error) {
        console.error('Error continuing conversation:', error);
        snackbar.error('Failed to send message');
        setInputValue(inputValue); // Restore input value on error
      } finally {
        setIsInternalLoading(false);
      }
    } else {
      // No existing session, use parent's callback to create new session
      onInference();
      // Show Session component after first message
      // setShowSession(true);
    }
  }, [inputValue, session, NewInference, appId, activeAssistantID, onInference, onSessionUpdate, snackbar, setInputValue]);

  // Add effect to update URL when session is created
  useEffect(() => {
    if (session?.id && !router.params.session_id) {
      router.setParams({ session_id: session.id });
    }
  }, [session?.id, router]);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        setInputValue(inputValue + "\n");
      } else {
        handleInference();
      }
      event.preventDefault();
    }
  };

  const onHandleFilterDocument = useCallback(async (docId: string) => {
    if (!appId) {
      snackbar.error('Unable to filter document, no app ID found')
      return
    }

    // Make a call to the API to get the correct format and ensure the user has access to the document
    const result = await api.getApiClient().v1ContextMenuList({
      app_id: appId || '',
    })
    if (result.status !== 200) {
      snackbar.error(`Unable to filter document, error from API: ${result.statusText}`)
      return
    }
    const filterAction = result.data?.data?.find(item => item.value?.includes(docId) && item.action_label?.toLowerCase().includes('filter'))
    if (!filterAction) {
      snackbar.error('Unable to filter document, no action found')
      return
    }
    setInputValue(filterAction.value || '');
  }, [setInputValue]);

  const handleSearchModeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newSearchMode = event.target.checked;
    setIsSearchMode(newSearchMode);
    if (newSearchMode && inputValue.trim() !== '') {
      onSearch(inputValue.trim());
    }
  };

  const handleChunkClick = (chunk: ISessionRAGResult) => {
    setSelectedChunk(chunk);
  };

  const handleCloseDialog = () => {
    setSelectedChunk(null);
  };

  const handleCopyContent = () => {
    if (selectedChunk) {
      navigator.clipboard.writeText(selectedChunk.content);
      snackbar.success('Content copied to clipboard');
    }
  };

  // Dummy handlers for interaction components
  const handleRegenerate = useCallback(async (interactionID: string, message: string) => {
    // TODO: Implement regeneration in preview mode if needed
    console.log('Regenerate not implemented in preview mode', { interactionID, message });
  }, []);

  const handleReloadSession = useCallback(async () => {
    // TODO: Implement session reload if needed
    console.log('Session reload not implemented in preview mode');
    return session;
  }, [session]);

  const handleClone = useCallback(async (mode: ICloneInteractionMode, interactionID: string): Promise<boolean> => {
    // TODO: Implement cloning if needed in preview mode
    console.log('Clone not implemented in preview mode', { mode, interactionID });
    return false;
  }, []);

  const retryFinetuneErrors = useCallback(() => {
    // TODO: Implement if needed
    console.log('Retry finetune errors not implemented in preview mode');
  }, []);

  const handleResetSession = useCallback(() => {
    // Remove session_id from URL
    router.setParams({
      session_id: '',
    });
    // Reset session state through parent callback
    if (onSessionUpdate) {
      onSessionUpdate(undefined as any);
    }
  }, [router, onSessionUpdate]);

  // Determine if we're currently loading (either from parent or internal)
  const isLoading = loading || isInternalLoading;

  // Header similar to CreateContent
  const inferenceHeader = app && (
    <Row
      id="PREVIEW_HEADER"
      sx={{
        position: 'relative',
        backgroundImage: `url(${app.config.helix.image || image || '/img/app-editor-swirl.webp'})`,
        backgroundPosition: 'top',
        backgroundRepeat: 'no-repeat',
        backgroundSize: (app.config.helix.image || image) ? 'cover' : 'auto',
        p: 0,
        minHeight: 0,
        height: '80px',
        alignItems: 'center',
        justifyContent: 'flex-start',
      }}
    >
      {(app.config.helix.image || image) && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.8)',
            zIndex: 1,
          }}
        />
      )}
      <Cell
        sx={{
          pt: 0.5,
          px: 3,
          position: 'relative',
          zIndex: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-start',
        }}
      >
        <AppCreateHeader app={app} />
      </Cell>
      <Cell
        sx={{
          position: 'absolute',
          top: 8,
          right: 16,
          zIndex: 2,
        }}
      >
        <Typography variant="caption" sx={{ color: 'white', fontWeight: 'bold' }}>
          Preview
        </Typography>
      </Cell>
    </Row>
  );

  return (
    <Grid item xs={12} md={6}
      sx={{
        position: 'relative',
        backgroundImage: !app ? `url(${image || '/img/app-editor-swirl.webp'})` : 'none',
        backgroundPosition: 'top',
        backgroundRepeat: 'no-repeat',
        backgroundSize: image ? 'cover' : 'auto',
        display: 'flex',
        flexDirection: 'column',
        borderRight: '1px solid #303047',
        borderBottom: '1px solid #303047',
        overflow: 'hidden',
        // Not sure why we need to set this to
        // 8 to achieve the same rounded corners as the
        // tab contents in App.tsx (there they are 2)
        borderTopRightRadius: 8,
        borderBottomRightRadius: 8,
      }}
    >
      {!app && image && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.8)',
            zIndex: 1,
          }}
        />
      )}
      
      {/* Header */}
      {app ? inferenceHeader : (
        <Box
          sx={{
            p: 2,
            flexShrink: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            position: 'relative',
            zIndex: 2,
            backgroundColor: 'rgba(0, 0, 0, 0.5)',
          }}
        >         
          <Avatar
            src={avatar}
            sx={{
              width: 80,
              height: 80,
              mb: 2,
              mt: 2,
              border: '2px solid #fff',
            }}
          />
        </Box>
      )}

      {/* Search Mode Toggle */}
      <Box
        sx={{
          px: 2,
          py: 1,
          flexShrink: 0,
          position: 'relative',
          zIndex: 2,
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          gap: 1,
        }}
      >
        <FormControlLabel
          control={
            <Switch
              checked={isSearchMode}
              onChange={handleSearchModeChange}
              color="primary"
            />
          }
          label={isSearchMode ? `Search ${name || 'Helix'} knowledge` : `Message ${name || 'Helix'}`}
          sx={{ color: 'white' }}
        />
        {session && !isSearchMode && (
          <Tooltip title="Start new conversation">
            <IconButton 
              onClick={handleResetSession}
              size="small"
              sx={{ 
                color: 'white',
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                }
              }}
            >
              <CreateIcon />
            </IconButton>
          </Tooltip>
        )}
      </Box>

      {/* Scrollable Results Area */}
      <Box
        ref={scrollableRef}
        sx={{
          position: 'relative',
          zIndex: 2,
          flex: '1 1 0',
          minHeight: 0,
          overflowY: isStreaming ? 'hidden' : 'auto',
          transition: 'overflow-y 0.3s ease',
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
          display: 'flex',
          flexDirection: 'column',
          // Custom scrollbar styling
          '&::-webkit-scrollbar': {
            width: '8px',
          },
          '&::-webkit-scrollbar-track': {
            background: 'rgba(255, 255, 255, 0.1)',
          },
          '&::-webkit-scrollbar-thumb': {
            background: 'rgba(255, 255, 255, 0.3)',
            borderRadius: '4px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: 'rgba(255, 255, 255, 0.5)',
          },
        }}
      >
        <Container maxWidth="lg" sx={{ py: 2, width: '100%' }}>
          {isSearchMode ? (
            hasKnowledgeSources ? (
              searchResults && searchResults.length > 0 ? (
                searchResults.map((result, index) => (
                  <Card key={index} sx={{ mb: 2, backgroundColor: 'rgba(0, 0, 0, 0.7)' }}>
                    <CardContent>
                      <Typography variant="h6" color="white">
                        Knowledge: {result.knowledge.name}
                      </Typography>
                      <Typography variant="caption" color="rgba(255, 255, 255, 0.7)">
                        Search completed in: {result.duration_ms}ms
                      </Typography>
                      {result.results.length > 0 ? (
                        result.results.map((chunk, chunkIndex) => (
                          <Tooltip title={chunk.content} arrow key={chunkIndex}>
                            <Box
                              sx={{
                                mt: 1,
                                p: 1,
                                border: '1px solid rgba(255, 255, 255, 0.3)',
                                borderRadius: '4px',
                                cursor: 'pointer',
                                '&:hover': {
                                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                                },
                              }}
                              onClick={() => handleChunkClick(chunk)}
                            >
                              <Typography variant="body2" color="white">
                                Source: {chunk.source}
                                <br />
                                Content: {chunk.content.substring(0, 50)}...
                              </Typography>
                            </Box>
                          </Tooltip>
                        ))
                      ) : (
                        <Typography variant="body2" color="white">
                          No matches found for this query.
                        </Typography>
                      )}
                    </CardContent>
                  </Card>
                ))
              ) : (
                <Typography variant="body1" color="white">No search results found.</Typography>
              )
            ) : (
              <Typography variant="body1" color="white">Add one or more knowledge sources to start searching.</Typography>
            )
          ) : (< ></>)}
        </Container>
      </Box>

      {/* Bottom Input Section - Similar to CreateContent */}
      {!isSearchMode && (
        <Box 
          sx={{ 
            flexShrink: 0,
            flexGrow: 0,
            position: 'relative',
            zIndex: 2,
            backgroundColor: 'rgba(0, 0, 0, 0.5)',            
          }}
        >
          <Container maxWidth="lg">
            <Box sx={{ py: 2 }}>
              <Row>
                <Cell flexGrow={1}>
                  <Box
                    sx={{
                      margin: '0 auto',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 2,
                    }}
                  >
                    {session ? (
                      <Box
                        sx={{
                          height: 'calc(100vh - 400px)', // Account for header and input areas
                          minHeight: '500px',
                          maxHeight: '100%',
                          overflow: 'hidden',
                          display: 'flex',
                          flexDirection: 'column',
                          position: 'relative',
                        }}
                      >
                        <Session 
                          previewMode={true}
                        />
                      </Box>
                    ) : (
                    <>
                    {conversationStarters.length > 0 && (
                        <Box sx={{ width: '100%' }}>
                          <Stack direction="row" spacing={2} justifyContent="center">
                            <ConversationStarters
                              mini={true}
                              conversationStarters={conversationStarters}
                              layout="horizontal"
                              header={false}
                              onChange={async (prompt) => {
                                setInputValue(prompt)
                                // Don't auto-trigger inference in preview mode
                              }}
                            />
                          </Stack>
                        </Box>
                      )}                                        
                      <Box sx={{ width: '100%' }}>
                        <InferenceTextField
                          appId={appId}
                          loading={isLoading}
                          type={SESSION_TYPE_TEXT}
                          focus="false"
                          value={inputValue}
                          disabled={false}
                          startAdornment={isBigScreen && (
                            activeAssistant ? (
                              activeAssistantAvatar ? (
                                <Avatar
                                  src={activeAssistantAvatar}
                                  sx={{
                                    width: '30px',
                                    height: '30px',
                                  }}
                                />
                              ) : null
                            ) : null
                          )}
                          promptLabel={activeAssistant ? `Chat with ${app?.config.helix.name || name}` : `Message ${name || 'Helix'}`}
                          onUpdate={setInputValue}
                          onInference={handleInference}
                          attachedImages={attachedImages}
                          onAttachedImagesChange={setAttachedImages}
                        />
                      </Box>
                    </>
                    )}
                  </Box>
                </Cell>
              </Row>
            </Box>
          </Container>
        </Box>
      )}

      {/* Search Mode Input */}
      {isSearchMode && (
        <Box
          sx={{
            p: 2,
            flexShrink: 0,
            flexGrow: 0,
            position: 'relative',
            zIndex: 2,
            backgroundColor: 'rgba(0, 0, 0, 0.5)',
            borderTop: '1px solid rgba(255, 255, 255, 0.1)',
          }}
        >
          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 1,
            }}
          >
            <ContextMenuModal appId={appId} textAreaRef={textFieldRef} onInsertText={(text) => {
              setInputValue(inputValue + text);
              if (isSearchMode) {
                onSearch(inputValue + text);
              }
            }} />
            <TextField
              id="searchEntry"
              fullWidth
              inputRef={textFieldRef}
              autoFocus
              label={`Search ${name || 'Helix'} knowledge`}
              value={inputValue}
              onChange={(e) => {
                setInputValue(e.target.value);
                if (isSearchMode) {
                  onSearch(e.target.value);
                }
              }}
              onKeyDown={handleKeyDown}
              disabled={isSearchMode && !hasKnowledgeSources}
              sx={{
                '& .MuiInputBase-root': {
                  backgroundColor: 'rgba(0, 0, 0, 0.9)',
                },
                '& .MuiFormHelperText-root': {
                  color: 'white',
                },
              }}
            />
          </Box>
        </Box>
      )}

      <Dialog
        open={selectedChunk !== null}
        onClose={handleCloseDialog}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          Content Details
          <IconButton
            aria-label="close"
            onClick={handleCloseDialog}
            sx={{
              position: 'absolute',
              right: 8,
              top: 8,
              color: (theme) => theme.palette.grey[500],
            }}
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent dividers>
          {selectedChunk && (
            <>
              <Typography variant="subtitle1" gutterBottom>
                Source: {selectedChunk.source.startsWith('http://') || selectedChunk.source.startsWith('https://') ? (
                  <Link href={selectedChunk.source} target="_blank" rel="noopener noreferrer">
                    {selectedChunk.source}
                  </Link>
                ) : selectedChunk.source}
              </Typography>
              <Typography variant="subtitle2" gutterBottom>
                Document ID: {selectedChunk.document_id}
              </Typography>
              <Typography variant="subtitle2" gutterBottom>
                Document Group ID: {selectedChunk.document_group_id}
              </Typography>
              <Typography variant="subtitle2" gutterBottom>
                Chunk characters: {selectedChunk.content.length}
              </Typography>
              <Typography variant="h6" gutterBottom>
                Chunk content:
              </Typography>
              <TextField
                value={selectedChunk.content}
                disabled={true}
                fullWidth
                multiline
                rows={10}
                id="content-details"
                name="content-details"
                label="Content Details"
                InputProps={{
                  style: { fontFamily: 'monospace' }
                }}
              />
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCopyContent} startIcon={<ContentCopyIcon />}>
            Copy
          </Button>
          <Button onClick={handleCloseDialog}>Close</Button>
        </DialogActions>
      </Dialog>
    </Grid>
  );
};

export default PreviewPanel;