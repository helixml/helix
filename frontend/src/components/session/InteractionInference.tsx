import { FC, useState, useEffect } from 'react'
import { styled } from '@mui/system'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import ReplayIcon from '@mui/icons-material/Replay'
import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import Markdown from './Markdown'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import RefreshIcon from '@mui/icons-material/Refresh'
import TextField from '@mui/material/TextField'
import CopyButtonWithCheck from './CopyButtonWithCheck'
import ToolStepsWidget from './ToolStepsWidget'

import { ThumbsUp, ThumbsDown, Download } from 'lucide-react'

import ExportDocument from '../export/ExportDocument'
import ToPDF from '../export/ToPDF'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import { useUpdateInteractionFeedback } from '../../services/interactionsService'

import {
  emitEvent,
} from '../../utils/analytics'

import {
  IServerConfig,
} from '../../types'

import { TypesInteraction, TypesSession, TypesFeedback } from '../../api/api'

const GeneratedImage = styled('img')({
  cursor: 'pointer',
  transition: 'transform 0.2s ease-in-out',
  '&:hover': {
    transform: 'scale(1.05)',
  },
})

const ImagePreview = styled('img')({
  height: '150px',
  width: '150px',
  objectFit: 'cover',
  border: '1px solid #000000',
  borderRadius: '4px',
  cursor: 'pointer',
  transition: 'transform 0.2s ease-in-out',
  '&:hover': {
    transform: 'scale(1.05)',
  },
})

