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
import FormControlLabel from '@mui/material/FormControlLabel'
import IconButton from '@mui/material/IconButton'
import Switch from '@mui/material/Switch'
import Link from '@mui/material/Link'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CloseIcon from '@mui/icons-material/Close'

import useSnackbar from '../../hooks/useSnackbar'
import MonacoEditor from '../widgets/MonacoEditor'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'
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
  { value: 'template', label: 'Rewrite — change the message text using a template' },
  { value: 'truncate', label: 'Shorten — cut the message if it is too long' },
  { value: 'filter', label: 'Sort / route — send each message down one or more branches' },
  { value: 'js', label: 'JavaScript — transform, route, and call HTTP APIs' },
]

const DEFAULT_JS_CODE = `// process runs once per inbound message.
// Return the event to publish, null to drop, or { out: "label", event } to route.
function process(event, ctx) {
  // Example: enrich via HTTP, then rewrite the body.
  // const res = http.get("https://api.example.com/lookup", {
  //   query: { email: event.from },
  //   headers: { "Authorization": "Bearer …" },
  // });
  // if (!res.ok) return null;
  // const data = res.json();
  // event.extra = Object.assign({}, event.extra || {}, data);

  event.body = event.body || "";
  return event;
}
`

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

// SyntaxHelp — plain-English reference for rewrite templates, route rules, and JS.
const SyntaxHelp: FC<{ kind: string }> = ({ kind }) => {
  const mono = { fontFamily: 'monospace', fontSize: '0.72rem' as const }

  if (kind === 'js') {
    const eventFields: [string, string][] = [
      ['event.from', 'who sent it'],
      ['event.subject', 'subject line'],
      ['event.body', 'main message text'],
      ['event.to', 'array of recipients'],
      ['event.thread_id', 'conversation / thread id'],
      ['event.message_id', 'this message’s id'],
      ['event.extra', 'object of extra fields (or null)'],
      ['event.reply_hint', 'how to reply via the origin transport'],
    ]
    const httpFns: [string, string][] = [
      ['http.get(url, opts?)', 'GET request'],
      ['http.post(url, opts?)', 'POST (opts.json / opts.body)'],
      ['http.put / patch / delete', 'same options shape'],
      ['http.request(method, url, opts?)', 'any method'],
      ['res.status / res.ok', 'status code / 2xx flag'],
      ['res.body / res.json()', 'raw body or parsed JSON'],
      ['res.headers', 'response headers (lowercase keys)'],
    ]
    return (
      <Box sx={{ border: '1px solid rgba(0,0,0,0.08)', borderRadius: 1, p: 1, backgroundColor: 'rgba(0,0,0,0.015)' }}>
        <Typography variant="caption" sx={{ fontWeight: 700, display: 'block' }}>
          Required entrypoint
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
          Define <code>function process(event, ctx)</code>. Return the event to publish,
          <code> null</code> to drop, <code>{'{ out: "label", event }'}</code> to route to a branch,
          or an array to fan out.
        </Typography>
        <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>
          Event fields
        </Typography>
        {eventFields.map(([f, d]) => (
          <Box key={f} sx={{ display: 'flex', gap: 1 }}>
            <Typography sx={{ ...mono, color: 'primary.main', minWidth: 150 }}>{f}</Typography>
            <Typography variant="caption" color="text.secondary">{d}</Typography>
          </Box>
        ))}
        <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>
          HTTP client
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
          Also available as <code>ctx.http</code>. Options: headers, query, body, json, timeout_ms
          (default 10s, max 30s).
        </Typography>
        {httpFns.map(([f, d]) => (
          <Box key={f} sx={{ display: 'flex', gap: 1 }}>
            <Typography sx={{ ...mono, color: 'primary.main', minWidth: 200 }}>{f}</Typography>
            <Typography variant="caption" color="text.secondary">{d}</Typography>
          </Box>
        ))}
        <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>
          Routing
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
          Create labeled branches below (or leave one default output). In code:
          <code>{' return { out: "vip", event }; '}</code>
          or by index <code>{' { out: 0, event } '}</code>.
        </Typography>
      </Box>
    )
  }

  const fields: [string, string][] = [
    ['.Message.from', 'who sent it (e.g. alice@vip.com)'],
    ['.Message.subject', 'subject line'],
    ['.Message.body', 'main message text'],
    ['.Message.to', 'who it was sent to'],
    ['.Message.thread_id', 'conversation / thread id'],
    ['.Message.message_id', 'this message’s id'],
    ['.Message.in_reply_to', 'id of the message it replies to'],
    ['.Message.extra', 'extra fields from the source (Slack, email, …)'],
  ]
  const funcs: [string, string][] = [
    ['upper S', 'make uppercase'],
    ['lower S', 'make lowercase'],
    ['trunc N S', 'keep only the first N characters of S'],
    ['default "x" S', 'use "x" if S is empty'],
    ['contains "bug" S', 'true if S contains "bug"'],
    ['hasPrefix "RE:" S', 'true if S starts with "RE:"'],
    ['hasSuffix "@vip.com" S', 'true if S ends with "@vip.com"'],
  ]
  return (
    <Box sx={{ border: '1px solid rgba(0,0,0,0.08)', borderRadius: 1, p: 1, backgroundColor: 'rgba(0,0,0,0.015)' }}>
      <Typography variant="caption" sx={{ fontWeight: 700, display: 'block' }}>
        Message fields you can use
      </Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
        Write them as {'{{ .Message.from }}'}, {'{{ .Message.subject }}'}, and so on.
      </Typography>
      {fields.map(([f, d]) => (
        <Box key={f} sx={{ display: 'flex', gap: 1 }}>
          <Typography sx={{ ...mono, color: 'primary.main', minWidth: 150 }}>{f}</Typography>
          <Typography variant="caption" color="text.secondary">{d}</Typography>
        </Box>
      ))}
      <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>
        Handy helpers
      </Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
        Order matters: the fixed text comes first, the field last
        (e.g. {'{{ hasSuffix "@vip.com" .Message.from }}'}).
      </Typography>
      {funcs.map(([f, d]) => (
        <Box key={f} sx={{ display: 'flex', gap: 1 }}>
          <Typography sx={{ ...mono, color: 'primary.main', minWidth: 150 }}>{f}</Typography>
          <Typography variant="caption" color="text.secondary">{d}</Typography>
        </Box>
      ))}
      {kind === 'filter' && (
        <>
          <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mt: 1 }}>
            When does a branch get a message?
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            The rule is a small template. If it comes out as “yes” (anything except empty,
            <code> false</code>, <code>0</code>, or <code>no</code>), that branch receives the message.
          </Typography>
          <Box sx={{ ...mono, mt: 0.5, whiteSpace: 'pre-wrap' }}>
            {'simple:   {{ hasSuffix "@vip.com" .Message.from }}\n'}
            {'          → true for VIP senders, false otherwise\n\n'}
            {'or:       {{ if contains "bug" (lower .Message.subject) }}yes{{ end }}\n'}
            {'          → "yes" when the subject mentions bug, empty otherwise'}
          </Box>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
            Leave a rule <b>blank</b> for a catch-all that gets every message.
            One message can match several branches at once.
          </Typography>
        </>
      )}
      {kind === 'template' && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
          Whatever the template prints becomes the new message text.
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
  const [jsCode, setJsCode] = useState(DEFAULT_JS_CODE)
  const [maxBytes, setMaxBytes] = useState('500')
  const [filterRows, setFilterRows] = useState<FilterRow[]>(DEFAULT_FILTER_ROWS)
  const [threadFollow, setThreadFollow] = useState(false)
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
      setJsCode((processor.config?.code as string) ?? DEFAULT_JS_CODE)
      setMaxBytes(String((processor.config?.max_bytes as number) ?? 500))
      const rows = (processor.outputs ?? []).map((o) => ({ label: o.label ?? '', match: o.match ?? '' }))
      setFilterRows(rows.length > 0 ? rows : DEFAULT_FILTER_ROWS)
      setThreadFollow(Boolean(processor.config?.thread_follow))
    } else {
      setName('')
      setKind('template')
      setInputTopicId(presetInputTopicId ?? '')
      setTemplate(DEFAULT_TEMPLATE)
      setJsCode(DEFAULT_JS_CODE)
      setMaxBytes('500')
      setFilterRows(DEFAULT_FILTER_ROWS)
      setThreadFollow(false)
    }
  }, [open, processor, presetInputTopicId])

  const topics = topicsData?.topics ?? []

  const config = useMemo<Record<string, unknown>>(() => {
    if (kind === 'truncate') return { max_bytes: parseInt(maxBytes, 10) || 0 }
    if (kind === 'filter') return { thread_follow: threadFollow }
    if (kind === 'js') return { code: jsCode }
    return { template }
  }, [kind, template, jsCode, maxBytes, threadFollow])

  // Thread-follow is only meaningful for the Slack auto-router (an
  // automated filter): it routes later messages in a thread to everyone
  // already participating, not just whoever is named. Hidden for ordinary
  // hand-built filters, where it has no effect.
  const showThreadFollow = kind === 'filter' && Boolean(processor?.automated)

  // Filter/js multi-branch outputs carry labels (and filter predicates).
  // Transforms with a single output omit this (server defaults to one).
  const outputs = useMemo(() => {
    if (kind !== 'filter' && kind !== 'js') return undefined
    // js with a single default branch needs no explicit outputs.
    if (kind === 'js' && filterRows.length === 1 && !filterRows[0].label && !filterRows[0].match) {
      return undefined
    }
    if (kind === 'js') {
      // Labels only — js routes via return { out: label }; match is unused.
      return filterRows.map((rw) => ({ label: rw.label, match: '' }))
    }
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
    <HelixOrgSideDrawer
      open={open}
      onClose={onClose}
      title={isEdit ? 'Edit processor' : 'New processor'}
      width={kind === 'js' ? 560 : 460}
    >
        <Stack spacing={1.5}>
          <Typography variant="body2" color="text.secondary">
            A processor sits between topics: it reads messages from one topic,
            then rewrites, shortens, sorts, or runs JavaScript on them onto new topics that bots can subscribe to.
          </Typography>
          <TextField
            size="small" label="Name" value={name} fullWidth required
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Support triage"
            helperText="Shown on the chart."
          />
          <TextField
            select size="small" label="What should it do?" value={kind} fullWidth
            onChange={(e) => {
              const next = e.target.value
              setKind(next)
              // Reset branch defaults when switching kinds on create.
              if (!isEdit) {
                if (next === 'filter') setFilterRows(DEFAULT_FILTER_ROWS)
                else if (next === 'js') setFilterRows([{ label: '', match: '' }])
              }
            }}
            helperText="Pick how messages are handled as they pass through."
          >
            {KINDS.map((k) => <MenuItem key={k.value} value={k.value}>{k.label}</MenuItem>)}
          </TextField>
          <TextField
            select size="small" label="Read messages from" value={inputTopicId} fullWidth required
            onChange={(e) => setInputTopicId(e.target.value)}
            helperText="The topic this processor listens to. You can also connect a topic to the IN port on the chart."
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
              size="small" label="How to rewrite the message" value={template} fullWidth
              onChange={(e) => setTemplate(e.target.value)}
              multiline minRows={4}
              InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.8rem' } }}
              helperText="This text is filled in from the message fields. The result becomes the new message body. Open “How to write rules” below for examples."
            />
          )}
          {kind === 'js' && (
            <Box>
              <Typography variant="body2" sx={{ mb: 0.5, fontWeight: 600 }}>
                JavaScript (function process)
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.75 }}>
                Must define process(event, ctx). Use http.get/post/… for outbound calls. Open help below for the full API.
              </Typography>
              <MonacoEditor
                value={jsCode}
                onChange={setJsCode}
                language="javascript"
                theme="helix-dark"
                height={320}
                minHeight={240}
                options={{
                  minimap: { enabled: false },
                  lineNumbers: 'on',
                  wordWrap: 'on',
                  scrollBeyondLastLine: false,
                  fontSize: 12,
                  tabSize: 2,
                  automaticLayout: true,
                }}
              />
            </Box>
          )}
          {kind === 'truncate' && (
            <TextField
              size="small" label="Maximum length (characters)" value={maxBytes} fullWidth
              onChange={(e) => setMaxBytes(e.target.value.replace(/[^0-9]/g, ''))}
              helperText="Longer messages are cut down to this length."
            />
          )}
          {showThreadFollow && (
            <Box sx={{ border: '1px solid rgba(0,0,0,0.08)', borderRadius: 1, p: 1, backgroundColor: 'rgba(0,0,0,0.015)' }}>
              <FormControlLabel
                control={<Switch size="small" checked={threadFollow} onChange={(e) => setThreadFollow(e.target.checked)} />}
                label={<Typography variant="body2" sx={{ fontWeight: 600 }}>Keep people in the thread</Typography>}
              />
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                After a bot is pulled into a Slack thread (for example because they were named),
                later messages in that same thread also go to them — even if they are not named again.
              </Typography>
            </Box>
          )}
          {kind === 'filter' && (
            <Stack spacing={1}>
              <Typography variant="body2" color="text.secondary">
                Add branches like a sorting tray. Each branch has a short name and a rule.
                When a message arrives, every branch whose rule matches gets a copy
                (we create a topic per branch automatically). Leave a rule blank to catch everything else.
                {isEdit ? ' Branches cannot be changed after create — make a new processor to redesign them.' : ''}
              </Typography>
              {filterRows.map((row, i) => (
                <Stack key={i} direction="row" spacing={0.5} alignItems="flex-start">
                  <TextField
                    size="small" label="Branch name" value={row.label} sx={{ width: 110 }} disabled={isEdit}
                    onChange={(e) => setFilterRows((rs) => rs.map((r, j) => j === i ? { ...r, label: e.target.value } : r))}
                    placeholder="vip"
                  />
                  <TextField
                    size="small" label="Rule (blank = catch-all)" value={row.match} fullWidth disabled={isEdit}
                    onChange={(e) => setFilterRows((rs) => rs.map((r, j) => j === i ? { ...r, match: e.target.value } : r))}
                    InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.75rem' } }}
                    placeholder={'{{ hasSuffix "@vip.com" .Message.from }}'}
                  />
                  {!isEdit && filterRows.length > 1 && (
                    <IconButton size="small" onClick={() => setFilterRows((rs) => rs.filter((_, j) => j !== i))} aria-label="Remove branch">
                      <CloseIcon sx={{ fontSize: 16 }} />
                    </IconButton>
                  )}
                </Stack>
              ))}
              {!isEdit && (
                <Button size="small" onClick={() => setFilterRows((rs) => [...rs, { label: '', match: '' }])} sx={{ alignSelf: 'flex-start' }}>
                  Add branch
                </Button>
              )}
            </Stack>
          )}
          {kind === 'js' && (
            <Stack spacing={1}>
              <Typography variant="body2" color="text.secondary">
                Optional labeled outputs for routing (<code>{'{ out: "label" }'}</code> in process).
                Leave as a single blank branch for a simple 1→1 transform.
                {isEdit ? ' Branches cannot be changed after create — make a new processor to redesign them.' : ''}
              </Typography>
              {filterRows.map((row, i) => (
                <Stack key={i} direction="row" spacing={0.5} alignItems="flex-start">
                  <TextField
                    size="small" label="Branch name" value={row.label} fullWidth disabled={isEdit}
                    onChange={(e) => setFilterRows((rs) => rs.map((r, j) => j === i ? { ...r, label: e.target.value } : r))}
                    placeholder="default"
                    helperText={i === 0 ? 'Used as return { out: "name", event }' : undefined}
                  />
                  {!isEdit && filterRows.length > 1 && (
                    <IconButton size="small" onClick={() => setFilterRows((rs) => rs.filter((_, j) => j !== i))} aria-label="Remove branch">
                      <CloseIcon sx={{ fontSize: 16 }} />
                    </IconButton>
                  )}
                </Stack>
              ))}
              {!isEdit && (
                <Button size="small" onClick={() => setFilterRows((rs) => [...rs, { label: '', match: '' }])} sx={{ alignSelf: 'flex-start' }}>
                  Add branch
                </Button>
              )}
            </Stack>
          )}

          {kind !== 'truncate' && (
            <Box>
              <Link component="button" type="button" underline="hover" variant="caption" onClick={() => setShowHelp((v) => !v)}>
                {showHelp ? '▾ Hide: how to write rules' : '▸ How to write rules (fields & examples)'}
              </Link>
              <Collapse in={showHelp}>
                <Box sx={{ mt: 0.5 }}><SyntaxHelp kind={kind} /></Box>
              </Collapse>
            </Box>
          )}

          <Box sx={{ mt: 0.5 }}>
            <Typography variant="caption" color="text.secondary">
              {inputTopicId
                ? `Example: latest real message on ${inputTopicId}`
                : 'Example message'}
            </Typography>
            {!inputTopicId ? (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, fontStyle: 'italic' }}>
                Choose a topic above to preview a recent message.
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
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  overflowWrap: 'anywhere',
                  overflow: 'auto',
                  maxHeight: 220,
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
    </HelixOrgSideDrawer>
  )
}

export default ProcessorConfigDrawer
