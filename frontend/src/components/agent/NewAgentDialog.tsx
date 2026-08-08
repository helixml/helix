import { FC, useEffect, useState } from 'react'
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
import { useCreateBot, useListHelixOrgBots } from '../../services/helixOrgService'
import {
  AGENT_KIND_CODING,
  AGENT_KIND_HELIX,
  AGENT_KIND_ORG,
  AGENT_TYPE_ZED_EXTERNAL,
} from '../../types'
import AgentConfigForm, { AgentConfigValue } from '../helix-org/BotRuntimeForm'

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
  const [name, setName] = useState('')
  const [kind, setKind] = useState(initialKind)
  const [runtimeConfig, setRuntimeConfig] = useState<AgentConfigValue>(emptyRuntimeConfig)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setName('')
    setKind(initialKind)
    setRuntimeConfig(emptyRuntimeConfig())
    setCreating(false)
    setError('')
  }, [open, initialKind])

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
          height: 600,
          maxHeight: 'calc(100% - 64px)',
        },
      }}
    >
      <DialogTitle>New Agent</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2.5}>
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
            <Stack spacing={1}>
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
                      justifyContent: 'flex-start',
                      textAlign: 'left',
                      px: 1.5,
                      py: 1.25,
                      borderColor: selected ? 'secondary.main' : 'divider',
                      backgroundColor: selected ? 'action.selected' : 'transparent',
                      '&:hover': {
                        borderColor: selected ? 'secondary.main' : 'text.secondary',
                        backgroundColor: selected ? 'action.selected' : 'action.hover',
                      },
                    }}
                  >
                    <Icon size={18} color={option.color} />
                    <Box sx={{ ml: 1.5 }}>
                      <Typography variant="body2" color="text.primary" sx={{ fontWeight: 600, lineHeight: 1.3 }}>
                        {option.label}
                      </Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.3 }}>
                        {option.description}
                      </Typography>
                    </Box>
                  </Button>
                )
              })}
            </Stack>
          </Box>

          {needsRuntime && (
            <AgentConfigForm
              value={runtimeConfig}
              onChange={(patch) => setRuntimeConfig((current) => ({ ...current, ...patch }))}
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
      <DialogActions>
        <Button onClick={onClose} disabled={creating}>Cancel</Button>
        <Button onClick={create} variant="contained" disabled={!canCreate || creating}>
          {creating ? 'Creating…' : 'Create Agent'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default NewAgentDialog
