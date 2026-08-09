import { FC, useEffect, useRef, useState } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import { Bot, Braces, Network } from 'lucide-react'

import { ApiCreateBotRequest } from '../../api/api'
import {
  CodeAgentRuntime,
  ICreateAgentParams,
} from '../../contexts/apps'
import useApps from '../../hooks/useApps'
import { useCodexSubscriptions } from '../../services/codexSubscriptionsService'
import { useCreateBot, useListHelixOrgBots } from '../../services/helixOrgService'
import {
  AGENT_KIND_CODING,
  AGENT_KIND_HELIX,
  AGENT_KIND_ORG,
  AGENT_TYPE_ZED_EXTERNAL,
} from '../../types'
import AgentConfigForm, { AgentConfigValue } from '../helix-org/BotRuntimeForm'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import {
  DEFAULT_CLAUDE_SUBSCRIPTION_MODEL,
  DEFAULT_CODEX_SUBSCRIPTION_MODEL,
} from './CodingAgentForm'

const kindOptions = [
  {
    value: AGENT_KIND_CODING,
    label: 'Coding Agent',
    description: 'External coding harness for projects and spec tasks',
    icon: Braces,
    color: '#38bdf8',
  },
  {
    value: AGENT_KIND_HELIX,
    label: 'Helix Agent',
    description: 'Native agent for chat, tools, and automations',
    icon: Bot,
    color: '#10b981',
  },
  {
    value: AGENT_KIND_ORG,
    label: 'Helix Org Agent',
    description: 'Worker managed through the organization chart',
    icon: Network,
    color: '#a78bfa',
  },
]

const emptyRuntimeConfig = (): AgentConfigValue => ({
  runtime: 'zed_agent',
  credentials: 'api_key',
  provider: '',
  model: '',
  reasoning_effort: 'none',
})

export const preferredSubscriptionRuntimeConfig = (
  codexSubscriptionCount: number,
  claudeSubscriptionCount: number,
): AgentConfigValue | undefined => {
  if (codexSubscriptionCount > 0) {
    return {
      ...emptyRuntimeConfig(),
      runtime: 'codex_cli',
      credentials: 'subscription',
      model: DEFAULT_CODEX_SUBSCRIPTION_MODEL,
    }
  }

  if (claudeSubscriptionCount > 0) {
    return {
      ...emptyRuntimeConfig(),
      runtime: 'claude_code',
      credentials: 'subscription',
      model: DEFAULT_CLAUDE_SUBSCRIPTION_MODEL,
    }
  }
}

const slugify = (value: string): string =>
  value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')

type Props = {
  open: boolean
  initialKind: string
  onClose: () => void
  onCreated: (kind: string, id: string) => void
}

