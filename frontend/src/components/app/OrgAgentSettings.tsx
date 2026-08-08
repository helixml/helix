import { FC, Key, useEffect, useMemo, useState } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CheckBoxIcon from '@mui/icons-material/CheckBox'
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank'
import EditOutlinedIcon from '@mui/icons-material/EditOutlined'

import ToolPickerDialog from '../helix-org/ToolPickerDialog'
import AgentConfigForm, { AgentConfigValue } from '../helix-org/BotRuntimeForm'
import useSnackbar from '../../hooks/useSnackbar'
import { useListProjects } from '../../services/projectService'
import {
  ToolDTO,
  useHelixOrgBot,
  useListBotSubscriptions,
  useListHelixOrgBots,
  useListHelixOrgTopics,
  useListHelixOrgTools,
  useSubscribeBot,
  useUnsubscribeBot,
  useUpdateBot,
  UpdateBotRequest,
} from '../../services/helixOrgService'

const OrgAgentSettings: FC<{
  agentID: string
  section: 'basics' | 'runtime' | 'tools' | 'access' | 'subscriptions'
  readOnly: boolean
  onCanonicalUpdate?: () => Promise<unknown> | void
}> = ({ agentID, section, readOnly, onCanonicalUpdate }) => {
  const snackbar = useSnackbar()
  const { data: agents = [] } = useListHelixOrgBots()
  const linkedAgent = agents.find((agent) => (agent.agent_id ?? agent.agent_app_id) === agentID)
  const { data: detail } = useHelixOrgBot(linkedAgent?.id, { enabled: !!linkedAgent?.id })
  const updateAgent = useUpdateBot()
  const { data: projects = [] } = useListProjects(linkedAgent?.organization_id, { enabled: section === 'access' && !!linkedAgent?.organization_id })
  const { data: catalogue = [] } = useListHelixOrgTools({ enabled: section === 'tools' && !!linkedAgent })
  const { data: topicsData, isLoading: topicsLoading } = useListHelixOrgTopics({ enabled: section === 'subscriptions' && !!linkedAgent })
  const { data: subscriptionsData, isLoading: subscriptionsLoading } = useListBotSubscriptions(linkedAgent?.id, { enabled: section === 'subscriptions' && !!linkedAgent })
  const subscribe = useSubscribeBot(linkedAgent?.id)
  const unsubscribe = useUnsubscribeBot(linkedAgent?.id)
  const [editingTools, setEditingTools] = useState(false)

  const agent = detail?.bot ?? linkedAgent
  const projectID = detail?.project_id
  const projectIDs = Array.from(new Set([...(agent?.project_ids ?? []), ...(projectID ? [projectID] : [])]))
  const knownTools = new Set(catalogue.map((tool) => tool.name))
  const toolOptions: ToolDTO[] = [
    ...catalogue,
    ...(agent?.tools ?? [])
      .filter((name) => !knownTools.has(name))
      .map((name) => ({ name, description: '(not in current catalogue)' })),
  ]

  const [name, setName] = useState('')
  const [runtimeConfig, setRuntimeConfig] = useState<AgentConfigValue>({
    runtime: '',
    credentials: 'api_key',
    provider: '',
    model: '',
    reasoning_effort: 'none',
  })

  useEffect(() => {
    setName(agent?.name ?? '')
    setRuntimeConfig({
      runtime: agent?.code_agent_runtime ?? '',
      credentials: agent?.code_agent_credential_type ?? 'api_key',
      provider: agent?.provider ?? '',
      model: agent?.model ?? '',
      reasoning_effort: agent?.reasoning_effort ?? 'none',
    })
  }, [agent?.name, agent?.code_agent_runtime, agent?.code_agent_credential_type, agent?.provider, agent?.model, agent?.reasoning_effort])

  const basicsDirty = useMemo(() => {
    if (!agent) return false
    return name !== (agent.name ?? '')
      || runtimeConfig.runtime !== (agent.code_agent_runtime ?? '')
      || runtimeConfig.credentials !== (agent.code_agent_credential_type ?? 'api_key')
      || runtimeConfig.provider !== (agent.provider ?? '')
      || runtimeConfig.model !== (agent.model ?? '')
      || (runtimeConfig.reasoning_effort ?? 'none') !== (agent.reasoning_effort ?? 'none')
  }, [agent, name, runtimeConfig])

  if (!agent?.id) return null

  const update = async (patch: UpdateBotRequest) => {
    try {
      await updateAgent.mutateAsync({ id: agent.id ?? '', ...patch })
      await onCanonicalUpdate?.()
      snackbar.success('Org agent updated')
    } catch (error: any) {
      snackbar.error(error?.response?.data?.error ?? error?.message ?? 'update failed')
    }
  }

  if (section === 'basics') {
    return (
      <Box sx={{ mb: 3 }}>
        <Typography variant="h5" sx={{ mb: 0.5 }}>Basics</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          Configure the org agent's name, coding harness, model, and reasoning effort.
        </Typography>
        <Stack spacing={3}>
          <TextField
            label="Agent name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            disabled={readOnly || updateAgent.isPending}
            helperText="Use a name that makes this agent easy to identify."
            fullWidth
          />
          <AgentConfigForm
            value={runtimeConfig}
            onChange={(patch) => setRuntimeConfig((current) => ({ ...current, ...patch }))}
            showReasoningEffort
            disabled={readOnly || updateAgent.isPending}
          />
          <Box>
            <Button
              variant="contained"
              disabled={readOnly || updateAgent.isPending || !basicsDirty || !name.trim() || !runtimeConfig.runtime}
              onClick={() => void update({
                name: name.trim(),
                code_agent_runtime: runtimeConfig.runtime as NonNullable<typeof agent.code_agent_runtime>,
                code_agent_credential_type: runtimeConfig.credentials as NonNullable<typeof agent.code_agent_credential_type>,
                provider: runtimeConfig.provider,
                model: runtimeConfig.model,
                reasoning_effort: runtimeConfig.reasoning_effort ?? 'none',
              })}
            >
              Save basics
            </Button>
          </Box>
        </Stack>
      </Box>
    )
  }

  if (section === 'runtime') {
    return (
      <Box sx={{ mt: 3 }}>
        <Typography variant="subtitle1">Context</Typography>
        <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
          <Typography variant="body1">Preserve context</Typography>
          <Switch
            checked={agent.preserve_context ?? false}
            onChange={(_event, checked) => void update({ preserve_context: checked })}
            disabled={readOnly || updateAgent.isPending}
            inputProps={{ 'aria-label': 'Preserve context' }}
          />
        </Stack>
        <Typography variant="body2" color="text.secondary">
          Keep this org agent's conversation context between triggered runs.
        </Typography>
      </Box>
    )
  }

  if (section === 'tools') {
    const tools = agent.tools ?? []
    return (
      <Box sx={{ mt: 3, mb: 3 }}>
        <Stack spacing={1.5}>
          <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
            <Box>
              <Typography variant="subtitle1">Org tools</Typography>
              <Typography variant="caption" color="text.secondary">
                Helix organization capabilities available to this org agent.
              </Typography>
            </Box>
            <Button
              variant="outlined"
              size="small"
              startIcon={<EditOutlinedIcon />}
              onClick={() => setEditingTools(true)}
              disabled={readOnly || updateAgent.isPending}
            >
              Edit tools
            </Button>
          </Stack>
          <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
            {[...tools].sort().map((tool) => (
              <Chip key={tool} label={tool} size="small" sx={{ fontFamily: 'monospace' }} />
            ))}
            {tools.length === 0 && (
              <Typography variant="body2" color="text.secondary">No org tools selected.</Typography>
            )}
          </Stack>
        </Stack>
        <ToolPickerDialog
          open={editingTools}
          tools={toolOptions}
          selectedTools={tools}
          onClose={() => setEditingTools(false)}
          onApply={(selectedTools) => {
            setEditingTools(false)
            void update({ tools: selectedTools })
          }}
        />
      </Box>
    )
  }

  if (section === 'subscriptions') {
    const topics = topicsData?.topics ?? []
    const subscribedIDs = new Set((subscriptionsData?.subscriptions ?? []).map((subscription) => subscription.topic_id))
    const subscribedTopics = topics.filter((topic) => subscribedIDs.has(topic.id))
    const updating = subscribe.isPending || unsubscribe.isPending

    return (
      <Box sx={{ mb: 3 }}>
        <Typography variant="h6" sx={{ mb: 0.5 }}>Topic subscriptions</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Topics that trigger this org agent.
        </Typography>
        <Autocomplete
          multiple
          disableCloseOnSelect
          disabled={readOnly || updating}
          loading={topicsLoading || subscriptionsLoading}
          options={topics}
          value={subscribedTopics}
          onChange={async (_event, value) => {
            const nextIDs = new Set(value.map((topic) => topic.id))
            const toAdd = value.filter((topic) => !subscribedIDs.has(topic.id))
            const toRemove = (subscriptionsData?.subscriptions ?? []).filter((subscription) => !nextIDs.has(subscription.topic_id))
            try {
              for (const topic of toAdd) await subscribe.mutateAsync(topic.id)
              for (const subscription of toRemove) await unsubscribe.mutateAsync(subscription.topic_id)
              if (toAdd.length || toRemove.length) snackbar.success('Topic subscriptions updated')
            } catch (error: any) {
              snackbar.error(error?.response?.data?.error ?? error?.message ?? 'subscription update failed')
            }
          }}
          getOptionLabel={(topic) => topic.id}
          isOptionEqualToValue={(a, b) => a.id === b.id}
          renderOption={(props, option, { selected }) => {
            const { key, ...liProps } = props as typeof props & { key?: Key }
            return (
              <li key={key ?? option.id} {...liProps}>
                <Checkbox
                  icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
                  checkedIcon={<CheckBoxIcon fontSize="small" />}
                  sx={{ mr: 1 }}
                  checked={selected}
                />
                <Box>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{option.id}</Typography>
                  {option.description && (
                    <Typography variant="caption" color="text.secondary">{option.description}</Typography>
                  )}
                </Box>
              </li>
            )
          }}
          renderTags={(value, getTagProps) => value.map((option, index) => {
            const { key, ...tagProps } = getTagProps({ index })
            return (
              <Chip
                key={key ?? option.id}
                {...tagProps}
                label={option.id}
                size="small"
                sx={{ fontFamily: 'monospace' }}
              />
            )
          })}
          renderInput={(params) => <TextField {...params} label="Topics" placeholder="Select topics" />}
        />
      </Box>
    )
  }

  return (
    <Box sx={{ mb: 3 }}>
      <Typography variant="h6" sx={{ mb: 0.5 }}>Project access</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Projects this org agent can use through its organization tools.
      </Typography>
      <Autocomplete
        multiple
        disableCloseOnSelect
        disabled={readOnly || updateAgent.isPending}
        options={projects}
        value={projects.filter((project) => !!project.id && projectIDs.includes(project.id))}
        onChange={(_event, value) => {
          const selected = value.map((project) => project.id).filter((id): id is string => !!id)
          void update({ project_ids: Array.from(new Set([...(projectID ? [projectID] : []), ...selected])) })
        }}
        getOptionDisabled={(project) => project.id === projectID}
        getOptionLabel={(project) => project.name || project.id || 'Unnamed project'}
        isOptionEqualToValue={(a, b) => a.id === b.id}
        renderOption={(props, option, { selected }) => {
          const { key, ...liProps } = props as typeof props & { key?: Key }
          return (
            <li key={key ?? option.id} {...liProps}>
              <Checkbox
                icon={<CheckBoxOutlineBlankIcon fontSize="small" />}
                checkedIcon={<CheckBoxIcon fontSize="small" />}
                sx={{ mr: 1 }}
                checked={selected}
              />
              <Box>
                <Typography variant="body2">{option.name || option.id}</Typography>
                {option.id === projectID && (
                  <Typography variant="caption" color="text.secondary">Own project</Typography>
                )}
              </Box>
            </li>
          )
        }}
        renderTags={(value, getTagProps) => value.map((option, index) => {
          const { key, onDelete, ...tagProps } = getTagProps({ index })
          const ownProject = option.id === projectID
          return (
            <Chip
              key={key ?? option.id}
              {...tagProps}
              onDelete={ownProject ? undefined : onDelete}
              label={`${option.name || option.id}${ownProject ? ' (own)' : ''}`}
              size="small"
            />
          )
        })}
        renderInput={(params) => <TextField {...params} placeholder="Select projects" />}
      />
    </Box>
  )
}

export default OrgAgentSettings
