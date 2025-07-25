import React, { FC, useMemo } from 'react'
import InteractionContainer from './InteractionContainer'
import InteractionFinetune from './InteractionFinetune'
import InteractionInference from './InteractionInference'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import EditIcon from '@mui/icons-material/Edit'
import CopyButtonWithCheck from './CopyButtonWithCheck'

import useAccount from '../../hooks/useAccount'

import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_MODE_INFERENCE,
  SESSION_CREATOR_ASSISTANT,
  SESSION_CREATOR_USER,
  ISession,
  IInteraction,
  IServerConfig,
  ICloneInteractionMode,
} from '../../types'

import {
  isImage,
} from '../../utils/filestore'

// Prop comparison function for React.memo
const areEqual = (prevProps: InteractionProps, nextProps: InteractionProps) => {
  // Compare serverConfig
  if (prevProps.serverConfig?.filestore_prefix !== nextProps.serverConfig?.filestore_prefix) {
    return false
  }

  // Compare interaction
  if (prevProps.interaction?.id !== nextProps.interaction?.id ||
    prevProps.interaction?.finished !== nextProps.interaction?.finished ||
    prevProps.interaction?.message !== nextProps.interaction?.message ||
    prevProps.interaction?.display_message !== nextProps.interaction?.display_message ||
    prevProps.interaction?.error !== nextProps.interaction?.error ||
    prevProps.interaction?.state !== nextProps.interaction?.state) {
    return false
  }

  // Compare session
  if (prevProps.session?.id !== nextProps.session?.id ||
    prevProps.session?.type !== nextProps.session?.type ||
    prevProps.session?.mode !== nextProps.session?.mode) {
    return false
  }

  // Compare other props
  if (prevProps.highlightAllFiles !== nextProps.highlightAllFiles ||
    prevProps.showFinetuning !== nextProps.showFinetuning) {
    return false
  }

  // Compare function references
  if (prevProps.retryFinetuneErrors !== nextProps.retryFinetuneErrors ||
    prevProps.onReloadSession !== nextProps.onReloadSession ||
    prevProps.onClone !== nextProps.onClone ||
    prevProps.onAddDocuments !== nextProps.onAddDocuments ||
    prevProps.onRegenerate !== nextProps.onRegenerate ||
    prevProps.onFilterDocument !== nextProps.onFilterDocument) {
    return false
  }

  return true
}

interface InteractionProps {
  serverConfig: IServerConfig;
  interaction: IInteraction;
  session: ISession;
  highlightAllFiles: boolean;
  retryFinetuneErrors: () => void;
  onReloadSession: () => Promise<any>;
  onClone: (mode: ICloneInteractionMode, interactionID: string) => Promise<boolean>;
  onAddDocuments?: () => void;
  onFilterDocument?: (docId: string) => void;
  headerButtons?: React.ReactNode;
  children?: React.ReactNode;
  isLastInteraction: boolean;
  isOwner: boolean;
  isAdmin: boolean;
  scrollToBottom?: () => void;
  appID?: string | null;
  onHandleFilterDocument?: (docId: string) => void;
  session_id: string;
  hasSubscription: boolean;
  onRegenerate?: (interactionID: string, message: string) => void;
  sessionSteps?: any[];
  showFinetuning?: boolean;
}

