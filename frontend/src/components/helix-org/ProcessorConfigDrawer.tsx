// ProcessorConfigDrawer creates or edits a Processor — the transform /
// filter node interposed between a Topic and the Workers that read it.
// Opened from the Chart's "Processor" button (create) and from clicking
// a processor node (edit, `processor` prop set).
//
// The drawer shows the most recent REAL message on the chosen input
// topic (no synthetic/fake data, no client-side transform) so the
// operator can see what their template/predicate will run against. The
// actual rendering happens server-side at runtime.

import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Collapse from '@mui/material/Collapse'
import Divider from '@mui/material/Divider'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CloseIcon from '@mui/icons-material/Close'

import useSnackbar from '../../hooks/useSnackbar'
import {
  ProcessorDTO,
  useCreateHelixOrgProcessor,
  useUpdateHelixOrgProcessor,
  useListHelixOrgTopics,
  useTopicSampleMessage,
} from '../../services/helixOrgService'

export type ProcessorConfigDrawerProps = {
  open: boolean
  onClose: () => void
  // Edit mode when set; otherwise create.
  processor?: ProcessorDTO | null
  // Prefill the input topic (e.g. opened from a topic context).
  presetInputTopicId?: string
}

const KINDS = [
  { value: 'template', label: 'template — rewrite the body via Go text/template' },
  { value: 'truncate', label: 'truncate — cap the body to N bytes' },
  { value: 'filter', label: 'filter / router — route by predicate to 1..N outputs' },
]

type FilterRow = { label: string; match: string }
const DEFAULT_FILTER_ROWS: FilterRow[] = [
  { label: 'vip', match: '{{ hasSuffix "@vip.com" .Message.from }}' },
  { label: 'default', match: '' },
]

const DEFAULT_TEMPLATE = 'From {{ .Message.from }}: {{ .Message.subject }}\n\n{{ .Message.body }}'

// prettyJson re-indents a canonical Message envelope for display; falls
// back to the raw string if it isn't valid JSON.
function prettyJson(raw?: string): string {
  if (!raw) return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

// SyntaxHelp is the in-drawer reference for the template / filter
// languages — the available message fields, the function set (shared by
// both kinds), and, for filters, how a predicate "matches".
const SyntaxHelp: FC<{ kind: string }> = ({ kind }) => {
  const mono = { fontFamily: 'monospace', fontSize: '0.72rem' as const }
  const fields: [string, string][] = [
    ['.Message.from', 'sender, e.g. "alice@vip.com"'],
    ['.Message.subject', 'subject line'],
    ['.Message.body', 'message body'],
    ['.Message.to', 'recipients (list)'],
    ['.Message.thread_id', 'conversation id'],
    ['.Message.message_id', 'unique message id'],
    ['.Message.in_reply_to', 'parent message id'],
    ['.Message.extra', 'transport-specific extras'],
  ]
  const funcs: [string, string][] = [
    ['upper S', 'UPPERCASE'],
    ['lower S', 'lowercase'],
    ['trunc N S', 'cap S to N bytes'],
    ['default "x" S', '"x" when S is empty'],
    ['contains "bug" S', 'S contains "bug"'],
    ['hasPrefix "RE:" S', 'S starts with "RE:"'],
    ['hasSuffix "@vip.com" S', 'S ends with "@vip.com"'],
  ]
  return (
    <Box sx={{ border: '1px solid rgba(0,0,0,0.08)', borderRadius: 1, p: 1, backgroundColor: 'rgba(0,0,0,0.015)' }}>
      <Typography variant="caption" sx={{ fontWeight: 700, display: 'block' }}>Fields — the {'{{ .Message.* }}'} context</Typography>
      {fields.map(([f, d]) => (
        <Box key={f} sx={{ display: 'flex', gap: 1 }}>
          <Typography sx={{ ...mono, color: 'primary.main', minWidth: 150 }}>{f}</Typography>
          <Typography variant="caption" color="text.secondary">{d}</Typography>
        </Box>
      ))}
      <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>
        Functions — note the test string comes FIRST, the field LAST
      </Typography>
      {funcs.map(([f, d]) => (
        <Box key={f} sx={{ display: 'flex', gap: 1 }}>
          <Typography sx={{ ...mono, color: 'primary.main', minWidth: 150 }}>{f}</Typography>
          <Typography variant="caption" color="text.secondary">{d}</Typography>
        </Box>
      ))}
      {kind === 'filter' && (
        <>
          <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>Matching</Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            A branch receives a message when its predicate renders to a <b>truthy</b> value — anything
            except empty, <code>false</code>, <code>0</code>, or <code>no</code>.
          </Typography>
          <Box sx={{ ...mono, mt: 0.5, whiteSpace: 'pre-wrap' }}>
            {'simplest:  {{ hasSuffix "@vip.com" .Message.from }}   → true / false\n'}
            {'or:        {{ if contains "bug" (lower .Message.subject) }}1{{ end }}\n'}
            {'              → "1" (match) when it contains "bug", else "" (no match)'}
          </Box>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
            The <code>1</code> is just "output something truthy" — any non-empty token works.
            An <b>empty</b> predicate is the default/catch-all (matches every message). A message can
            match several branches at once.
          </Typography>
        </>
      )}
      {kind === 'template' && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
          The rendered text becomes the new message <b>body</b>.
        </Typography>
      )}
    </Box>
  )
}

