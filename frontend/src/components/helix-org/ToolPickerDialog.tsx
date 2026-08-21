import { FC, useEffect, useMemo, useState } from 'react'
import Accordion from '@mui/material/Accordion'
import AccordionDetails from '@mui/material/AccordionDetails'
import AccordionSummary from '@mui/material/AccordionSummary'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import InputAdornment from '@mui/material/InputAdornment'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import SearchIcon from '@mui/icons-material/Search'

import { ToolDTO } from '../../services/helixOrgService'

export type ToolCapabilityGroupKey =
  | 'coordination'
  | 'agents'
  | 'topics'
  | 'delivery'
  | 'sandboxes'
  | 'infrastructure'
  | 'credentials'
  | 'other'

export interface ToolCapabilityGroup {
  key: ToolCapabilityGroupKey
  title: string
  description: string
}

export const TOOL_CAPABILITY_GROUPS: ToolCapabilityGroup[] = [
  {
    key: 'coordination',
    title: 'Coordination & awareness',
    description: 'Understand the organization, inspect agent activity, and communicate with people or reporting-line peers.',
  },
  {
    key: 'agents',
    title: 'Agent administration',
    description: 'Create and configure agents, control their desktops, and change the capabilities available to them.',
  },
  {
    key: 'topics',
    title: 'Topics & automation',
    description: 'Publish and route events, manage subscriptions, and build processors that transform topic traffic.',
  },
  {
    key: 'delivery',
    title: 'Projects & delivery',
    description: 'Discover projects and repositories, attach code to agents, and manage specification-driven work.',
  },
  {
    key: 'sandboxes',
    title: 'Sandbox environments',
    description: 'Create and administer standalone organization containers independently from agent desktops.',
  },
  {
    key: 'infrastructure',
    title: 'Infrastructure & servers',
    description: 'Manage organization assets and operate linked servers through commands, files, health checks, and SSH.',
  },
  {
    key: 'credentials',
    title: 'Credentials & secrets',
    description: 'Read project secrets and mint short-lived credentials for approved external providers.',
  },
  {
    key: 'other',
    title: 'Other tools',
    description: 'Registered or legacy capabilities that do not yet belong to one of the standard groups.',
  },
]

const COORDINATION_TOOLS = new Set([
  'managers',
  'reports',
  'list_bots',
  'get_bot',
  'bot_log',
  'read_events',
  'dm',
  'ask_human',
  'set_human_contact',
])

const AGENT_TOOLS = new Set([
  'create_bot',
  'set_bot_content',
  'delete_bot',
  'attach_tool',
  'detach_tool',
  'start_bot',
  'stop_bot',
  'restart_bot',
  'get_bot_project',
  'configure_bot_project',
])

const TOPIC_TOOLS = new Set([
  'list_topics',
  'get_topic',
  'list_topic_events',
  'create_topic',
  'topic_members',
  'subscribe',
  'unsubscribe',
  'publish',
  'list_processors',
  'get_processor',
  'create_processor',
  'update_processor',
  'delete_processor',
])

export const toolCapabilityGroupKey = (name: string): ToolCapabilityGroupKey => {
  if (name.includes('sandbox')) return 'sandboxes'
  if (name.startsWith('server_') || name.includes('asset')) return 'infrastructure'
  if (COORDINATION_TOOLS.has(name)) return 'coordination'
  if (AGENT_TOOLS.has(name)) return 'agents'
  if (TOPIC_TOOLS.has(name)) return 'topics'
  if (name.includes('spectask') || name.includes('project') || name.includes('repository')) return 'delivery'
  if (name === 'get_secret' || name === 'list_secrets') return 'credentials'
  return 'other'
}

export interface GroupedToolCatalogueEntry extends ToolCapabilityGroup {
  tools: ToolDTO[]
}