export const InteractionInference: FC<{
  imageURLs?: string[],
  message?: string,
  error?: string,
  serverConfig?: IServerConfig,
  interaction: TypesInteraction,
  session: TypesSession,
  upgrade?: boolean,
  isFromAssistant?: boolean,
  onFilterDocument?: (docId: string) => void,
  onRegenerate?: (interactionID: string, message: string) => void,
  isEditing?: boolean,
  editedMessage?: string,
  setEditedMessage?: (msg: string) => void,
  handleCancel?: () => void,
  handleSave?: () => void,
  isLastInteraction?: boolean,
  sessionSteps?: any[],
}> = ({
  imageURLs = [],
  message,
  error,
  serverConfig,
  interaction,
  session,  
  upgrade,
  isFromAssistant: isFromAssistant,
  onFilterDocument,
  onRegenerate,
  isEditing: externalIsEditing,
  editedMessage: externalEditedMessage,
  setEditedMessage: externalSetEditedMessage,
  handleCancel: externalHandleCancel,
  handleSave: externalHandleSave,
  isLastInteraction,
  sessionSteps = [],
}) => {
    const account = useAccount()
    const router = useRouter()
    const [viewingError, setViewingError] = useState(false)
    const [viewingExport, setViewingExport] = useState(false)
    const [selectedImage, setSelectedImage] = useState<string | null>(null)
    const [internalIsEditing, setInternalIsEditing] = useState(false)
    const [internalEditedMessage, setInternalEditedMessage] = useState(message || '')
    const [currentFeedback, setCurrentFeedback] = useState<TypesFeedback | undefined>(interaction.feedback)
    const isEditing = externalIsEditing !== undefined ? externalIsEditing : internalIsEditing
    const editedMessage = externalEditedMessage !== undefined ? externalEditedMessage : internalEditedMessage
    const setEditedMessage = externalSetEditedMessage || setInternalEditedMessage

    const { updateFeedback } = useUpdateInteractionFeedback(session.id || '', interaction.id || '')
    const handleCancel = externalHandleCancel || (() => { setInternalEditedMessage(message || ''); setInternalIsEditing(false) })
    const handleSave = externalHandleSave || (() => { if (onRegenerate && internalEditedMessage !== message) { onRegenerate(interaction.id || '', internalEditedMessage) } setInternalIsEditing(false) })

    const handleFeedback = async (feedback: TypesFeedback) => {
      try {
        await updateFeedback({ feedback })
        setCurrentFeedback(feedback)
      } catch (error) {
        console.error('Failed to update feedback:', error)
      }
    }

    useEffect(() => {
      setCurrentFeedback(interaction.feedback)
    }, [interaction.feedback])

    // Filter tool steps for this interaction
    const toolSteps = sessionSteps
      .filter(step => step.interaction_id === interaction.id)
      .map(step => ({
        id: step.id || '',
        icon: step.icon || '',
        name: step.name || '',
        type: step.type || '',
        message: step.message || '',
        created: step.created || '',
        details: {
          arguments: step.details?.arguments || {}
        }
      }))

    if (!serverConfig || !serverConfig.filestore_prefix) return null
    if (!interaction) return null

    const getFileURL = (url: string) => {
      if (!url) return ''
      if (!serverConfig) return ''
      if (url.startsWith('data:')) return url
      return `${serverConfig.filestore_prefix}/${url}?redirect_urls=true`
    }

    return (
      <>
        {
          serverConfig?.filestore_prefix && imageURLs
            .filter(file => {
              return account.token ? true : false
            })
            .map((imageURL: string) => {
              const useURL = getFileURL(imageURL)
              return (
                <Box
                  sx={{
                    mb: 2,
                    display: 'flex',
                    gap: 1,
                  }}
                  key={useURL}
                >
                  <ImagePreview
                    src={useURL}
                    onClick={() => setSelectedImage(useURL)}
                    alt="Preview"
                  />
                </Box>
              )
            })
        }
        {toolSteps.length > 0 && isFromAssistant && (
          <ToolStepsWidget steps={toolSteps} />
        )}
        {
          message && onRegenerate && (
            <Box
              sx={{
                my: 0.5,
                display: 'flex',
                alignItems: 'flex-start',
                position: 'relative',
                flexDirection: 'column',
                gap: 0.5,
              }}
            >
              <Box sx={{ width: '100%' }}>
                {isEditing ? (
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                    <TextField
                      multiline
                      fullWidth
                      value={editedMessage}
                      onChange={(e) => setEditedMessage(e.target.value)}
                      sx={{
                        '& .MuiInputBase-root': {
                          backgroundColor: 'rgba(255, 255, 255, 0.05)',
                          borderRadius: 1,
                        },
                      }}
                    />
                    <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
                      <Button
                        size="small"
                        onClick={handleCancel}
                        sx={{ textTransform: 'none' }}
                      >
                        Cancel
                      </Button>
                      <Button
                        size="small"
                        variant="contained"
                        onClick={handleSave}
                        sx={{ textTransform: 'none' }}
                      >
                        Save
                      </Button>
                    </Box>
                  </Box>
                ) : (
                  <>
                    <Box sx={{ 
                      position: 'relative',
                      '&:hover .action-buttons': {
                        opacity: 1
                      }
                    }}>
                      <Markdown
                        text={message}
                        session={session}
                        getFileURL={getFileURL}
                        showBlinker={false}
                        isStreaming={false}
                        onFilterDocument={onFilterDocument}
                      />
                      {isFromAssistant && (
                        <Box 
                          className="action-buttons"
                          sx={{ 
                            display: 'flex', 
                            justifyContent: 'left', 
                            alignItems: 'center', 
                            mt: 1, 
                            gap: 1,
                            opacity: isLastInteraction ? 1 : 0,
                            transition: 'opacity 0.2s ease-in-out',
                            position: 'relative'
                          }}
                        >
                          <Tooltip title="Regenerate this response">
                            <IconButton
                              onClick={() => onRegenerate(interaction.id || '', interaction.prompt_message || '')}
                              size="small"
                              className="regenerate-btn"
                              sx={theme => ({
                                mt: 0.5,
                                color: theme.palette.mode === 'light' ? '#888' : '#bbb',
                                '&:hover': {
                                  color: theme.palette.mode === 'light' ? '#000' : '#fff',
                                },
                              })}
                              aria-label="regenerate"
                            >
                              <RefreshIcon sx={{ fontSize: 20 }} />
                            </IconButton>
                          </Tooltip>

                          <CopyButtonWithCheck text={message || ''} alwaysVisible={isLastInteraction} />

                          <Tooltip title="Export to PDF">
                            <IconButton
                              onClick={() => setViewingExport(true)}
                              size="small"
                              className="export-btn"
                              sx={theme => ({
                                mt: 0.5,
                                color: theme.palette.mode === 'light' ? '#888' : '#bbb',
                                '&:hover': {
                                  color: theme.palette.mode === 'light' ? '#000' : '#fff',
                                },
                              })}
                              aria-label="export"
                            >
                              <Download size={16} />
                            </IconButton>
                          </Tooltip>
                                                      
                          <Tooltip title="Love this">
                            <IconButton
                              onClick={() => handleFeedback(TypesFeedback.FeedbackLike)}
                              size="small"
                              className="thumbs-up-btn"
                              sx={theme => ({
                                mt: 0.5,
                                color: currentFeedback === TypesFeedback.FeedbackLike 
                                  ? '#4caf50' 
                                  : theme.palette.mode === 'light' ? '#888' : '#bbb',
                                '&:hover': {
                                  color: currentFeedback === TypesFeedback.FeedbackLike 
                                    ? '#45a049'
                                    : theme.palette.mode === 'light' ? '#000' : '#fff',
                                },
                              })}
                              aria-label="thumbs up"
                            >
                              <ThumbsUp size={16} fill={currentFeedback === TypesFeedback.FeedbackLike ? '#4caf50' : 'none'} />
                            </IconButton>
                          </Tooltip>

                          <Tooltip title="Needs improvement">
                            <IconButton
                              onClick={() => handleFeedback(TypesFeedback.FeedbackDislike)}
                              size="small"
                              className="thumbs-down-btn"
                              sx={theme => ({
                                mt: 0.5,
                                color: currentFeedback === TypesFeedback.FeedbackDislike 
                                  ? '#f44336' 
                                  : theme.palette.mode === 'light' ? '#888' : '#bbb',
                                '&:hover': {
                                  color: currentFeedback === TypesFeedback.FeedbackDislike 
                                    ? '#d32f2f'
                                    : theme.palette.mode === 'light' ? '#000' : '#fff',
                                },
                              })}
                              aria-label="thumbs down"
                            >
                              <ThumbsDown size={16} fill={currentFeedback === TypesFeedback.FeedbackDislike ? '#f44336' : 'none'} />
                            </IconButton>
                          </Tooltip>
                          
                          
                          
                        </Box>
                      )}
                    </Box>
                  </>
                )}
              </Box>              
            </Box>
          )
        }
        {
          error && (
            <Row sx={{
              mt: 3,
            }}>
              <Cell grow>
                <Alert severity="error">
                  The system has encountered an error -
                  <ClickLink
                    sx={{
                      pl: 0.5,
                      pr: 0.5,
                    }}
                    onClick={() => {
                      setViewingError(true)
                    }}
                  >
                    click here
                  </ClickLink>
                  to view the details.
                </Alert>
              </Cell>
              {
                !upgrade && onRegenerate && !message && (
                  <Cell
                    sx={{
                      ml: 2,
                    }}
                  >
                    <Button
                      variant="contained"
                      color="secondary"
                      size="small"
                      endIcon={<ReplayIcon />}
                      onClick={() => onRegenerate(interaction.id || '', '')}
                    >
                      Retry
                    </Button>
                  </Cell>
                )
              }
              {
                upgrade && (
                  <Cell
                    sx={{
                      ml: 2,
                    }}
                  >
                    <Button
                      variant="contained"
                      color="secondary"
                      size="small"
                      onClick={() => {
                        emitEvent({
                          name: 'queue_upgrade_clicked'
                        })
                        router.navigate('account')
                      }}
                    >
                      Upgrade
                    </Button>
                  </Cell>
                )
              }
            </Row>
          )
        }
        {
          viewingError && (
            <TerminalWindow
              open
              title="Error"
              data={error}
              onClose={() => {
                setViewingError(false)
              }}
            />
          )
        }
        {
          viewingExport && (
            <ExportDocument
              open={viewingExport}
              onClose={() => setViewingExport(false)}
            >
              <ToPDF
                markdown={message || ''}
                onClose={() => setViewingExport(false)}
                filename={`${session.name}-${interaction.id || 'export'}.pdf`}
              />
            </ExportDocument>
          )
        }
        {
          selectedImage && (
            <Box
              sx={{
                position: 'fixed',
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                bgcolor: 'rgba(0, 0, 0, 0.8)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                zIndex: 9999,
              }}
              onClick={() => setSelectedImage(null)}
            >
              <GeneratedImage
                src={selectedImage}
                sx={{
                  maxHeight: '90vh',
                  maxWidth: '90vw',
                  objectFit: 'contain',
                }}
                onClick={(e) => e.stopPropagation()}
              />
            </Box>
          )
        }
      </>
    )
  }

export default InteractionInference