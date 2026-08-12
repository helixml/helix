import { FC, useCallback, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import Box from '@mui/material/Box'
import { alpha } from '@mui/material/styles'

import { TypesInteractionState } from '../../api/api'
import { useStreaming } from '../../contexts/streaming'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useListInteractions } from '../../services/sessionService'
import { useRefreshSpecTaskStatus } from '../../services/specTaskService'
import { SESSION_TYPE_TEXT } from '../../types'
import RobustPromptInput from '../common/RobustPromptInput'
import EmbeddedSessionView, { EmbeddedSessionViewHandle } from './EmbeddedSessionView'
import SessionPromptQueue from './SessionPromptQueue'
import { getChatColors } from './chatStyles'
import type { WorkspaceReviewComment } from '../workspace-inspector/workspaceReviewComments'

interface AgentChatProps {
  sessionId: string
  projectId?: string
  specTaskId?: string
  placeholder?: string
  disabled?: boolean
  showSessionPromptQueue?: boolean
  enableInteractionDebugCopy?: boolean
  onWillSend?: () => void
  leadingActions?: ReactNode
  footerContent?: ReactNode
  reviewComments?: readonly WorkspaceReviewComment[]
  onRemoveReviewComment?: (commentId: string) => void
  onReviewCommentsSent?: () => void
}

/** Shared org/spec-task conversation surface. */
const AgentChat: FC<AgentChatProps> = ({
  sessionId,
  projectId,
  specTaskId,
  placeholder,
  disabled,
  showSessionPromptQueue = false,
  enableInteractionDebugCopy,
  onWillSend,
  leadingActions,
  footerContent,
  reviewComments,
  onRemoveReviewComment,
  onReviewCommentsSent,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)
  const [isCancelling, setIsCancelling] = useState(false)
  const apiClient = api.getApiClient()
  const refreshSpecTaskStatus = useRefreshSpecTaskStatus(specTaskId)

  const { data: latestInteractionsResponse, refetch: refetchLatestInteraction } = useListInteractions(
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
    void refreshSpecTaskStatus()
    // Provider state remains mounted; primitive route state selects the current
    // session and task whose queries need refreshing.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, specTaskId])

  const handleCancel = useCallback(async () => {
    if (isCancelling) return
    setIsCancelling(true)
    try {
      const response = await api.getApiClient().v1SessionsCancelCreate(sessionId)
      if (response.data?.status === 'noop') {
        snackbar.info('The agent is no longer running a turn')
      }
      await refetchLatestInteraction()
    } catch (error: any) {
      snackbar.error(error?.message || 'Failed to interrupt current turn')
    } finally {
      setIsCancelling(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, isCancelling])

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
        color: (theme) => getChatColors(theme).foreground,
        backgroundColor: (theme) => getChatColors(theme).canvas,
      }}
    >
      <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, width: '100%', overflow: 'hidden', display: 'flex' }}>
        <EmbeddedSessionView
          ref={sessionViewRef}
          sessionId={sessionId}
          enableInteractionDebugCopy={enableInteractionDebugCopy}
        />
      </Box>

      {showSessionPromptQueue && <SessionPromptQueue sessionId={sessionId} />}

      <Box
        sx={{
          flexShrink: 0,
          pl: { xs: 1.5, sm: 2.5 },
          pr: { xs: 1.5, sm: 2.5 },
          '@media (pointer: fine)': {
            pl: 4.5,
          },
          pt: 1.25,
          pb: { xs: 1.25, sm: 1.75 },
          backgroundColor: (theme) => alpha(getChatColors(theme).canvas, 0.98),
        }}
      >
        <Box sx={{ width: '100%', maxWidth: 768, mx: 'auto' }}>
          <Box sx={{ position: 'relative', zIndex: 1 }}>
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
              isCancelling={isCancelling}
              leadingActions={leadingActions}
              placeholder={placeholder}
              disabled={disabled}
              enableSandboxCompletions
              reviewComments={reviewComments}
              onRemoveReviewComment={onRemoveReviewComment}
              onReviewCommentsSent={onReviewCommentsSent}
            />
          </Box>
          {footerContent && (
            <Box
              data-chat-context-bar="true"
              sx={{
                position: 'relative',
                zIndex: 0,
                minWidth: 0,
                minHeight: 34,
                mt: -1,
                mx: { xs: 1.25, sm: 2.5 },
                px: 1.5,
                pt: 1.75,
                pb: 0.625,
                bgcolor: (theme) => getChatColors(theme).composerSurface,
                border: '1px solid',
                borderColor: (theme) => getChatColors(theme).border,
                borderRadius: '0 0 14px 14px',
                boxShadow: (theme) => theme.palette.mode === 'light'
                  ? '0 8px 18px -16px rgba(0,0,0,0.45)'
                  : 'inset 0 1px rgba(255,255,255,0.02)',
              }}
            >
              {footerContent}
            </Box>
          )}
        </Box>
      </Box>
    </Box>
  )
}

export default AgentChat
