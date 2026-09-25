// BotInstanceProfileSettings edits what an Org Bot's instances get: their
// default sandbox, the MCP servers they keep, the org tools they may call and
// whether the helix agent skills are linked. Instances are minimal by default
// (browser only); everything here is opt-in. Changes save as they are made
// and apply to each instance on its next sandbox start.

import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'
import { Pencil } from 'lucide-react'

import { TypesSandboxRuntime } from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetProject } from '../../services/projectService'
import {
  BotDetailDTO,
  BotInstanceProfile,
  ToolDTO,
  useListHelixOrgTools,
  useUpdateBot,
} from '../../services/helixOrgService'
import { sandboxRuntimeLabel } from './BotSandboxForm'
import ToolPickerDialog from './ToolPickerDialog'

const BOT_RUNTIME = '__bot__'

const BUILT_IN_SERVERS: Array<{ name: string; label: string; description: string; desktopOnly?: boolean }> = [
  { name: 'chrome-devtools', label: 'Browser', description: 'Chrome, driven through DevTools' },
  { name: 'helix-session', label: 'Session history', description: 'Search and read this chat\'s own history' },
  { name: 'helix-desktop', label: 'Desktop control', description: 'Screenshots and input on the desktop', desktopOnly: true },
  { name: 'kodit', label: 'Code search', description: 'Search the project\'s indexed repositories' },
]

const BotInstanceProfileSettings: FC<{
  detail?: BotDetailDTO
  readOnly: boolean
}> = ({ detail, readOnly }) => {
  const snackbar = useSnackbar()
  const updateBot = useUpdateBot()
  const bot = detail?.bot
  const projectID = detail?.project_id || ''
  const { data: project } = useGetProject(projectID, !!projectID)
  const { data: catalogue = [] } = useListHelixOrgTools({ enabled: !!bot })
  const [editingTools, setEditingTools] = useState(false)
  // Edits show immediately; the saved profile replaces them once it comes
  // back, and a failed save rolls them back.
  const savedProfile: BotInstanceProfile = bot?.instance_profile ?? { mcp_servers: ['chrome-devtools'], tools: [] }
  const savedKey = JSON.stringify(savedProfile)
  const [profile, setProfile] = useState<BotInstanceProfile>(savedProfile)
  useEffect(() => {
    setProfile(JSON.parse(savedKey) as BotInstanceProfile)
  }, [savedKey])

  if (!bot?.id) return null

  const mcpServers = profile.mcp_servers ?? []
  const tools = profile.tools ?? []
  const disabled = readOnly
  const projectServers = (project?.skills?.mcps ?? [])
    .map((mcp) => mcp.name || '')
    .filter((name) => !!name && !BUILT_IN_SERVERS.some((server) => server.name === name))

  // Instance tools come from the bot's own tools: the server serves the
  // intersection anyway, so offering anything else would only mislead.
  const botTools = new Set(bot.tools ?? [])
  const toolOptions: ToolDTO[] = catalogue.filter((tool) => botTools.has(tool.name || ''))

  const save = async (patch: Partial<BotInstanceProfile>) => {
    const next = { ...profile, mcp_servers: mcpServers, tools, ...patch }
    setProfile(next)
    try {
      await updateBot.mutateAsync({ id: bot.id ?? '', instance_profile: next })
      snackbar.success('Instance settings saved')
    } catch (error: any) {
      setProfile(savedProfile)
      snackbar.error(error?.response?.data?.error ?? error?.message ?? 'Failed to save instance settings')
    }
  }

  const toggleServer = (name: string, checked: boolean) => {
    const next = checked ? [...mcpServers, name] : mcpServers.filter((server) => server !== name)
    void save({ mcp_servers: next })
  }

  const botRuntime = sandboxRuntimeLabel(bot.effective_sandbox_runtime)

  return (
    <Stack spacing={3}>
      <Box>
        <Typography variant="body1" sx={{ mb: 0.5 }}>Default sandbox</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          New instances start in this environment unless the caller picks one.
        </Typography>
        <FormControl size="small" sx={{ minWidth: 240 }}>
          <Select
            value={profile.sandbox_runtime || BOT_RUNTIME}
            disabled={disabled}
            onChange={(event) => {
              const value = event.target.value
              void save({ sandbox_runtime: value === BOT_RUNTIME ? undefined : value as TypesSandboxRuntime })
            }}
            inputProps={{ 'aria-label': 'Default instance sandbox' }}
          >
            <MenuItem value={BOT_RUNTIME}>Same as the bot{botRuntime ? ` (${botRuntime})` : ''}</MenuItem>
            <MenuItem value={TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop}>Full Desktop</MenuItem>
            <MenuItem value={TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu}>Headless</MenuItem>
          </Select>
        </FormControl>
      </Box>

      <Box>
        <Typography variant="body1" sx={{ mb: 0.5 }}>MCP servers</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          Instances keep only the servers checked here.
        </Typography>
        <Stack>
          {BUILT_IN_SERVERS.map((server) => (
            <FormControlLabel
              key={server.name}
              disabled={disabled}
              control={(
                <Checkbox
                  size="small"
                  checked={mcpServers.includes(server.name)}
                  onChange={(_event, checked) => toggleServer(server.name, checked)}
                />
              )}
              label={(
                <Typography variant="body2">
                  {server.label}
                  <Typography component="span" variant="body2" color="text.secondary">
                    {` — ${server.description}${server.desktopOnly ? ' (desktop instances only)' : ''}`}
                  </Typography>
                </Typography>
              )}
            />
          ))}
          {projectServers.map((name) => (
            <FormControlLabel
              key={name}
              disabled={disabled}
              control={(
                <Checkbox
                  size="small"
                  checked={mcpServers.includes(name)}
                  onChange={(_event, checked) => toggleServer(name, checked)}
                />
              )}
              label={(
                <Typography variant="body2">
                  {name}
                  <Typography component="span" variant="body2" color="text.secondary"> — project MCP server</Typography>
                </Typography>
              )}
            />
          ))}
        </Stack>
      </Box>

      <Box>
        <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2} sx={{ mb: 1 }}>
          <Box>
            <Typography variant="body1" sx={{ mb: 0.5 }}>Org tools</Typography>
            <Typography variant="body2" color="text.secondary">
              Helix organization tools instances may call, chosen from this bot's own tools.
            </Typography>
          </Box>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Pencil size={16} />}
            onClick={() => setEditingTools(true)}
            disabled={disabled}
          >
            Edit tools
          </Button>
        </Stack>
        <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
          {[...tools].sort().map((tool) => (
            <Chip key={tool} label={tool} size="small" sx={{ fontFamily: 'var(--helix-font-mono)' }} />
          ))}
          {tools.length === 0 && (
            <Typography variant="body2" color="text.secondary">None. Instances can't reach Helix.</Typography>
          )}
        </Stack>
        <ToolPickerDialog
          open={editingTools}
          tools={toolOptions}
          selectedTools={tools}
          onClose={() => setEditingTools(false)}
          onApply={(selectedTools) => {
            setEditingTools(false)
            void save({ tools: selectedTools })
          }}
        />
      </Box>

      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
        <Box>
          <Typography variant="body1">Helix agent skills</Typography>
          <Typography variant="body2" color="text.secondary">
            Link the helix-* skills. The project repository's own skills are always available.
          </Typography>
        </Box>
        <Switch
          checked={!!profile.helix_skills}
          onChange={(_event, checked) => void save({ helix_skills: checked })}
          disabled={disabled}
          inputProps={{ 'aria-label': 'Helix agent skills' }}
        />
      </Stack>
    </Stack>
  )
}

export default BotInstanceProfileSettings
