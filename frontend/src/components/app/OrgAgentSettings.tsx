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
import { Pencil, Square, SquareCheck } from 'lucide-react'

import ToolPickerDialog from '../helix-org/ToolPickerDialog'
import AgentConfigForm, { AgentConfigValue } from '../helix-org/BotRuntimeForm'
import MonacoEditor from '../widgets/MonacoEditor'
import useSnackbar from '../../hooks/useSnackbar'
import { useListProjects } from '../../services/projectService'
import {
  ToolDTO,
  BotDetailDTO,
  useListHelixOrgProcessors,
  useListHelixOrgTools,
  useUpdateBot,
  UpdateBotRequest,
} from '../../services/helixOrgService'
import { useAgentAttachments, useCreateAgentAttachment, useDeleteAgentAttachment, useTriggers } from '../../services/triggerService'

const OrgAgentSettings: FC<{
  agentID: string
  section: 'basics' | 'instructions' | 'runtime' | 'tools' | 'access' | 'subscriptions'
  readOnly: boolean
  onCanonicalUpdate?: () => Promise<unknown> | void
  embedded?: boolean
  detail?: BotDetailDTO
}> = ({ agentID, section, readOnly, onCanonicalUpdate, embedded = false, detail }) => {
  const snackbar = useSnackbar()
  const updateAgent = useUpdateBot()
  const agent = detail?.bot
  const { data: projects = [] } = useListProjects(agent?.organization_id, { enabled: section === 'access' && !!agent?.organization_id })
  const { data: catalogue = [] } = useListHelixOrgTools({ enabled: section === 'tools' && !!agent })
  const { data: triggers = [], isLoading: triggersLoading } = useTriggers()
  const { data: processors = [], isLoading: processorsLoading } = useListHelixOrgProcessors({ enabled: section === 'subscriptions' && !!agent })
  const { data: attachments = [], isLoading: attachmentsLoading } = useAgentAttachments(agent?.id)
  const attach = useCreateAgentAttachment(agent?.id)
  const detach = useDeleteAgentAttachment(agent?.id)
  const [editingTools, setEditingTools] = useState(false)

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
  const [content, setContent] = useState('')
  const [runtimeConfig, setRuntimeConfig] = useState<AgentConfigValue>({
    runtime: '',
    credentials: 'api_key',
    provider: '',
    model: '',
    reasoning_effort: 'none',
  })

  useEffect(() => {
    setName(agent?.name ?? '')
    setContent(agent?.content ?? '')
    setRuntimeConfig({
      runtime: agent?.code_agent_runtime ?? '',
      credentials: agent?.code_agent_credential_type ?? 'api_key',
      provider: agent?.provider ?? '',
      model: agent?.model ?? '',
      reasoning_effort: agent?.reasoning_effort ?? 'none',
    })
  }, [agent?.name, agent?.content, agent?.code_agent_runtime, agent?.code_agent_credential_type, agent?.provider, agent?.model, agent?.reasoning_effort])

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
      <Box sx={{ mb: embedded ? 0 : 3 }}>
        {!embedded && (<>
        <Typography variant="h5" sx={{ mb: 0.5 }}>Basics</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          Configure the org agent's name, coding harness, model, and reasoning effort.
        </Typography>
        </>)}
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

  if (section === 'instructions') {
    const instructionsDirty = content !== (agent.content ?? '')
    return (
      <Stack spacing={1.5}>
        {!embedded && (<>
          <Typography variant="h5">Instructions</Typography>
          <Typography variant="body2" color="text.secondary">
            Markdown instructions applied to every org-agent interaction.
          </Typography>
        </>)}
        <MonacoEditor
          value={content}
          onChange={setContent}
          onSave={() => {
            if (instructionsDirty) void update({ content })
          }}
          language="markdown"
          readOnly={readOnly || updateAgent.isPending}
          minHeight={220}
          maxHeight={520}
          autoHeight
          theme="helix-dark"
          options={{
            overviewRulerLanes: 0,
            overviewRulerBorder: false,
            hideCursorInOverviewRuler: true,
          }}
        />
        <Stack direction="row" alignItems="center" justifyContent="space-between">
          <Typography variant="caption" color="text.secondary">
            Markdown supported. Cmd/Ctrl+S saves immediately.
          </Typography>
          <Button
            variant="contained"
            size="small"
            disabled={readOnly || updateAgent.isPending || !instructionsDirty}
            onClick={() => void update({ content })}
          >
            Save instructions
          </Button>
        </Stack>
      </Stack>
    )
  }

  if (section === 'runtime') {
    return (
      <Box sx={{ mt: embedded ? 0 : 3 }}>
        {!embedded && <Typography variant="subtitle1">Context</Typography>}
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
      <Box sx={{ mt: embedded ? 0 : 3, mb: embedded ? 0 : 3 }}>
        <Stack spacing={1.5}>
          <Stack direction="row" alignItems="center" justifyContent={embedded ? 'flex-end' : 'space-between'} spacing={2}>
            {!embedded && <Box>
              <Typography variant="subtitle1">Org tools</Typography>
              <Typography variant="caption" color="text.secondary">
                Helix organization capabilities available to this org agent.
              </Typography>
            </Box>}
            <Button
              variant="outlined"
              size="small"
              startIcon={<Pencil size={16} />}
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
    type SourceOption = { key: string; label: string; description: string; source: { kind: string; trigger_id?: string; processor_id?: string; output_id?: string } }
    const options: SourceOption[] = [
      ...triggers.map((trigger) => ({ key: `trigger:${trigger.id}`, label: trigger.name || trigger.id!, description: `Trigger · ${trigger.kind}`, source: { kind: 'trigger', trigger_id: trigger.id } })),
      ...processors.flatMap((processor) => processor.outputs.map((output) => ({ key: `processor_output:${processor.id}:${output.id}`, label: `${processor.name} · ${output.label || output.id}`, description: `Processed event · ${output.id}`, source: { kind: 'processor_output', processor_id: processor.id, output_id: output.id } }))),
    ]
    const sourceKey = (source: { kind?: string; trigger_id?: string; processor_id?: string; output_id?: string }) => source.kind === 'trigger' ? `trigger:${source.trigger_id}` : `processor_output:${source.processor_id}:${source.output_id}`
    const attachedKeys = new Set(attachments.map((item) => sourceKey(item.source ?? {})))
    const selected = options.filter((option) => attachedKeys.has(option.key))
    const updating = attach.isPending || detach.isPending

    return (
      <Box sx={{ mb: embedded ? 0 : 3 }}>
        {!embedded && (<>
        <Typography variant="h6" sx={{ mb: 0.5 }}>Triggers</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Choose what starts this agent. You can use a Trigger directly or the result of a Processor.
        </Typography>
        </>)}
        <Autocomplete
          multiple
          disableCloseOnSelect
          disabled={readOnly || updating}
          loading={triggersLoading || processorsLoading || attachmentsLoading}
          options={options}
          value={selected}
          onChange={async (_event, value) => {
            const nextKeys = new Set(value.map((source) => source.key))
            const toAdd = value.filter((source) => !attachedKeys.has(source.key))
            const toRemove = attachments.filter((item) => !nextKeys.has(sourceKey(item.source ?? {})))
            try {
              for (const source of toAdd) await attach.mutateAsync({ source: source.source })
              for (const item of toRemove) await detach.mutateAsync(item.id!)
              if (toAdd.length || toRemove.length) snackbar.success('Triggers updated')
            } catch (error: any) {
              snackbar.error(error?.response?.data?.summary ?? error?.message ?? 'Could not update triggers')
            }
          }}
          getOptionLabel={(source) => source.label}
          isOptionEqualToValue={(a, b) => a.key === b.key}
          renderOption={(props, option, { selected }) => {
            const { key, ...liProps } = props as typeof props & { key?: Key }
            return (
              <li key={key ?? option.key} {...liProps}>
                <Checkbox
                  icon={<Square size={18} />}
                  checkedIcon={<SquareCheck size={18} />}
                  sx={{ mr: 1 }}
                  checked={selected}
                />
                <Box>
                  <Typography variant="body2">{option.label}</Typography>
                  <Typography variant="caption" color="text.secondary">{option.description}</Typography>
                </Box>
              </li>
            )
          }}
          renderTags={(value, getTagProps) => value.map((option, index) => {
            const { key, ...tagProps } = getTagProps({ index })
            return (
              <Chip
                key={key ?? option.key}
                {...tagProps}
                label={option.label}
                size="small"
              />
            )
          })}
          renderInput={(params) => <TextField {...params} label="Starts when" placeholder="Choose triggers or processed events" />}
        />
      </Box>
    )
  }

  return (
    <Box sx={{ mb: embedded ? 0 : 3 }}>
      {!embedded && (<>
      <Typography variant="h6" sx={{ mb: 0.5 }}>Project access</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Projects this org agent can use through its organization tools.
      </Typography>
      </>)}
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
                icon={<Square size={18} />}
                checkedIcon={<SquareCheck size={18} />}
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
        renderInput={(params) => (
          <TextField
            {...params}
            label={embedded ? 'Project access' : undefined}
            placeholder="Select projects"
          />
        )}
      />
    </Box>
  )
}

export default OrgAgentSettings
