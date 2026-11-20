// EmbedOrgBotWorkspace is an org bot's session inside an embed (HelixOS shows
// it in an iframe): the same workspace Helix's own org-bot chat uses — desktop,
// Diff, Files, Browser, Usage, Settings, terminal, Start/Stop/Restart, Keep
// Alive, pull request proposals — around the bot's chat.
//
// Embeds have no :org_id route param, so the org is passed in explicitly and
// every bot hook is given it. The model control in the composer is read-only
// here: changing it writes only this session's config, which the bot's own
// settings would not reflect and a restart could undo.

import { FC, useCallback, useState } from 'react'
import Box from '@mui/material/Box'

import AgentChat from '../session/AgentChat'
import CodeAgentExecutionControls from '../agent/CodeAgentExecutionControls'
import AgentRestartRequiredBanner from './AgentRestartRequiredBanner'
import OrgAgentSessionWorkspace from './OrgAgentSessionWorkspace'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetSessionExecutionConfig } from '../../services/sessionService'
import { TypesCodeAgentExecutionConfig } from '../../api/api'
import {
  useActivateBot,
  useHelixOrgBot,
  useRestartBotAgent,
  useStopBotAgent,
} from '../../services/helixOrgService'

interface EmbedOrgBotWorkspaceProps {
  sessionId: string
  projectId: string
  organizationId: string
  botId: string
}

const EmbedOrgBotWorkspace: FC<EmbedOrgBotWorkspaceProps> = ({ sessionId, projectId, organizationId, botId }) => {
  const snackbar = useSnackbar()
  // Polled: the workspace gates its surfaces on the sandbox status, which
  // changes underneath an open page whenever the bot starts or stops.
  const { data: botDetail } = useHelixOrgBot(botId, { orgID: organizationId, refetchInterval: 5000 })
  const bot = botDetail?.bot
  const activate = useActivateBot(organizationId)
  const stop = useStopBotAgent(organizationId)
  const restart = useRestartBotAgent(organizationId)
  const busy = activate.isPending || stop.isPending || restart.isPending

  const { data: executionConfig } = useGetSessionExecutionConfig(sessionId, true)
  const codeAgentConfig: TypesCodeAgentExecutionConfig | undefined =
    executionConfig?.code_agent_config
    || (executionConfig?.runtime && executionConfig?.model
      ? {
          runtime: executionConfig.runtime,
          credential_type: executionConfig.credential_type,
          provider_ref: executionConfig.provider_ref,
          model: executionConfig.model,
          reasoning_effort: executionConfig.reasoning_effort,
          service_tier: executionConfig.service_tier,
        }
      : undefined)

  const [appendText, setAppendText] = useState<string | undefined>(undefined)
  const [profileExpanded, setProfileExpanded] = useState(false)
  const appendToChat = useCallback((text: string) => setAppendText(text), [])

  const fail = (action: string) => (error: any) => {
    snackbar.error(error?.response?.data?.error ?? error?.message ?? `Could not ${action} the bot`)
  }

  return (
    <Box sx={{ height: '100%', minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      <AgentRestartRequiredBanner
        key={botId}
        visible={!!bot?.restart_required}
        working={false}
        busy={busy}
        onRestart={() => { restart.mutateAsync(botId).catch(fail('restart')) }}
      />
      <Box sx={{ flex: 1, minHeight: 0 }}>
        <OrgAgentSessionWorkspace
          sessionId={sessionId}
          organizationId={organizationId}
          bot={bot}
          onStart={() => { activate.mutateAsync(botId).catch(fail('start')) }}
          onStop={() => { stop.mutateAsync(botId).catch(fail('stop')) }}
          onRestart={() => { restart.mutateAsync(botId).catch(fail('restart')) }}
          lifecycleBusy={busy}
          onAppendToChat={appendToChat}
          expandedBotProfile={profileExpanded}
        >
          {/* Bounded like Session.tsx's chat: the workspace hosts its child in a
              block box, so without this the chat grows to its full content
              height and pushes the composer off-screen. */}
          <Box sx={{ height: '100%', minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <AgentChat
            sessionId={sessionId}
            projectId={projectId}
            appendText={appendText}
            orgBot={bot}
            onWelcomeChange={setProfileExpanded}
            welcomeWhenEmpty={!!bot}
            welcomeHeading="Start a conversation"
            starterPrompts={['What can you help with?', 'What are you working on?', 'What should we work on next?']}
            onSelectStarter={appendToChat}
            leadingActions={
              <CodeAgentExecutionControls value={codeAgentConfig} onChange={() => undefined} disabled compact />
            }
            placeholder={`Message ${bot?.name || botId}...`}
          />
          </Box>
        </OrgAgentSessionWorkspace>
      </Box>
    </Box>
  )
}

export default EmbedOrgBotWorkspace