const ProcessorConfigDrawer: FC<ProcessorConfigDrawerProps> = ({ open, onClose, processor, presetInputTopicId }) => {
  const snackbar = useSnackbar()
  const isEdit = Boolean(processor)
  const createProc = useCreateHelixOrgProcessor()
  const updateProc = useUpdateHelixOrgProcessor()
  const { data: topicsData } = useListHelixOrgTopics({ enabled: open })

  const [name, setName] = useState('')
  const [kind, setKind] = useState('template')
  const [inputTopicId, setInputTopicId] = useState('')
  const [template, setTemplate] = useState(DEFAULT_TEMPLATE)
  const [maxBytes, setMaxBytes] = useState('500')
  const [filterRows, setFilterRows] = useState<FilterRow[]>(DEFAULT_FILTER_ROWS)
  const [showHelp, setShowHelp] = useState(false)

  // Most recent real message on the input topic (null = topic has none).
  const { data: sample, isFetching: sampleLoading } = useTopicSampleMessage(inputTopicId, { enabled: open && !!inputTopicId })

  // Reset form each open from the processor under edit (or defaults).
  useEffect(() => {
    if (!open) return
    if (processor) {
      setName(processor.name ?? '')
      setKind(processor.kind ?? 'template')
      setInputTopicId(processor.input_topic_id ?? '')
      setTemplate((processor.config?.template as string) ?? DEFAULT_TEMPLATE)
      setMaxBytes(String((processor.config?.max_bytes as number) ?? 500))
      const rows = (processor.outputs ?? []).map((o) => ({ label: o.label ?? '', match: o.match ?? '' }))
      setFilterRows(rows.length > 0 ? rows : DEFAULT_FILTER_ROWS)
    } else {
      setName('')
      setKind('template')
      setInputTopicId(presetInputTopicId ?? '')
      setTemplate(DEFAULT_TEMPLATE)
      setMaxBytes('500')
      setFilterRows(DEFAULT_FILTER_ROWS)
    }
  }, [open, processor, presetInputTopicId])

  const topics = topicsData?.topics ?? []

  const config = useMemo<Record<string, unknown>>(() => {
    if (kind === 'truncate') return { max_bytes: parseInt(maxBytes, 10) || 0 }
    if (kind === 'filter') return {}
    return { template }
  }, [kind, template, maxBytes])

  // Filter outputs carry the per-branch predicates; transforms have a
  // single auto-provisioned output (undefined → server defaults to one).
  const outputs = useMemo(() => {
    if (kind !== 'filter') return undefined
    return filterRows.map((rw) => ({ label: rw.label, match: rw.match }))
  }, [kind, filterRows])

  const canSubmit = Boolean(name.trim()) && (isEdit || Boolean(inputTopicId))

  const submit = async () => {
    try {
      if (isEdit && processor) {
        await updateProc.mutateAsync({ id: processor.id, attrs: { name: name.trim(), kind, config, input_topic_id: inputTopicId } })
        snackbar.success(`updated ${processor.id}`)
      } else {
        const created = await createProc.mutateAsync({
          name: name.trim(),
          input_topic_id: inputTopicId,
          kind,
          config,
          outputs,
        })
        snackbar.success(`created ${created.id} → ${created.outputs?.[0]?.topic_id ?? 'output topic'}`)
      }
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.errors?.[0]?.detail ?? err?.response?.data?.error ?? err?.message ?? 'save failed')
    }
  }

  const busy = createProc.isPending || updateProc.isPending

  return (
    <Drawer anchor="right" open={open} onClose={onClose} PaperProps={{ sx: { backgroundImage: 'none' } }}>
      <Box sx={{ p: 2.5, width: 460 }}>
        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
          <Typography variant="h6">{isEdit ? 'Edit processor' : 'New processor'}</Typography>
          <IconButton size="small" onClick={onClose}><CloseIcon /></IconButton>
        </Stack>
        <Stack spacing={1.5}>
          <TextField
            size="small" label="Name" value={name} fullWidth required
            onChange={(e) => setName(e.target.value)}
            placeholder="Inbox formatter"
          />
          <TextField
            select size="small" label="Kind" value={kind} fullWidth
            onChange={(e) => setKind(e.target.value)}
            helperText="A new kind is one Go file — template, truncate and filter ship in v1."
          >
            {KINDS.map((k) => <MenuItem key={k.value} value={k.value}>{k.label}</MenuItem>)}
          </TextField>
          <TextField
            select size="small" label="Input topic" value={inputTopicId} fullWidth required
            onChange={(e) => setInputTopicId(e.target.value)}
            helperText="The topic this processor reads. An output topic is auto-created; subscribe workers to it on the chart. (You can also re-wire this on the chart by dragging a Topic into the IN port.)"
          >
            {topics.map((tp) => (
              <MenuItem key={tp.id} value={tp.id ?? ''} sx={{ fontFamily: 'monospace' }}>
                {tp.id}{tp.name ? ` — ${tp.name}` : ''}
              </MenuItem>
            ))}
          </TextField>
          <Divider sx={{ my: 0.5 }} />
          {kind === 'template' && (
            <TextField
              size="small" label="Template (Go text/template)" value={template} fullWidth
              onChange={(e) => setTemplate(e.target.value)}
              multiline minRows={4}
              InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.8rem' } }}
              helperText="The rendered text becomes the new body. See “syntax help” below for fields & functions."
            />
          )}
          {kind === 'truncate' && (
            <TextField
              size="small" label="Max bytes" value={maxBytes} fullWidth
              onChange={(e) => setMaxBytes(e.target.value.replace(/[^0-9]/g, ''))}
              helperText="Cap the body to this many bytes (rune-safe)."
            />
          )}
          {kind === 'filter' && (
            <Stack spacing={1}>
              <Typography variant="caption" color="text.secondary">
                Outputs — each is one branch: a predicate + a destination topic (auto-created). A message
                goes to every branch whose predicate is truthy; an empty predicate is the default/catch-all.
                See “syntax help” below. {isEdit && '(Branches are immutable on edit — recreate to change.)'}
              </Typography>
              {filterRows.map((row, i) => (
                <Stack key={i} direction="row" spacing={0.5} alignItems="flex-start">
                  <TextField
                    size="small" label="label" value={row.label} sx={{ width: 110 }} disabled={isEdit}
                    onChange={(e) => setFilterRows((rs) => rs.map((r, j) => j === i ? { ...r, label: e.target.value } : r))}
                  />
                  <TextField
                    size="small" label="predicate (empty = default)" value={row.match} fullWidth disabled={isEdit}
                    onChange={(e) => setFilterRows((rs) => rs.map((r, j) => j === i ? { ...r, match: e.target.value } : r))}
                    InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.75rem' } }}
                  />
                  {!isEdit && filterRows.length > 1 && (
                    <IconButton size="small" onClick={() => setFilterRows((rs) => rs.filter((_, j) => j !== i))}>
                      <CloseIcon sx={{ fontSize: 16 }} />
                    </IconButton>
                  )}
                </Stack>
              ))}
              {!isEdit && (
                <Button size="small" onClick={() => setFilterRows((rs) => [...rs, { label: '', match: '' }])} sx={{ alignSelf: 'flex-start' }}>
                  + branch
                </Button>
              )}
            </Stack>
          )}

          {/* Syntax reference (template + filter share the same engine). */}
          {kind !== 'truncate' && (
            <Box>
              <Link component="button" type="button" underline="hover" variant="caption" onClick={() => setShowHelp((v) => !v)}>
                {showHelp ? '▾ Hide syntax help' : '▸ Show syntax help (fields, functions, matching)'}
              </Link>
              <Collapse in={showHelp}>
                <Box sx={{ mt: 0.5 }}><SyntaxHelp kind={kind} /></Box>
              </Collapse>
            </Box>
          )}

          {/* Most recent real message on the input topic, shown as the raw
              canonical JSON so the operator can see exactly which fields
              are available to template / filter on. */}
          <Box sx={{ mt: 0.5 }}>
            <Typography variant="caption" color="text.secondary">
              Latest message {inputTopicId ? `on ${inputTopicId}` : ''} — the raw {'{{ .Message }}'} JSON
            </Typography>
            {!inputTopicId ? (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, fontStyle: 'italic' }}>
                Select an input topic to see its latest message.
              </Typography>
            ) : sampleLoading ? (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, fontStyle: 'italic' }}>
                Loading…
              </Typography>
            ) : !sample ? (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, fontStyle: 'italic' }}>
                No messages on this topic yet.
              </Typography>
            ) : (
              <Box
                component="pre"
                sx={{
                  m: 0, mt: 0.5, p: 1,
                  border: '1px solid rgba(0,0,0,0.08)', borderRadius: 1,
                  backgroundColor: 'rgba(0,0,0,0.025)',
                  fontFamily: 'monospace', fontSize: '0.72rem', lineHeight: 1.4,
                  whiteSpace: 'pre', overflow: 'auto', maxHeight: 220,
                }}
              >
                {prettyJson(sample.raw) || JSON.stringify({ from: sample.from, subject: sample.subject, body: sample.body }, null, 2)}
              </Box>
            )}
          </Box>

          <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
            <Button variant="contained" onClick={submit} disabled={busy || !canSubmit}>
              {busy ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </Button>
            <Button variant="text" onClick={onClose}>Cancel</Button>
          </Stack>
        </Stack>
      </Box>
    </Drawer>
  )
}

export default ProcessorConfigDrawer