export const groupToolCatalogue = (tools: ToolDTO[]): GroupedToolCatalogueEntry[] => {
  const byGroup = new Map<ToolCapabilityGroupKey, ToolDTO[]>()
  for (const tool of tools) {
    const name = tool.name?.trim()
    if (!name) continue
    const normalizedTool = { ...tool, name }
    const key = toolCapabilityGroupKey(name)
    const current = byGroup.get(key) ?? []
    current.push(normalizedTool)
    byGroup.set(key, current)
  }
  return TOOL_CAPABILITY_GROUPS
    .map((group) => ({
      ...group,
      tools: (byGroup.get(group.key) ?? []).sort((a, b) => (a.name ?? '').localeCompare(b.name ?? '')),
    }))
    .filter((group) => group.tools.length > 0)
}

interface ToolPickerDialogProps {
  open: boolean
  tools: ToolDTO[]
  selectedTools: string[]
  onClose: () => void
  onApply: (tools: string[]) => void
}

const normalizedSelection = (names: string[]) => Array.from(new Set(names)).sort()

const ToolPickerDialog: FC<ToolPickerDialogProps> = ({
  open,
  tools,
  selectedTools,
  onClose,
  onApply,
}) => {
  const [draftTools, setDraftTools] = useState<string[]>([])
  const [search, setSearch] = useState('')

  useEffect(() => {
    if (!open) return
    setDraftTools(normalizedSelection(selectedTools))
    setSearch('')
  }, [open, selectedTools])

  const groupedTools = useMemo(() => groupToolCatalogue(tools), [tools])
  const selected = useMemo(() => new Set(draftTools), [draftTools])
  const searchValue = search.trim().toLowerCase()
  const visibleGroups = useMemo(() => {
    if (!searchValue) return groupedTools
    return groupedTools
      .map((group) => ({
        ...group,
        tools: group.tools.filter((tool) =>
          `${tool.name ?? ''} ${tool.description ?? ''}`.toLowerCase().includes(searchValue),
        ),
      }))
      .filter((group) => group.tools.length > 0)
  }, [groupedTools, searchValue])

  const toggleTool = (name: string) => {
    setDraftTools((current) => {
      const next = new Set(current)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return normalizedSelection(Array.from(next))
    })
  }

  const toggleGroup = (group: GroupedToolCatalogueEntry) => {
    const names = group.tools.map((tool) => tool.name ?? '').filter(Boolean)
    const allEnabled = names.every((name) => selected.has(name))
    setDraftTools((current) => {
      const next = new Set(current)
      for (const name of names) {
        if (allEnabled) next.delete(name)
        else next.add(name)
      }
      return normalizedSelection(Array.from(next))
    })
  }

  const handleApply = () => {
    onApply(normalizedSelection(draftTools))
    onClose()
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          bgcolor: 'background.paper',
          height: '80vh',
          maxHeight: 'calc(100% - 32px)',
        },
      }}
    >
      <DialogTitle component="div" sx={{ pb: 1, flexShrink: 0 }}>
        <Typography variant="h6">Agent tools</Typography>
        <Typography variant="body2" color="text.secondary">
          Choose the MCP capabilities this agent can call. Section checkboxes enable or disable an entire capability area.
        </Typography>
        <Typography variant="caption" color="text.secondary">
          Applying a selection updates this form; use Save agent to persist the configuration.
        </Typography>
      </DialogTitle>
      <DialogContent
        dividers
        sx={{
          p: 0,
          minHeight: 0,
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <Box
          sx={{
            p: 2,
            flexShrink: 0,
            bgcolor: 'background.paper',
            borderBottom: '1px solid',
            borderColor: 'divider',
            position: 'sticky',
            top: 0,
            zIndex: 1,
          }}
        >
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
            <TextField
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search tools"
              size="small"
              fullWidth
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchIcon fontSize="small" />
                  </InputAdornment>
                ),
              }}
            />
            <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
              <Button size="small" onClick={() => setDraftTools(normalizedSelection(tools.map((tool) => tool.name ?? '').filter(Boolean)))}>
                Enable all
              </Button>
              <Button size="small" onClick={() => setDraftTools([])} disabled={draftTools.length === 0}>
                Clear all
              </Button>
            </Stack>
          </Stack>
        </Box>

        <Box sx={{ minHeight: 0, flex: 1, overflowY: 'auto', p: 2 }}>
          <Stack spacing={2}>
            {visibleGroups.length === 0 ? (
              <Box sx={{ py: 6, textAlign: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                  No tools match “{search}”.
                </Typography>
              </Box>
            ) : visibleGroups.map((visibleGroup) => {
              const fullGroup = groupedTools.find((group) => group.key === visibleGroup.key) ?? visibleGroup
              const groupNames = fullGroup.tools.map((tool) => tool.name ?? '').filter(Boolean)
              const enabledCount = groupNames.filter((name) => selected.has(name)).length
              const allEnabled = enabledCount === groupNames.length
              const someEnabled = enabledCount > 0 && !allEnabled
              return (
                <Accordion
                  key={visibleGroup.key}
                  disableGutters
                  elevation={0}
                  sx={{
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: '8px !important',
                    bgcolor: 'transparent',
                    '&:before': { display: 'none' },
                  }}
                >
                  <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={{ px: 1.5 }}>
                    <Checkbox
                      checked={allEnabled}
                      indeterminate={someEnabled}
                      onClick={(event) => event.stopPropagation()}
                      onFocus={(event) => event.stopPropagation()}
                      onChange={() => toggleGroup(fullGroup)}
                      inputProps={{ 'aria-label': `Toggle all ${fullGroup.title} tools` }}
                      sx={{ alignSelf: 'flex-start', mt: -0.25, mr: 0.75 }}
                    />
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="subtitle2" sx={{ fontSize: '0.8rem' }}>{fullGroup.title}</Typography>
                        <Chip
                          label={`${enabledCount}/${groupNames.length}`}
                          size="small"
                          sx={{ height: 20, fontSize: '0.65rem' }}
                        />
                      </Stack>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ display: 'block', mt: 0.25, fontSize: '0.68rem' }}
                      >
                        {fullGroup.description}
                      </Typography>
                    </Box>
                  </AccordionSummary>
                  <AccordionDetails sx={{ pt: 0, px: 1.5, pb: 1.5 }}>
                    <Box sx={{ borderTop: '1px solid', borderColor: 'divider' }}>
                      {visibleGroup.tools.map((tool) => {
                        const name = tool.name ?? ''
                        const enabled = selected.has(name)
                        return (
                          <Box
                            component="label"
                            key={name}
                            sx={{
                              display: 'flex',
                              alignItems: 'center',
                              gap: 1.5,
                              py: 1.25,
                              px: 0.5,
                              borderBottom: '1px solid',
                              borderColor: 'divider',
                              bgcolor: 'transparent',
                              cursor: 'pointer',
                              '&:last-child': { borderBottom: 0 },
                            }}
                          >
                            <Box sx={{ minWidth: 0, flex: 1 }}>
                              <Typography
                                variant="body2"
                                sx={{ fontFamily: 'monospace', fontSize: '0.75rem', fontWeight: 600, wordBreak: 'break-word' }}
                              >
                                {name}
                              </Typography>
                              <Typography
                                variant="caption"
                                color="text.secondary"
                                title={tool.description || undefined}
                                sx={{
                                  display: '-webkit-box',
                                  mt: 0.25,
                                  fontSize: '0.68rem',
                                  lineHeight: 1.4,
                                  overflow: 'hidden',
                                  WebkitBoxOrient: 'vertical',
                                  WebkitLineClamp: 2,
                                }}
                              >
                                {tool.description || 'No description is available for this tool.'}
                              </Typography>
                            </Box>
                            <Checkbox
                              checked={enabled}
                              onChange={() => toggleTool(name)}
                              inputProps={{ 'aria-label': `Enable ${name}` }}
                              size="small"
                              sx={{ flexShrink: 0 }}
                            />
                          </Box>
                        )
                      })}
                    </Box>
                  </AccordionDetails>
                </Accordion>
              )
            })}
          </Stack>
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 2, py: 1.5, flexShrink: 0 }}>
        <Typography variant="body2" color="text.secondary" sx={{ mr: 'auto' }}>
          {draftTools.length} of {tools.length} enabled
        </Typography>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" color="secondary" onClick={handleApply}>
          Apply selection
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default ToolPickerDialog
