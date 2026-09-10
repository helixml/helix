// NewBotDialog is the shared "create bot" side drawer used by the Chart
// canvas (toolbar + right-click + per-node "new bot") and the Bots list's
// "+ New bot" header action. A Bot is created in one step: display name,
// generated id, content (markdown prompt), and an optional parent bot it
// reports to.

import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Collapse from '@mui/material/Collapse'
import Divider from '@mui/material/Divider'
import FormControlLabel from '@mui/material/FormControlLabel'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import { ChevronDown, MessageCircle, Wrench } from 'lucide-react'

import {
  ApiCreateBotRequest,
  TypesCodeAgentExecutionConfig,
} from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'
import useRouter from '../../hooks/useRouter'
import { appendPromptDraft } from '../../hooks/usePromptHistory'
import {
  CreateBotRequest,
  useCreateBot,
  useListHelixOrgBots,
  useListHelixOrgTools,
} from '../../services/helixOrgService'
import CodeAgentConfigPicker from '../agent/CodeAgentConfigPicker'
import BotSandboxForm, { BotSandboxValue } from './BotSandboxForm'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'
import { queueOrgBotChatDraft } from './orgBotChatDraft'
import ToolPickerDialog from './ToolPickerDialog'

export type NewBotDialogProps = {
  open: boolean
  onClose: () => void
  onCreated?: (id: string) => void
  // When set, the Reports-to field is prefilled with this parent bot id.
  presetParentId?: string
}

const slugify = (value: string): string =>
  value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')

const uniqueSlug = (base: string, existingIds: Set<string>): string => {
  if (!base || !existingIds.has(base)) return base
  for (let i = 1; i < 100; i++) {
    const candidate = `${base}-${i}`
    if (!existingIds.has(candidate)) return candidate
  }
  return base
}