export const Interaction: FC<InteractionProps> = ({
  serverConfig,
  interaction,
  session,
  highlightAllFiles,
  retryFinetuneErrors,
  onReloadSession,
  onClone,
  onAddDocuments,
  onFilterDocument,
  headerButtons,
  children,
  isLastInteraction,
  isOwner,
  isAdmin,
  scrollToBottom,
  appID,
  onHandleFilterDocument,
  session_id,
  hasSubscription,
  onRegenerate,
  sessionSteps = [],
  showFinetuning = true,
}) => {
  const account = useAccount()

  // Memoize computed values
  const displayData = useMemo(() => {
    let displayMessage: string = ''
    let imageURLs: string[] = []
    let isLoading = interaction?.creator == SESSION_CREATOR_ASSISTANT && !interaction.finished
    let useMessageText = interaction ? (interaction.display_message || interaction.message || '') : ''

    if (!isLoading) {
      if (session.type == SESSION_TYPE_TEXT) {
        if (!interaction?.lora_dir) {
          if (interaction?.message) {
            displayMessage = useMessageText
          } else {
            displayMessage = interaction.status || ''
          }
        }
        // Check for images in content
        if (interaction?.content?.parts) {
          interaction.content.parts.forEach(part => {
            if (typeof part === 'object' && part !== null && 'type' in part && part.type === 'image_url' && 'image_url' in part && part.image_url?.url) {
              imageURLs.push(part.image_url.url)
            }
          })
        }

      } else if (session.type == SESSION_TYPE_IMAGE) {
        if (interaction?.creator == SESSION_CREATOR_USER) {
          displayMessage = useMessageText || ''
        }
        else {
          if (session.mode == SESSION_MODE_INFERENCE && interaction?.files && interaction?.files.length > 0) {
            imageURLs = interaction.files.filter(isImage)
          }
        }
      }
    }

    return {
      displayMessage,
      imageURLs,
      isLoading
    }
  }, [interaction, session])

  const { displayMessage, imageURLs, isLoading } = displayData
  
  const [isEditing, setIsEditing] = React.useState(false)
  const [editedMessage, setEditedMessage] = React.useState(displayMessage || '')
  const [isHovering, setIsHovering] = React.useState(false)

  const isUser = interaction?.creator == SESSION_CREATOR_USER
  const isLive = interaction?.creator == SESSION_CREATOR_ASSISTANT && !interaction.finished;

  if (!serverConfig || !serverConfig.filestore_prefix) return null

  const handleEditClick = () => setIsEditing(true)
  const handleCancel = () => {
    setEditedMessage(displayMessage || '')
    setIsEditing(false)
  }
  const handleSave = () => {
    if (onRegenerate && editedMessage !== displayMessage) {
      onRegenerate(interaction.id, editedMessage)
    }
    setIsEditing(false)
  }

  // ChatGPT-like: user messages right-aligned, bordered, with background; assistant left-aligned, no background/border
  const containerAlignment = isUser ? 'right' : 'left';
  const containerBorder = isUser;
  const containerBackground = isUser; // Only user messages get the background

  return (
    <Box
      sx={{
        mb: 0.5,
        display: 'flex',
        flexDirection: 'column',
        alignItems: isUser ? 'flex-end' : 'flex-start',
      }}
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => setIsHovering(false)}
    >
      <InteractionContainer        
        buttons={headerButtons}
        background={containerBackground}
        align={containerAlignment}
        border={containerBorder}
        isAssistant={interaction?.creator == SESSION_CREATOR_ASSISTANT}
      >
        {
          showFinetuning && (
            <InteractionFinetune
              serverConfig={serverConfig}
              interaction={interaction}
              session={session}
              highlightAllFiles={highlightAllFiles}
              retryFinetuneErrors={retryFinetuneErrors}
              onReloadSession={onReloadSession}
              onClone={onClone}
              onAddDocuments={onAddDocuments}
            />
          )
        }

        {/* Only show one of the components - no transition or overlay */}
        {isLive ? (
          children
        ) : (
          <InteractionInference
            serverConfig={serverConfig}
            session={session}
            interaction={interaction}
            imageURLs={imageURLs}
            message={displayMessage}
            error={interaction?.error}            
            upgrade={interaction.data_prep_limited}
            isFromAssistant={interaction?.creator == SESSION_CREATOR_ASSISTANT}
            onFilterDocument={onFilterDocument}
            onRegenerate={onRegenerate}
            isEditing={isEditing}
            editedMessage={editedMessage}
            setEditedMessage={setEditedMessage}
            handleCancel={handleCancel}
            handleSave={handleSave}
            isLastInteraction={isLastInteraction}
            sessionSteps={sessionSteps}
          />
        )}
      </InteractionContainer>
      {/* Edit button floating below and right-aligned, only for user messages, not editing, and message present */}
      {isUser && !isEditing && displayMessage && (
        <Box 
          sx={{ 
            width: '100%', 
            display: 'flex', 
            justifyContent: 'flex-end', 
            mt: 0.5, 
            gap: 0.5,
            opacity: isHovering ? 1 : 0,
            transition: 'opacity 0.2s ease-in-out'
          }}
        >
          <CopyButtonWithCheck text={displayMessage} alwaysVisible={isHovering} />
          <Tooltip title="Edit">
            <IconButton
              onClick={handleEditClick}
              size="small"
              sx={theme => ({
                color: theme.palette.mode === 'light' ? '#888' : '#bbb',
                '&:hover': {
                  color: theme.palette.mode === 'light' ? '#000' : '#fff',
                },
              })}
              aria-label="edit"
            >
              <EditIcon sx={{ fontSize: 20 }} />
            </IconButton>
          </Tooltip>
        </Box>
      )}
    </Box>
  )
}

export default React.memo(Interaction, areEqual)