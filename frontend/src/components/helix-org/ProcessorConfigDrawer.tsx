// ProcessorConfigDrawer creates or edits a Processor — the transform
// node interposed between a Topic and the Workers that read it. It is
// opened from the Chart ("+ Processor") and from a processor node's
// menu (edit mode, `processor` prop set).
//
// The flagship affordance is the LIVE PREVIEW: as the operator edits the
// template, the drawer (debounced) POSTs /processors/preview and renders
// real recent messages from the input topic before → after, server-side,
// so there is no Go↔JS template drift. When the input topic has no
// recent messages it falls back to a synthetic sample so the editor is
// never blank.

import { FC, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Divider from '@mui/material/Divider'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CloseIcon from '@mui/icons-material/Close'
import ArrowRightAltIcon from '@mui/icons-material/ArrowRightAlt'

import useSnackbar from '../../hooks/useSnackbar'
import {
  ProcessorDTO,
  ProcessorPreviewPair,
  useCreateHelixOrgProcessor,
  useUpdateHelixOrgProcessor,
  usePreviewHelixOrgProcessor,
  useListHelixOrgTopics,
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
  { label: 'match', match: '{{ if hasSuffix "@vip.com" .Message.from }}1{{ end }}' },
  { label: 'default', match: '' },
]

const DEFAULT_TEMPLATE = 'From {{ .Message.from }}: {{ .Message.subject }}\n\n{{ .Message.body }}'

const SYNTHETIC_SAMPLE = { from: 'alice@example.com', subject: 'Invoice #7', body: 'Please review the attached invoice.' }

const ProcessorConfigDrawer: FC<ProcessorConfigDrawerProps> = ({ open, onClose, processor, presetInputTopicId }) => {
  const snackbar = useSnackbar()
  const isEdit = Boolean(processor)
  const createProc = useCreateHelixOrgProcessor()
  const updateProc = useUpdateHelixOrgProcessor()
  const preview = usePreviewHelixOrgProcessor()
  const { data: topicsData } = useListHelixOrgTopics({ enabled: open })

  const [name, setName] = useState('')
  const [kind, setKind] = useState('template')
  const [inputTopicId, setInputTopicId] = useState('')
  const [template, setTemplate] = useState(DEFAULT_TEMPLATE)
  const [maxBytes, setMaxBytes] = useState('500')
  const [filterRows, setFilterRows] = useState<FilterRow[]>(DEFAULT_FILTER_ROWS)
  const [pairs, setPairs] = useState<ProcessorPreviewPair[]>([])
  const [previewErr, setPreviewErr] = useState('')

  // Reset form each open from the processor under edit (or defaults).
  useEffect(() => {
    if (!open) return
    setPairs([])
    setPreviewErr('')
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

  // Debounced live preview. Fetches real messages from the input topic;
  // falls back to a synthetic sample when the topic has none.
  const previewRef = useRef(preview)
  previewRef.current = preview
  useEffect(() => {
    if (!open) return
    const handle = setTimeout(async () => {
      try {
        let res = await previewRef.current.mutateAsync({
          kind,
          config,
          outputs,
          input_topic_id: inputTopicId || undefined,
          count: 5,
        })
        if (res.length === 0) {
          res = await previewRef.current.mutateAsync({ kind, config, outputs, samples: [SYNTHETIC_SAMPLE] })
        }
        setPairs(res)
        setPreviewErr('')
      } catch (err: any) {
        setPairs([])
        setPreviewErr(err?.response?.data?.errors?.[0]?.detail ?? err?.response?.data?.error ?? err?.message ?? 'preview failed')
      }
    }, 350)
    return () => clearTimeout(handle)
  }, [open, kind, config, outputs, inputTopicId])

  const canSubmit = Boolean(name.trim()) && (isEdit || Boolean(inputTopicId))

  const submit = async () => {
    try {
      if (isEdit && processor) {
        await updateProc.mutateAsync({ id: processor.id, attrs: { name: name.trim(), kind, config } })
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
            helperText="A new kind is one Go file — template + truncate ship in v1."
          >
            {KINDS.map((k) => <MenuItem key={k.value} value={k.value}>{k.label}</MenuItem>)}
          </TextField>
          {isEdit ? (
            <Box>
              <Typography variant="caption" color="text.secondary">Input topic (immutable)</Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{inputTopicId}</Typography>
            </Box>
          ) : (
            <TextField
              select size="small" label="Input topic" value={inputTopicId} fullWidth required
              onChange={(e) => setInputTopicId(e.target.value)}
              helperText="The topic this processor reads. An output topic is auto-created; subscribe workers to it on the chart."
            >
              {topics.map((tp) => (
                <MenuItem key={tp.id} value={tp.id ?? ''} sx={{ fontFamily: 'monospace' }}>
                  {tp.id}{tp.name ? ` — ${tp.name}` : ''}
                </MenuItem>
              ))}
            </TextField>
          )}
          <Divider sx={{ my: 0.5 }} />
          {kind === 'template' && (
            <TextField
              size="small" label="Template (Go text/template)" value={template} fullWidth
              onChange={(e) => setTemplate(e.target.value)}
              multiline minRows={4}
              InputProps={{ sx: { fontFamily: 'monospace', fontSize: '0.8rem' } }}
              helperText="Fields: .Message.body/.from/.subject/.thread_id … · funcs: upper lower trunc default contains hasPrefix hasSuffix"
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
                Outputs — each is one branch: a boolean predicate + a destination topic (auto-created).
                A message is published to every output whose predicate is truthy; an empty predicate is
                an unconditional default. {isEdit && '(Branches are immutable on edit — recreate to change.)'}
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

          {/* Live preview */}
          <Box sx={{ mt: 0.5 }}>
            <Typography variant="caption" color="text.secondary">
              Live preview {inputTopicId ? `(recent messages from ${inputTopicId})` : '(synthetic sample)'}
            </Typography>
            {previewErr ? (
              <Typography variant="body2" sx={{ color: 'error.main', mt: 0.5, fontFamily: 'monospace', fontSize: '0.75rem' }}>
                {previewErr}
              </Typography>
            ) : pairs.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, fontStyle: 'italic' }}>
                {preview.isPending ? 'Rendering…' : 'No preview yet.'}
              </Typography>
            ) : (
              <Stack spacing={1} sx={{ mt: 0.5 }}>
                {pairs.map((p, i) => (
                  <Box key={i} sx={{ border: '1px solid rgba(0,0,0,0.08)', borderRadius: 1, p: 1 }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace', fontSize: '0.7rem' }}>
                      before
                    </Typography>
                    <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', fontFamily: 'monospace', fontSize: '0.75rem', mb: 0.5 }}>
                      {p.before?.body || '(empty)'}
                    </Typography>
                    <Stack direction="row" alignItems="center" spacing={0.5} sx={{ color: 'text.secondary' }}>
                      <ArrowRightAltIcon sx={{ fontSize: 16 }} />
                      <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}>after</Typography>
                    </Stack>
                    {p.error ? (
                      <Typography variant="body2" sx={{ color: 'error.main', fontFamily: 'monospace', fontSize: '0.75rem' }}>
                        {p.error}
                      </Typography>
                    ) : kind === 'filter' ? (
                      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: (p.after ?? []).length === 0 ? 'warning.main' : 'success.main' }}>
                        {(p.after ?? []).length === 0
                          ? 'dropped (no branch matched)'
                          : `routed to ${(p.after ?? []).length} branch${(p.after ?? []).length === 1 ? '' : 'es'}`}
                      </Typography>
                    ) : (p.after ?? []).length === 0 ? (
                      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: 'warning.main' }}>
                        dropped
                      </Typography>
                    ) : (
                      (p.after ?? []).map((a, j) => (
                        <Typography key={j} variant="body2" sx={{ whiteSpace: 'pre-wrap', fontFamily: 'monospace', fontSize: '0.75rem' }}>
                          {a.body || '(empty)'}
                        </Typography>
                      ))
                    )}
                  </Box>
                ))}
              </Stack>
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