const NewAgentDialog: FC<Props> = ({ open, initialKind, onClose, onCreated }) => {
  const apps = useApps()
  const createOrgAgent = useCreateBot()
  const { data: orgAgents } = useListHelixOrgBots({ enabled: open })
  const { data: claudeSubscriptions, isFetched: claudeSubscriptionsFetched } = useClaudeSubscriptions()
  const { data: codexSubscriptions, isFetched: codexSubscriptionsFetched } = useCodexSubscriptions()
  const [name, setName] = useState('')
  const [kind, setKind] = useState(initialKind)
  const [runtimeConfig, setRuntimeConfig] = useState<AgentConfigValue>(emptyRuntimeConfig)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const initializedRuntime = useRef(false)
  const claudeSubscriptionCount = claudeSubscriptions?.length ?? 0
  const codexSubscriptionCount = codexSubscriptions?.length ?? 0

  useEffect(() => {
    if (!open) return
    setName('')
    setKind(initialKind)
    setRuntimeConfig(emptyRuntimeConfig())
    setCreating(false)
    setError('')
    initializedRuntime.current = false
  }, [open, initialKind])

  useEffect(() => {
    if (!open || initializedRuntime.current) return

    const preferredRuntime = preferredSubscriptionRuntimeConfig(
      codexSubscriptionCount,
      claudeSubscriptionCount,
    )
    if (preferredRuntime) {
      setRuntimeConfig(preferredRuntime)
      initializedRuntime.current = true
      return
    }

    if (codexSubscriptionsFetched && claudeSubscriptionsFetched) {
      initializedRuntime.current = true
    }
  }, [
    open,
    initialKind,
    codexSubscriptionCount,
    claudeSubscriptionCount,
    codexSubscriptionsFetched,
    claudeSubscriptionsFetched,
  ])

  const trimmedName = name.trim()
  const needsRuntime = kind === AGENT_KIND_CODING || kind === AGENT_KIND_ORG
  const runtimeComplete = runtimeConfig.credentials === 'subscription'
    ? !!runtimeConfig.model
    : !!runtimeConfig.provider && !!runtimeConfig.model
  const canCreate = !!trimmedName && (!needsRuntime || runtimeComplete)

  const appParams = (): ICreateAgentParams => ({
    name: trimmedName,
    description: kind === AGENT_KIND_CODING ? 'Code development agent for projects and spec tasks' : '',
    systemPrompt: '',
    ...(kind === AGENT_KIND_CODING ? {
      agentType: AGENT_TYPE_ZED_EXTERNAL,
      codeAgentRuntime: runtimeConfig.runtime as CodeAgentRuntime,
      codeAgentCredentialType: runtimeConfig.credentials as 'api_key' | 'subscription',
      claudeSubscriptionModel: runtimeConfig.runtime === 'claude_code'
        && runtimeConfig.credentials === 'subscription'
        ? runtimeConfig.model
        : undefined,
      provider: runtimeConfig.provider,
      model: runtimeConfig.runtime === 'claude_code'
        && runtimeConfig.credentials === 'subscription'
        ? ''
        : runtimeConfig.model,
      reasoningEffort: runtimeConfig.reasoning_effort,
    } : {}),
    reasoningModelProvider: '',
    reasoningModel: '',
    reasoningModelEffort: 'none',
    generationModelProvider: '',
    generationModel: '',
    smallReasoningModelProvider: '',
    smallReasoningModel: '',
    smallReasoningModelEffort: 'none',
    smallGenerationModelProvider: '',
    smallGenerationModel: '',
  })

  const create = async () => {
    if (!canCreate || creating) return
    setCreating(true)
    setError('')
    try {
      if (kind === AGENT_KIND_ORG) {
        const baseID = slugify(trimmedName) || 'agent'
        const existingIDs = new Set((orgAgents ?? []).map((agent) => agent.id))
        let id = baseID
        for (let suffix = 2; existingIDs.has(id); suffix += 1) {
          id = `${baseID}-${suffix}`
        }
        const payload = {
          id,
          name: trimmedName,
          content: `# ${trimmedName}`,
          code_agent_runtime: runtimeConfig.runtime as ApiCreateBotRequest['code_agent_runtime'],
          code_agent_credential_type: runtimeConfig.credentials as ApiCreateBotRequest['code_agent_credential_type'],
          provider: runtimeConfig.provider,
          model: runtimeConfig.model,
          reasoning_effort: runtimeConfig.reasoning_effort,
        } satisfies ApiCreateBotRequest
        const created = await createOrgAgent.mutateAsync(payload)
        await apps.loadApps()
        onCreated(kind, created.id ?? id)
      } else {
        const created = await apps.createAgent(appParams())
        if (!created?.id) throw new Error('Agent creation returned no ID')
        onCreated(kind, created.id)
      }
    } catch (cause: any) {
      setError(cause?.response?.data?.error ?? cause?.message ?? 'Failed to create agent')
      setCreating(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={creating ? undefined : onClose}
      fullWidth
      maxWidth="sm"
      PaperProps={{
        sx: {
          width: 600,
          maxHeight: 'calc(100dvh - 16px)',
          m: { xs: 1, sm: 2 },
        },
      }}
    >
      <DialogTitle sx={{ px: { xs: 2, sm: 3 }, py: 2 }}>New Agent</DialogTitle>
      <DialogContent dividers sx={{ px: { xs: 2, sm: 3 }, py: 2.5 }}>
        <Stack spacing={2.5} sx={{ width: '100%', maxWidth: 536, mx: 'auto' }}>
          <TextField
            label="Agent name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Software Developer"
            autoFocus
            fullWidth
            size="small"
          />

          <Box>
            <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
              Agent kind
            </Typography>
            <Box
              sx={{
                display: 'grid',
                gridTemplateColumns: { xs: '1fr', sm: 'repeat(3, minmax(0, 1fr))' },
                gap: 1.25,
                width: '100%',
              }}
            >
              {kindOptions.map((option) => {
                const Icon = option.icon
                const selected = kind === option.value
                return (
                  <Button
                    key={option.value}
                    variant="outlined"
                    color="inherit"
                    onClick={() => setKind(option.value)}
                    aria-pressed={selected}
                    sx={{
                      textTransform: 'none',
                      minWidth: 0,
                      minHeight: { xs: 88, sm: 116 },
                      flexDirection: 'column',
                      justifyContent: 'center',
                      textAlign: 'center',
                      px: 1.25,
                      py: 1.5,
                      borderColor: selected ? 'secondary.main' : 'divider',
                      backgroundColor: selected ? 'action.selected' : 'transparent',
                      '&:hover': {
                        borderColor: selected ? 'secondary.main' : 'text.secondary',
                        backgroundColor: selected ? 'action.selected' : 'action.hover',
                      },
                    }}
                  >
                    <Icon size={18} color={option.color} />
                    <Box sx={{ mt: 0.75 }}>
                      <Typography color="text.primary" sx={{ fontSize: '0.78rem', fontWeight: 600, lineHeight: 1.25 }}>
                        {option.label}
                      </Typography>
                      <Typography color="text.secondary" sx={{ display: { xs: 'none', sm: 'block' }, mt: 0.25, fontSize: '0.66rem', lineHeight: 1.25 }}>
                        {option.description}
                      </Typography>
                    </Box>
                  </Button>
                )
              })}
            </Box>
          </Box>

          {needsRuntime && (
            <AgentConfigForm
              value={runtimeConfig}
              onChange={(patch) => {
                initializedRuntime.current = true
                setRuntimeConfig((current) => ({ ...current, ...patch }))
              }}
              showReasoningEffort
            />
          )}

          {kind === AGENT_KIND_HELIX && (
            <Typography variant="body2" color="text.secondary">
              After creation, you’ll continue in the full agent editor to configure instructions, tools, and triggers.
            </Typography>
          )}
          {error && <Alert severity="error">{error}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: { xs: 2, sm: 3 }, py: 1.25 }}>
        <Button onClick={onClose} disabled={creating}>Cancel</Button>
        <Button onClick={create} variant="contained" disabled={!canCreate || creating}>
          {creating ? 'Creating…' : 'Create Agent'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default NewAgentDialog
