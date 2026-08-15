// Right-side detail drawer for a single LLM call in the admin panel.
// Shows full metadata, token/cost breakdown, a request summary, and
// tabbed JSON views of request / response / original request / error.

import React, { FC, useMemo, useState, useEffect } from 'react'
import {
  Box,
  Chip,
  Divider,
  Drawer,
  IconButton,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from '@mui/material'
import { X, Copy } from 'lucide-react'
import { TypesLLMCall } from '../../api/api'
import JsonView from '../widgets/JsonView'
import useSnackbar from '../../hooks/useSnackbar'

export const formatTokenCount = (n?: number): string => {
  if (n === undefined || n === null) return '-'
  if (n >= 1000000) return `${(n / 1000000).toFixed(2)}M`
  if (n >= 10000) return `${(n / 1000).toFixed(1)}k`
  return n.toLocaleString()
}

const formatCost = (n?: number): string | undefined => {
  if (!n) return undefined
  return `$${n.toFixed(n < 0.01 ? 6 : 4)}`
}

const formatDuration = (ms?: number): string => {
  if (ms === undefined || ms === null) return '-'
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${ms}ms`
}

// Pull a plain-text preview of the assistant output out of either an
// OpenAI-shaped (choices[0].message) or Anthropic-shaped (content blocks)
// response object.
const extractResponseSummary = (response: any): {
  text?: string
  toolCalls?: { name: string, arguments?: string }[]
  finishReason?: string
} => {
  if (!response || typeof response !== 'object') return {}
  const choice = response.choices?.[0]
  if (choice) {
    const toolCalls = (choice.message?.tool_calls || []).map((tc: any) => ({
      name: tc.function?.name || tc.id || 'tool',
      arguments: tc.function?.arguments,
    }))
    return {
      text: choice.message?.content || undefined,
      toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
      finishReason: choice.finish_reason || undefined,
    }
  }
  if (Array.isArray(response.content)) {
    const text = response.content
      .filter((b: any) => b.type === 'text')
      .map((b: any) => b.text)
      .join('\n') || undefined
    const toolCalls = response.content
      .filter((b: any) => b.type === 'tool_use')
      .map((b: any) => ({ name: b.name, arguments: JSON.stringify(b.input) }))
    return {
      text,
      toolCalls: toolCalls.length > 0 ? toolCalls : undefined,
      finishReason: response.stop_reason || undefined,
    }
  }
  return {}
}

const summarizeRequest = (request: any): { label: string, value: string }[] => {
  if (!request || typeof request !== 'object') return []
  const out: { label: string, value: string }[] = []
  const messages = request.messages
  if (Array.isArray(messages)) {
    out.push({ label: 'Messages', value: String(messages.length) })
    const chars = messages.reduce((acc: number, m: any) => {
      if (typeof m.content === 'string') return acc + m.content.length
      if (Array.isArray(m.content)) {
        return acc + m.content.reduce((a: number, p: any) => a + (typeof p.text === 'string' ? p.text.length : 0), 0)
      }
      return acc
    }, 0)
    out.push({ label: 'Context size', value: `${chars.toLocaleString()} chars` })
  }
  if (Array.isArray(request.tools) && request.tools.length > 0) {
    out.push({ label: 'Tools', value: String(request.tools.length) })
  }
  if (request.max_tokens) out.push({ label: 'Max tokens', value: String(request.max_tokens) })
  if (request.temperature !== undefined && request.temperature !== null) {
    out.push({ label: 'Temperature', value: String(request.temperature) })
  }
  if (request.reasoning_effort) out.push({ label: 'Reasoning effort', value: String(request.reasoning_effort) })
  if (request.stream !== undefined) out.push({ label: 'Stream', value: request.stream ? 'yes' : 'no' })
  return out
}

const MetaRow: FC<{ label: string, value?: React.ReactNode, copyValue?: string }> = ({ label, value, copyValue }) => {
  const snackbar = useSnackbar()
  if (value === undefined || value === null || value === '') return null
  return (
    <Stack direction="row" alignItems="center" spacing={1} sx={{ minHeight: 28 }}>
      <Typography variant="caption" color="text.secondary" sx={{ width: 130, flexShrink: 0 }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ fontFamily: 'var(--helix-font-mono)', fontSize: '0.8rem', wordBreak: 'break-all' }}>
        {value}
      </Typography>
      {copyValue && (
        <Tooltip title="Copy">
          <IconButton
            size="small"
            aria-label={`Copy ${label}`}
            sx={{ p: 0.25 }}
            onClick={() => {
              navigator.clipboard.writeText(copyValue)
              snackbar.success('Copied to clipboard')
            }}
          >
            <Copy size={13} />
          </IconButton>
        </Tooltip>
      )}
    </Stack>
  )
}

const StatBox: FC<{ label: string, value: string, hint?: string }> = ({ label, value, hint }) => (
  <Tooltip title={hint || ''}>
    <Box
      sx={{
        background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
        border: '1px solid rgba(255,255,255,0.06)',
        borderRadius: 2,
        p: 1.5,
        minWidth: 90,
        flex: 1,
      }}
    >
      <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.65rem', display: 'block' }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ fontSize: '0.85rem', fontFamily: 'var(--helix-font-mono)', fontWeight: 600 }}>
        {value}
      </Typography>
    </Box>
  </Tooltip>
)

interface LLMCallDrawerProps {
  call: TypesLLMCall | null
  open: boolean
  onClose: () => void
}

const LLMCallDrawer: FC<LLMCallDrawerProps> = ({ call, open, onClose }) => {
  const [tab, setTab] = useState<'response' | 'request' | 'original_request'>('response')

  useEffect(() => {
    if (open) setTab('response')
  }, [open, call?.id])

  const response = call?.response as any
  const request = call?.request as any
  const originalRequest = call?.original_request as any

  const responseSummary = useMemo(() => extractResponseSummary(response), [response])
  const requestSummary = useMemo(() => summarizeRequest(request), [request])

  const totalCost = formatCost(call?.total_cost)

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{ sx: { backgroundImage: 'none', width: 720, maxWidth: '100vw' } }}
    >
      {call && (
        <Box sx={{ p: 2.5, height: '100%', display: 'flex', flexDirection: 'column', boxSizing: 'border-box' }}>
          <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1, flexShrink: 0 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
              <Typography variant="h6" noWrap>{call.model || 'LLM Call'}</Typography>
              {call.error ? (
                <Chip label="error" size="small" color="error" />
              ) : (
                <Chip label="ok" size="small" color="success" variant="outlined" />
              )}
              {call.stream && <Chip label="stream" size="small" variant="outlined" />}
            </Stack>
            <IconButton size="small" onClick={onClose} aria-label="Close">
              <X size={18} />
            </IconButton>
          </Stack>

          <Box sx={{ flex: 1, overflow: 'auto', minHeight: 0 }}>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              {call.created ? new Date(call.created).toLocaleString() : ''}
              {call.provider ? ` · ${call.provider}` : ''}
              {call.step ? ` · ${call.step}` : ''}
            </Typography>

            <Stack direction="row" spacing={1} sx={{ mb: 1.5 }}>
              <StatBox label="Duration" value={formatDuration(call.duration_ms)} />
              <StatBox
                label="First token"
                value={formatDuration(call.time_to_first_token_ms)}
                hint="Time to first streamed token"
              />
              <StatBox
                label="Prompt tokens"
                value={formatTokenCount(call.prompt_tokens)}
                hint={`Context length: ${call.prompt_tokens?.toLocaleString() || '-'} tokens`}
              />
              <StatBox label="Completion" value={formatTokenCount(call.completion_tokens)} />
              <StatBox label="Total" value={formatTokenCount(call.total_tokens)} />
            </Stack>

            {(call.cache_read_tokens || call.cache_write_tokens || totalCost) ? (
              <Stack direction="row" spacing={1} sx={{ mb: 1.5 }}>
                <StatBox label="Cache read" value={formatTokenCount(call.cache_read_tokens)} />
                <StatBox label="Cache write" value={formatTokenCount(call.cache_write_tokens)} />
                <StatBox label="Prompt cost" value={formatCost(call.prompt_cost) || '-'} />
                <StatBox label="Completion cost" value={formatCost(call.completion_cost) || '-'} />
                <StatBox label="Total cost" value={totalCost || '-'} />
              </Stack>
            ) : null}

            <Divider sx={{ my: 1.5 }} />

            <MetaRow label="Call ID" value={call.id} copyValue={call.id} />
            <MetaRow label="Session" value={call.session_id} copyValue={call.session_id} />
            <MetaRow label="Interaction" value={call.interaction_id} copyValue={call.interaction_id} />
            <MetaRow label="App" value={call.app_id} copyValue={call.app_id} />
            <MetaRow label="Project" value={call.project_id} copyValue={call.project_id} />
            <MetaRow label="Spec task" value={call.spec_task_id} copyValue={call.spec_task_id} />
            <MetaRow label="User" value={call.user_id} copyValue={call.user_id} />
            <MetaRow label="Agent runtime" value={call.code_agent_runtime} />
            {requestSummary.map(item => (
              <MetaRow key={item.label} label={item.label} value={item.value} />
            ))}
            <MetaRow label="Finish reason" value={responseSummary.finishReason} />
            {responseSummary.toolCalls && (
              <MetaRow
                label="Tool calls"
                value={responseSummary.toolCalls.map(tc => tc.name).join(', ')}
              />
            )}

            {call.error && (
              <Box
                sx={{
                  mt: 1.5,
                  p: 1.5,
                  borderRadius: 1,
                  border: '1px solid',
                  borderColor: 'error.main',
                  backgroundColor: 'rgba(244, 67, 54, 0.08)',
                }}
              >
                <Typography variant="caption" color="error" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
                  Error
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'var(--helix-font-mono)', fontSize: '0.8rem', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                  {call.error}
                </Typography>
              </Box>
            )}

            {responseSummary.text && (
              <Box sx={{ mt: 1.5 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                  Response text
                </Typography>
                <Box
                  sx={{
                    mt: 0.5,
                    p: 1.5,
                    borderRadius: 1,
                    backgroundColor: '#121212',
                    maxHeight: 220,
                    overflow: 'auto',
                  }}
                >
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: '0.8rem' }}>
                    {responseSummary.text}
                  </Typography>
                </Box>
              </Box>
            )}

            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mt: 2, mb: 1, minHeight: 36 }}>
              <Tab value="response" label="Response" sx={{ minHeight: 36, py: 0.5 }} />
              <Tab value="request" label="Request" sx={{ minHeight: 36, py: 0.5 }} />
              {originalRequest && (
                <Tab value="original_request" label="Original request" sx={{ minHeight: 36, py: 0.5 }} />
              )}
            </Tabs>

            {tab === 'response' && (
              <JsonView data={call.error && !response ? { error: call.error } : response} withFancyRendering={false} withFancyRenderingControls={false} />
            )}
            {tab === 'request' && (
              <JsonView data={request} withFancyRendering={false} withFancyRenderingControls={false} />
            )}
            {tab === 'original_request' && (
              <JsonView data={originalRequest} withFancyRendering={false} withFancyRenderingControls={false} />
            )}
          </Box>
        </Box>
      )}
    </Drawer>
  )
}

export default LLMCallDrawer
