import { FC, useCallback, useMemo, useRef } from 'react'
import Box from '@mui/material/Box'
import { alpha } from '@mui/material/styles'

import { TypesInteractionState } from '../../api/api'
import { useStreaming } from '../../contexts/streaming'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useListInteractions } from '../../services/sessionService'
import { SESSION_TYPE_TEXT } from '../../types'
import RobustPromptInput from '../common/RobustPromptInput'
import EmbeddedSessionView, { EmbeddedSessionViewHandle } from './EmbeddedSessionView'
import SessionPromptQueue from './SessionPromptQueue'
import { CHAT_FONT_FAMILY, getChatColors } from './chatStyles'

interface AgentChatProps {
  sessionId: string
  projectId?: string
  specTaskId?: string
  placeholder?: string
  disabled?: boolean
  showSessionPromptQueue?: boolean
  autoScrollOnMount?: boolean
  enableInteractionDebugCopy?: boolean
  onWillSend?: () => void
}

/** Shared org/spec-task conversation surface. */
const AgentChat: FC<AgentChatProps> = ({
  sessionId,
  projectId,
  specTaskId,
  placeholder,
  disabled,
  showSessionPromptQueue = false,
  autoScrollOnMount,
  enableInteractionDebugCopy,
  onWillSend,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)
  const apiClient = api.getApiClient()

  const { data: latestInteractionsResponse } = useListInteractions(
    sessionId,
    0,
    1,
    'desc',
    { enabled: !!sessionId, refetchInterval: 3000 },
  )
  const isAgentBusy = useMemo(
    () => latestInteractionsResponse?.data?.interactions?.[0]?.state ===
      TypesInteractionState.InteractionStateWaiting,
    [latestInteractionsResponse?.data?.interactions?.[0]?.state],
  )

  const handleSend = useCallback(async (message: string, interrupt?: boolean) => {
    await streaming.NewInference({
      type: SESSION_TYPE_TEXT,
      message,
      sessionId,
      interrupt: interrupt ?? true,
    })
    // streaming is a context object; sessionId is the only value that should
    // rebind this callback.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId])

  const handleCancel = useCallback(async () => {
    try {
      await api.getApiClient().v1SessionsCancelCreate(sessionId)
    } catch (error: any) {
      snackbar.error(error?.message || 'Failed to interrupt current turn')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId])

  const handleFileUpload = useCallback(async (file: File): Promise<string | null> => {
    try {
      const response = await api.getApiClient().v1ExternalAgentsUploadCreate(
        sessionId,
        { file },
        { open_file_manager: false },
      )
      if (!response.data?.path) return null
      return response.data.path
    } catch (error) {
      console.error('Chat attachment upload failed:', error)
      snackbar.error(`Failed to upload ${file.name}`)
      return null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId])

  return (
    <Box
      data-agent-chat
      sx={{
        flex: 1,
        minHeight: 0,
        minWidth: 0,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: CHAT_FONT_FAMILY,
        color: (theme) => getChatColors(theme).foreground,
        backgroundColor: (theme) => getChatColors(theme).canvas,
      }}
    >
      <Box sx={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex' }}>
        <EmbeddedSessionView
          ref={sessionViewRef}
          sessionId={sessionId}
          autoScrollOnMount={autoScrollOnMount}
          enableInteractionDebugCopy={enableInteractionDebugCopy}
        />
      </Box>

      {showSessionPromptQueue && <SessionPromptQueue sessionId={sessionId} />}

      <Box
        sx={{
          flexShrink: 0,
          px: { xs: 1.5, sm: 2.5 },
          pt: 1.25,
          pb: { xs: 1.25, sm: 1.75 },
          borderTop: '1px solid',
          borderColor: (theme) => getChatColors(theme).border,
          backgroundColor: (theme) => alpha(getChatColors(theme).canvas, 0.96),
        }}
      >
        <Box sx={{ width: '100%', maxWidth: 768, mx: 'auto' }}>
          <RobustPromptInput
            sessionId={sessionId}
            specTaskId={specTaskId}
            projectId={projectId}
            apiClient={apiClient}
            onSend={handleSend}
            onWillSend={onWillSend}
            onHeightChange={() => sessionViewRef.current?.scrollToBottom()}
            onFileUpload={handleFileUpload}
            onCancel={handleCancel}
            isAgentBusy={isAgentBusy}
            placeholder={placeholder}
            disabled={disabled}
          />
        </Box>
      </Box>
    </Box>
  )
}

export default AgentChat