const NewBotDialog: FC<NewBotDialogProps> = ({ open, onClose, onCreated, presetParentId }) => {
  const snackbar = useSnackbar()
  const router = useRouter()
  const create = useCreateBot()
  const { data: botsData } = useListHelixOrgBots({ enabled: open })

  const [name, setName] = useState('')
  const [content, setContent] = useState('')
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [parentId, setParentId] = useState(presetParentId ?? '')
  const [tools, setTools] = useState<string[]>([])
  const [toolPickerOpen, setToolPickerOpen] = useState(false)
  const [permissionLevel, setPermissionLevel] = useState<'standard' | 'manager'>('standard')
  const [agentConfig, setAgentConfig] = useState<TypesCodeAgentExecutionConfig>()
  const [sandbox, setSandbox] = useState<BotSandboxValue>({ runtime: '', vcpus: 0 })
  const [preserveContext, setPreserveContext] = useState(false)
  const { data: toolCatalogue } = useListHelixOrgTools({ enabled: open && advancedOpen })

  useEffect(() => {
    if (!open) return
    setName('')
    setContent('')
    setAdvancedOpen(false)
    setParentId(presetParentId ?? '')
    setTools([])
    setToolPickerOpen(false)
    setPermissionLevel('standard')
    setAgentConfig(undefined)
    setSandbox({ runtime: '', vcpus: 0 })
    setPreserveContext(false)
  }, [open, presetParentId])

  const bots = botsData ?? []
  // Keyed on botsData (not the freshly-allocated `bots`) so it's stable while
  // the query is loading and recomputes once, when the list arrives.
  const existingIds = useMemo(() => new Set((botsData ?? []).map((b) => b.id)), [botsData])
  const id = uniqueSlug(slugify(name), existingIds)

  const talkToChiefOfStaff = () => {
    const organizationId = router.params.org_id || ''
    if (!organizationId) return
    const chiefOfStaff = bots.find((bot) => bot.id === 'chief-of-staff')
    const draft = 'I would like to create a new bot'

    if (chiefOfStaff?.session_id) {
      appendPromptDraft(chiefOfStaff.session_id, draft)
      onClose()
      router.navigate('org_session', {
        org_id: organizationId,
        session_id: chiefOfStaff.session_id,
      })
      return
    }

    queueOrgBotChatDraft(
      organizationId,
      'chief-of-staff',
      draft,
    )
    onClose()
    router.navigate('org_bot_session', {
      org_id: organizationId,
      bot_id: 'chief-of-staff',
    })
  }

  const submit = async () => {
    if (!id) {
      snackbar.error('Bot name must contain at least one letter or number')
      return
    }
    try {
      const payload: CreateBotRequest = {
        id,
        name: name.trim(),
        content,
        ...(parentId ? { parent_id: parentId } : {}),
        ...(permissionLevel === 'manager' ? { owner: true } : { tools }),
        ...(preserveContext ? { preserve_context: true } : {}),
        ...(sandbox.runtime
          ? { sandbox_runtime: sandbox.runtime as ApiCreateBotRequest['sandbox_runtime'] }
          : {}),
        ...(sandbox.vcpus
          ? { sandbox_resource_overrides: { vcpus: sandbox.vcpus } }
          : {}),
        ...(agentConfig
          ? {
              code_agent_runtime: agentConfig.runtime,
              code_agent_credential_type: agentConfig.credential_type,
              provider: agentConfig.provider_ref,
              model: agentConfig.model,
              reasoning_effort: agentConfig.reasoning_effort,
            }
          : {}),
      }
      const res = await create.mutateAsync(payload)
      onCreated?.(res.id ?? id)
      if (parentId) {
        snackbar.success(`Org bot ${res.id ?? id} created, reporting to ${parentId}`)
      } else {
        snackbar.success(`Org bot ${res.id ?? id} created - drag an edge from a manager to set who it reports to`)
      }
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'create org bot failed')
    }
  }

  return (
    <>
      <HelixOrgSideDrawer
        open={open}
        onClose={onClose}
        title="New Org Bot"
        width={460}
        footer={(
          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button onClick={onClose} variant="text">Cancel</Button>
            <Button onClick={submit} variant="contained" color="secondary" disabled={create.isPending}>
              {create.isPending ? 'Creating…' : 'Create'}
            </Button>
          </Stack>
        )}
      >
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Create an org bot. You can change its configuration after creation.
          </Typography>
          <TextField
            label="Name"
            placeholder="Chief of Staff"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            fullWidth
            size="small"
          />
          <TextField
            label="Instructions"
            placeholder="# Engineer&#10;Builds and ships software."
            value={content}
            onChange={(e) => setContent(e.target.value)}
            multiline
            minRows={7}
            fullWidth
            size="small"
            helperText="Instructions to follow, set on every interaction."
          />

          <Box>
            <Button
              size="small"
              variant="text"
              aria-expanded={advancedOpen}
              onClick={() => setAdvancedOpen((current) => !current)}
              endIcon={(
                <ChevronDown
                  size={14}
                  style={{ transform: advancedOpen ? 'rotate(180deg)' : 'none', transition: 'transform 150ms ease' }}
                />
              )}
              sx={{ px: 0.5, minWidth: 0, color: 'text.secondary', textTransform: 'none' }}
            >
              Advanced
            </Button>
            <Collapse in={advancedOpen} unmountOnExit>
              <Stack spacing={2.5} sx={{ pt: 1.5 }}>
                {presetParentId ? (
                  <TextField
                    label="Reports to"
                    value={presetParentId}
                    InputProps={{ readOnly: true }}
                    helperText="Manager this org bot reports to."
                    fullWidth
                    size="small"
                    sx={{ '& input': { fontFamily: 'var(--helix-font-mono)' } }}
                  />
                ) : (
                  <TextField
                    select
                    label="Reports to (optional)"
                    value={parentId}
                    onChange={(e) => setParentId(e.target.value)}
                    helperText="Manager this org bot reports to. Leave blank to wire it later in the chart."
                    fullWidth
                    size="small"
                  >
                    <MenuItem value="">(none)</MenuItem>
                    {bots.map((bot) => (
                      <MenuItem key={bot.id} value={bot.id ?? ''}>
                        {bot.name || bot.id}
                      </MenuItem>
                    ))}
                  </TextField>
                )}

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 0.75 }}>Harness</Typography>
                  <CodeAgentConfigPicker
                    value={agentConfig}
                    onChange={(next) => setAgentConfig(next)}
                  />
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                    Uses the organization default until you configure a harness and model.
                  </Typography>
                </Box>

                <BotSandboxForm
                  value={sandbox}
                  onChange={(patch) => setSandbox((current) => ({ ...current, ...patch }))}
                  inheritLabel="Organization default"
                />

                <FormControlLabel
                  control={(
                    <Switch
                      checked={preserveContext}
                      onChange={(_event, checked) => setPreserveContext(checked)}
                    />
                  )}
                  label="Preserve context across triggers"
                />

                <Box>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Tools & permissions</Typography>
                  <TextField
                    select
                    label="Permission level"
                    value={permissionLevel}
                    onChange={(event) => setPermissionLevel(event.target.value as 'standard' | 'manager')}
                    helperText={permissionLevel === 'manager'
                      ? 'Adds every organization-management capability to the standard worker tool set. Custom tool selection is ignored.'
                      : 'Includes chat, project and repository discovery, and the complete spec-task tool set, plus additions selected below.'}
                    fullWidth
                    size="small"
                  >
                    <MenuItem value="standard">Standard</MenuItem>
                    <MenuItem value="manager">Organization manager</MenuItem>
                  </TextField>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<Wrench size={16} />}
                    onClick={() => setToolPickerOpen(true)}
                    disabled={permissionLevel === 'manager'}
                    sx={{ mt: 1.5, textTransform: 'none' }}
                  >
                    {tools.length > 0
                      ? `${tools.length} additional ${tools.length === 1 ? 'tool' : 'tools'}`
                      : 'Configure tools'}
                  </Button>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                    Human access to this bot follows organization RBAC and app access grants.
                  </Typography>
                </Box>
              </Stack>
            </Collapse>
          </Box>

          <Divider>
            <Typography variant="caption" color="text.secondary">or</Typography>
          </Divider>
          <Stack spacing={1.5} alignItems="center">
            <Typography variant="body2" color="text.secondary" align="center">
              Alternatively, tell your Chief of Staff what kind of bot you need and it will help you create one.
            </Typography>
            <Button
              variant="outlined"
              startIcon={<MessageCircle size={16} />}
              onClick={talkToChiefOfStaff}
              disabled={!router.params.org_id}
              sx={{ textTransform: 'none' }}
            >
              Talk to Chief of Staff
            </Button>
          </Stack>

        </Stack>
      </HelixOrgSideDrawer>

      <ToolPickerDialog
        open={toolPickerOpen}
        tools={toolCatalogue ?? []}
        selectedTools={tools}
        onClose={() => setToolPickerOpen(false)}
        onApply={setTools}
        helperText="Apply the capabilities to this new bot. They are saved when you create it."
      />
    </>
  )
}

export default NewBotDialog
