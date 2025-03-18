import React, { useRef, useState } from 'react';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Avatar from '@mui/material/Avatar';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import SendIcon from '@mui/icons-material/Send';
import CircularProgress from '@mui/material/CircularProgress';
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

import Interaction from '../session/Interaction';
import InteractionLiveStream from '../session/InteractionLiveStream';

import { ISession, ISessionRAGResult, IKnowledgeSearchResult } from '../../types';

interface PreviewPanelProps {
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
}

const PreviewPanel: React.FC<PreviewPanelProps> = ({
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
  serverConfig,
  themeConfig,
  snackbar,
}) => {
  const textFieldRef = useRef<HTMLTextAreaElement>();
  const [selectedChunk, setSelectedChunk] = useState<ISessionRAGResult | null>(null);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if (event.shiftKey) {
        setInputValue(inputValue + "\n");
      } else {
        onInference();
      }
      event.preventDefault();
    }
  };

  const handleFilterDocument = (docId: string) => {
    setInputValue(`[DOC_ID:${docId}] `);
  };

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

  return (
    <Grid item xs={12} md={6}
      sx={{
        position: 'relative',
        backgroundImage: `url(${image || '/img/app-editor-swirl.webp'})`,
        backgroundPosition: 'top',
        backgroundRepeat: 'no-repeat',
        backgroundSize: image ? 'cover' : 'auto',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        borderRight: '1px solid #303047',
        borderBottom: '1px solid #303047',
      }}
    >
      {image && (
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
      {/* Fixed Preview Bar */}
      <Box
        sx={{
          p: 2,
          flexShrink: 0,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          position: 'relative',
          zIndex: 2,
          marginLeft: "-15px",
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
        }}
      >
        <Typography variant="h6" sx={{ mb: 2, color: 'white' }}>
          Preview
        </Typography>
        <Avatar
          src={avatar}
          sx={{
            width: 80,
            height: 80,
            mb: 2,
            border: '2px solid #fff',
          }}
        />
        <FormControlLabel
          control={
            <Switch
              checked={isSearchMode}
              onChange={handleSearchModeChange}
              color="primary"
            />
          }
          label={isSearchMode ? `Search ${name || 'Helix'} knowledge` : `Message ${name || 'Helix'}`}
          sx={{ mb: 2, color: 'white' }}
        />
        <Box
          sx={{
            width: '100%',
            flexGrow: 0,
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <TextField
            id="textEntry"
            fullWidth
            inputRef={textFieldRef}
            autoFocus
            label={isSearchMode ? `Search ${name || 'Helix'} knowledge` : `Message ${name || 'Helix'}`}
            helperText={isSearchMode ? "" : "Prompt the assistant with a message, integrations and scripts are selected based on their descriptions"}
            value={inputValue}
            onChange={(e) => {
              setInputValue(e.target.value);
              if (isSearchMode) {
                onSearch(e.target.value);
              }
            }}
            multiline={!isSearchMode}
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
          {!isSearchMode && (
            <Button
              id="sendButton"
              variant='contained'
              onClick={onInference}
              sx={{
                color: themeConfig.darkText,
                ml: 2,
                mb: 3,
              }}
              endIcon={loading ? <CircularProgress size={16} /> : <SendIcon />}
              disabled={loading}
            >
              Send
            </Button>
          )}
        </Box>
      </Box>
      {/* Scrollable Results Area */}
      <Box
        sx={{
          position: 'relative',
          zIndex: 2,
          flexGrow: 1,
          overflowY: 'auto',
          p: 2,
          marginLeft: "-15px",
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
        }}
      >
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
        ) : (
          session && (
            <>
              {
                session.interactions.map((interaction: any, i: number) => {
                  const interactionsLength = session.interactions.length || 0;
                  const isLastInteraction = i == interactionsLength - 1;
                  const isLive = isLastInteraction && !interaction.finished;

                  if (!session) return null;
                  return (
                    <Interaction
                      key={i}
                      serverConfig={serverConfig}
                      interaction={interaction}
                      session={session}
                      onFilterDocument={handleFilterDocument}
                    >
                      {
                        isLive && (
                          <InteractionLiveStream
                            session_id={session.id}
                            interaction={interaction}
                            session={session}
                            serverConfig={serverConfig}
                            onFilterDocument={handleFilterDocument}
                          />
                        )
                      }
                    </Interaction>
                  );
                })
              }
            </>
          )
        )}
      </Box>
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