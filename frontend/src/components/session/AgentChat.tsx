import { FC, useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import { alpha } from '@mui/material/styles'
import { ChevronRight, CircleHelp, ListTodo, UsersRound } from 'lucide-react'

import { TypesInteractionState } from '../../api/api'
import { useStreaming } from '../../contexts/streaming'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { useListInteractions } from '../../services/sessionService'
import { useRefreshSpecTaskStatus } from '../../services/specTaskService'
import { SESSION_TYPE_TEXT } from '../../types'
import RobustPromptInput from '../common/RobustPromptInput'
import ChatWelcome from './ChatWelcome'
import { composerSendMode, shouldShowWelcome } from './minimalChatLogic'
import EmbeddedSessionView, { EmbeddedSessionViewHandle } from './EmbeddedSessionView'
import type { ResponseEntry } from './InteractionInference'
import { ComposerPlanProgress, planStepsFromResponseEntries } from './PlanProgress'
import PendingQuestionCard from './PendingQuestionCard'
import SessionPromptQueue from './SessionPromptQueue'
import { useSessionPromptQueue } from './useSessionPromptQueue'
import { getChatColors } from './chatStyles'
import type { WorkspaceReviewComment } from '../workspace-inspector/workspaceReviewComments'
import type { BotDTO } from '../../services/helixOrgService'

interface AgentChatProps {
  sessionId: string
  projectId?: string
  specTaskId?: string
  placeholder?: string
  disabled?: boolean
  showSessionPromptQueue?: boolean
  enableInteractionDebugCopy?: boolean
  onWillSend?: () => void
  appendText?: string
  leadingActions?: ReactNode
  footerContent?: ReactNode
  /** Requests shown after the last message (e.g. PR proposals awaiting approval). */
  actionCards?: ReactNode
  reviewComments?: readonly WorkspaceReviewComment[]
  onRemoveReviewComment?: (commentId: string) => void
  onReviewCommentsSent?: () => void
  /**
   * Customer-facing mode, for embedding this chat in someone else's product.
   *
   * Strips the surface back to a conversation: no developer controls in the
   * composer, and the session's opening briefing is not rendered as though the
   * customer had typed it. Before anyone has said anything it shows the
   * welcome screen instead of an empty thread — the same one Helix's own new
   * chat page uses, so starting a conversation looks like starting a
   * conversation rather than like arriving in an empty log.
   */
  minimal?: boolean
  /** Heading for the welcome screen. */
  welcomeHeading?: string
  /** Optional line under the welcome heading. */
  welcomeSubheading?: string
  /** Show the welcome screen for a full-app chat while it has no interactions. */
  welcomeWhenEmpty?: boolean
  /** Prompts offered on the empty-chat welcome screen. */
  starterPrompts?: readonly string[]
  /** Fills the composer with a selected starter prompt. */
  onSelectStarter?: (prompt: string) => void
  /** Enables the org-bot profile, starters and reply identity. */
  orgBot?: BotDTO
  /** Reports the authoritative empty-thread state to the surrounding profile. */
  onWelcomeChange?: (visible: boolean) => void
}

// Inside an iframe (an embedded task) the host page owns focus.
const isFramed = typeof window !== "undefined" && window.self !== window.top

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
  appendText,
  leadingActions,
  footerContent,
  actionCards,
  reviewComments,
  onRemoveReviewComment,
  onReviewCommentsSent,
  minimal = false,
  welcomeHeading = 'What would you like to know?',
  welcomeSubheading,
  welcomeWhenEmpty = false,
  starterPrompts = [],
  onSelectStarter,
  orgBot,
  onWelcomeChange,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()
  const sessionViewRef = useRef<EmbeddedSessionViewHandle>(null)
  const [isCancelling, setIsCancelling] = useState(false)
  const [hasSentInWelcome, setHasSentInWelcome] = useState(false)
  const [composerPlanExpanded, setComposerPlanExpanded] = useState(false)
  const [dismissedPlanInteractionId, setDismissedPlanInteractionId] = useState<string | null>(null)
  const apiClient = api.getApiClient()
  // Only a spec task opens with a hidden briefing turn; an org bot instance's
  // first interaction is the customer's own message.
  const hasBriefingTurn = !!specTaskId
  const refreshSpecTaskStatus = useRefreshSpecTaskStatus(specTaskId)

  const {
    data: latestInteractionsResponse,
    isLoading: latestInteractionsLoading,
    refetch: refetchLatestInteraction,
  } = useListInteractions(
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
  // Session-keyed queue for sessions without a spec task; the spec-task
  // composer carries its own backend-backed queue instead.
  const sessionQueue = useSessionPromptQueue(showSessionPromptQueue ? sessionId : '', isAgentBusy)
  const hasSessionQueue = showSessionPromptQueue && sessionQueue.entries.length > 0
  const latestInteraction = latestInteractionsResponse?.data?.interactions?.[0]
  const latestInteractionId = latestInteraction?.id || null
  const { data: pendingQuestionsResponse } = useListInteractions(
    sessionId,
    0,
    100,
    'desc',
    {
      enabled: !!sessionId,
      refetchInterval: 3000,
      pendingQuestionOnly: true,
    },
  )
  const pendingQuestionInteractions = pendingQuestionsResponse?.data?.interactions ?? []
  const totalInteractionCount = latestInteractionsResponse?.data?.totalCount ?? 0
  const hasOrgHireBriefing = welcomeWhenEmpty
    && totalInteractionCount === 1
    && latestInteraction?.trigger === 'org_hire'
    && latestInteraction.state === TypesInteractionState.InteractionStateComplete
    && !latestInteraction.error
  // Has the customer said anything yet?
  //
  // One interaction means only the session's opening briefing exists — the
  // agent was told who it is talking to, and replied with a greeting. Nobody
  // has asked it anything, so there is no conversation to show. hasSentInWelcome
  // covers the gap between clicking send and the count catching up on the next
  // poll, which would otherwise flash the welcome screen back for a moment.
  const showWelcome = pendingQuestionInteractions.length === 0
    && (!welcomeWhenEmpty || !hasSessionQueue)
    && shouldShowWelcome(
      minimal || welcomeWhenEmpty,
      hasSentInWelcome,
      totalInteractionCount,
      latestInteractionsLoading,
      hasBriefingTurn || hasOrgHireBriefing,
    )
  useLayoutEffect(() => {
    onWelcomeChange?.(showWelcome)
    // The parent callback does not decide welcome visibility.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showWelcome])
  const composerPlanSteps = isAgentBusy
    ? planStepsFromResponseEntries(latestInteraction?.response_entries as unknown as ResponseEntry[] | undefined)
    : []
  const showComposerPlan = composerPlanSteps.length > 0
    && dismissedPlanInteractionId !== latestInteractionId

  const handleSend = useCallback(async (message: string, interrupt?: boolean) => {
    setComposerPlanExpanded(false)
    setDismissedPlanInteractionId(null)
    await streaming.NewInference({
      type: SESSION_TYPE_TEXT,
      message,
      sessionId,
      interrupt: interrupt ?? true,
    })
    setHasSentInWelcome(true)
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
      } else if (response.data?.status === 'pending') {
        snackbar.info('Cancellation queued; waiting for the agent to reconnect and acknowledge it')
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

  const sendMode = composerSendMode(minimal, !!(specTaskId && projectId))
  const composer = (
    <RobustPromptInput
      minimal={minimal}
      sendMode={sendMode}
      sessionId={sessionId}
      specTaskId={specTaskId}
      projectId={projectId}
      apiClient={apiClient}
      onSend={handleSend}
      onWillSend={() => {
        if (sendMode === 'queued') setHasSentInWelcome(true)
        onWillSend?.()
      }}
      appendText={appendText}
      onHeightChange={() => sessionViewRef.current?.scrollToBottom()}
      onFileUpload={handleFileUpload}
      onCancel={handleCancel}
      isAgentBusy={isAgentBusy}
      isCancelling={isCancelling}
      leadingActions={leadingActions}
      showContextUsage={!minimal}
      // Focusing inside an iframe scrolls the embedding page to the composer;
      // a host page (HelixOS, Gatewaze) decides where focus goes.
      autoFocus={!isFramed}
      placeholder={placeholder}
      disabled={disabled}
      enableSandboxCompletions
      reviewComments={reviewComments}
      onRemoveReviewComment={onRemoveReviewComment}
      onReviewCommentsSent={onReviewCommentsSent}
      hasAttachedHeader={
        (showComposerPlan && composerPlanExpanded) || hasSessionQueue
      }
    />
  )

  const starterIcons = [UsersRound, ListTodo, CircleHelp]
  const starterActions = starterPrompts.length > 0 && onSelectStarter ? (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: orgBot ? 1.5 : 0 }}>
      {starterPrompts.map((prompt, index) => {
        const StarterIcon = starterIcons[index] || CircleHelp
        return (
          <Button
            key={prompt}
            onClick={() => onSelectStarter(prompt)}
            disabled={disabled}
            endIcon={<ChevronRight size={orgBot ? 20 : 18} />}
            startIcon={orgBot ? (
              <Box
                component="span"
                sx={(theme) => ({
                  width: 42,
                  height: 42,
                  display: 'grid',
                  placeItems: 'center',
                  flexShrink: 0,
                  borderRadius: 1.5,
                  color: 'primary.main',
                  backgroundColor: alpha(theme.palette.primary.main, theme.palette.mode === 'light' ? 0.16 : 0.22),
                })}
              >
                <StarterIcon size={22} aria-hidden="true" />
              </Box>
            ) : undefined}
            sx={(theme) => ({
              minHeight: orgBot ? 72 : 48,
              px: orgBot ? 1.5 : 1,
              justifyContent: 'space-between',
              gap: 1.5,
              borderRadius: orgBot ? 2 : 0,
              borderTop: '1px solid',
              borderColor: 'divider',
              color: 'text.primary',
              backgroundColor: orgBot
                ? alpha(theme.palette.primary.main, theme.palette.mode === 'light' ? 0.08 : 0.1)
                : 'transparent',
              textTransform: 'none',
              fontWeight: 600,
              textAlign: 'left',
              '& .MuiButton-startIcon': { mr: 0.5 },
              '& .MuiButton-endIcon': { ml: 'auto' },
              '&:hover': orgBot ? {
                backgroundColor: alpha(theme.palette.primary.main, theme.palette.mode === 'light' ? 0.14 : 0.16),
              } : undefined,
              '&:last-of-type': { borderBottom: '1px solid', borderColor: 'divider' },
            })}
          >
            {prompt}
          </Button>
        )
      })}
    </Box>
  ) : null

  // Minimal mode owns the whole frame, including its composer.
  if (showWelcome && minimal) {
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
        }}
      >
        <ChatWelcome heading={welcomeHeading} subheading={welcomeSubheading}>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {starterActions}
            {composer}
          </Box>
        </ChatWelcome>
      </Box>
    )
  }

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
        {showWelcome ? (
          <Box
            sx={{
              width: '100%',
              px: { xs: 2, sm: 3 },
              display: 'flex',
              alignItems: orgBot ? 'flex-start' : 'center',
              overflowY: orgBot ? 'auto' : 'hidden',
            }}
          >
            <Box sx={{ width: '100%', maxWidth: orgBot ? 540 : 480, mx: 'auto', pt: orgBot ? { xs: 3, sm: 4 } : 0, pb: orgBot ? 2 : 0 }}>
              <Typography component="h2" variant="h5" sx={{ fontWeight: orgBot ? 700 : 600, mb: welcomeSubheading ? 1 : 3 }}>
                {welcomeHeading}
              </Typography>
              {welcomeSubheading && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                  {welcomeSubheading}
                </Typography>
              )}
              {starterActions}
            </Box>
          </Box>
        ) : (
          <EmbeddedSessionView
            ref={sessionViewRef}
            sessionId={sessionId}
            enableInteractionDebugCopy={enableInteractionDebugCopy}
            minimal={minimal}
            hasBriefingTurn={hasBriefingTurn}
            assistantName={orgBot?.name || orgBot?.id}
            assistantPortraitIndex={orgBot?.portrait_index}
            trailingContent={
              actionCards || pendingQuestionInteractions.length > 0 ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  {actionCards}
                  {pendingQuestionInteractions.map((interaction) =>
                    interaction.id && interaction.pending_question ? (
                      <PendingQuestionCard
                        key={interaction.id}
                        interactionId={interaction.id}
                        pendingQuestion={interaction.pending_question}
                      />
                    ) : null,
                  )}
                </Box>
              ) : undefined
            }
          />
        )}
      </Box>

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
            {showComposerPlan && (
              <ComposerPlanProgress
                steps={composerPlanSteps}
                expanded={composerPlanExpanded}
                onToggle={() => {
                  setComposerPlanExpanded((value) => !value)
                  requestAnimationFrame(() => sessionViewRef.current?.scrollToBottom())
                }}
                onDismiss={() => {
                  setDismissedPlanInteractionId(latestInteractionId)
                  setComposerPlanExpanded(false)
                }}
              />
            )}
            {hasSessionQueue && (
              <SessionPromptQueue
                sessionId={sessionId}
                entries={sessionQueue.entries}
                onRemove={sessionQueue.remove}
                onRestartAgent={sessionQueue.restartAgent}
              />
            )}
            {composer}
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
