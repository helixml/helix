import { FC } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import CodeAgentHarnessRow, { HarnessHealth } from '../providers/CodeAgentHarnessRow'
import ClaudeSubscriptionConnect, { useClaudeSubscriptions } from './ClaudeSubscriptionConnect'
import CodexSubscriptionConnect from './CodexSubscriptionConnect'
import { useCodexSubscriptions } from '../../services/codexSubscriptionsService'
import { formatClaudeAccountIdentity } from './claudeSubscriptionUtils'
import { formatCodexAccountIdentity } from './codexSubscriptionUtils'
import useLightTheme from '../../hooks/useLightTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

// Account settings shows the same expandable harness rows as the org providers
// page, so a subscription looks and reads the same wherever you meet it. The
// difference is scope: enabling a harness is an org decision, so these rows
// carry no toggle — only the credential you personally hold.
const AccountSubscriptions: FC = () => {
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const panelBg = lightTheme.isLight ? lightTheme.panelColor : themeConfig.darkPanel

  const { data: claudeSubscriptions, isLoading: claudeLoading } = useClaudeSubscriptions()
  const { data: codexSubscriptions, isLoading: codexLoading } = useCodexSubscriptions()

  const claudeSub = claudeSubscriptions?.find((sub) => sub.owner_type === 'user')
  const codexSub = codexSubscriptions?.find((sub) => sub.owner_type === 'user')

  // Helix refreshes OAuth tokens in the background, so a timestamp in the past
  // is normal and self-healing. The server's status is the only signal that
  // means the credential actually needs attention.
  const claudeIsBroken = !!claudeSub && claudeSub.status !== 'active'

  // The identity Anthropic reported for the token — account email when the
  // profile fetch succeeded, else the verified Claude org, else the plan alone.
  const claudeIdentity = claudeSub
    ? formatClaudeAccountIdentity({
        accountEmail: claudeSub.account_email,
        accountName: claudeSub.account_display_name,
        organizationId: claudeSub.claude_organization_id,
        plan: claudeSub.subscription_type,
        tier: claudeSub.rate_limit_tier,
      })
    : ''

  const claudeStatus = claudeLoading
    ? 'Loading…'
    : !claudeSub
      ? 'Not connected'
      : claudeIsBroken
        ? 'Needs re-authentication'
        : claudeIdentity || 'Connected'

  const claudeHealth: HarnessHealth = !claudeSub ? 'unavailable' : claudeIsBroken ? 'attention' : 'ready'

  // The ChatGPT account OpenAI's signed id_token attests, same treatment as
  // the Claude row above.
  const codexIdentity = codexSub
    ? formatCodexAccountIdentity({
        accountEmail: codexSub.account_email,
        accountName: codexSub.account_display_name,
        accountId: codexSub.account_id,
        plan: codexSub.plan_type,
      })
    : ''

  const codexStatus = codexLoading
    ? 'Loading…'
    : !codexSub
      ? 'Not connected'
      : codexIdentity || 'Connected'
  const codexHealth: HarnessHealth = codexSub ? 'ready' : 'unavailable'

  return (
    <Box sx={{ mt: 2, backgroundColor: panelBg, p: 2, borderRadius: 2 }}>
      <Typography variant="h6">Coding agent subscriptions</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        Connect a Claude or ChatGPT subscription to authenticate coding agents in your desktop
        sessions. These are subscriptions, not API keys — add API keys under AI Providers.
      </Typography>

      <CodeAgentHarnessRow
        runtime="claude_code"
        health={claudeHealth}
        status={claudeStatus}
        enabled={!!claudeSub}
      >
        <ClaudeSubscriptionConnect variant="account" embedded />
      </CodeAgentHarnessRow>

      <CodeAgentHarnessRow
        runtime="codex_cli"
        health={codexHealth}
        status={codexStatus}
        enabled={!!codexSub}
      >
        <Stack spacing={1.5} alignItems="flex-start">
          <Typography variant="body2" color="text.secondary">
            Use your ChatGPT account with Codex CLI inside desktop agents.
          </Typography>
          <CodexSubscriptionConnect />
        </Stack>
      </CodeAgentHarnessRow>
    </Box>
  )
}

export default AccountSubscriptions
