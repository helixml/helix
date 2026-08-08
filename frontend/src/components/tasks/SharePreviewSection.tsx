import React, { FC, useState } from 'react'
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { ArrowRight, Code2, Copy, ExternalLink, RotateCw, Share2, Trash2 } from 'lucide-react'

import useSnackbar from '../../hooks/useSnackbar'
import { useGetConfig } from '../../services/userService'
import { TypesVHostRoute } from '../../api/api'
import {
  useCreateSessionPreviewToken,
  useDeleteSessionPreviewToken,
  useRotateSessionPreviewToken,
  useSessionPreviewTokens,
} from '../../services/sessionPreviewService'
import { sandboxPreviewUrl } from './sandboxBrowserUrl'

interface SharePreviewSectionProps {
  sessionId: string
}

/**
 * SharePreviewSection — UI on a spec task page for sharing a running
 * preview of whatever's bound to the session's container port. Mints a
 * random `share-…` hostname, lets the user open it in a new tab, copy
 * the URL, or grab an iframe snippet to embed it elsewhere.
 *
 * Lives on the spec task detail page because the session ID is in scope
 * there; the same control is meaningful on any sandbox detail page that
 * has access to a `ses_*` ID.
 */
const SharePreviewSection: FC<SharePreviewSectionProps> = ({ sessionId }) => {
  const snackbar = useSnackbar()
  const configQuery = useGetConfig()
  const previewURLHTTPS = configQuery.data?.preview_url_https ?? true

  const [portInput, setPortInput] = useState('8080')
  const [embedOpenFor, setEmbedOpenFor] = useState<TypesVHostRoute | null>(null)

  const { data: tokens, isLoading } = useSessionPreviewTokens(sessionId)
  const mintMutation = useCreateSessionPreviewToken(sessionId)
  const rotateMutation = useRotateSessionPreviewToken(sessionId)
  const deleteMutation = useDeleteSessionPreviewToken(sessionId)

  if (!sessionId) {
    return null
  }

  const handleMint = () => {
    const n = parseInt(portInput, 10)
    if (!Number.isInteger(n) || n < 1 || n > 65535) {
      snackbar.error('Port must be a whole number 1..65535')
      return
    }
    mintMutation.mutate(n, {
      onError: (error) => snackbar.error(
        `Couldn't create preview: ${error instanceof Error ? error.message : error}`,
      ),
    })
  }

  const hasTokens = !!tokens && tokens.length > 0

  return (
    <Box sx={{ mb: 3 }}>
      <Stack direction="row" alignItems="center" spacing={1} mb={1}>
        <Share2 size={18} />
        <Typography variant="subtitle2" color="text.secondary">
          Share preview URLs
        </Typography>
      </Stack>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Got an app running on a port in this agent's container? Share it
        with a teammate or embed it in a doc — Helix mints an
        unguessable URL that maps to that port. Revoke or rotate any
        time.
      </Typography>

      {isLoading ? (
        <CircularProgress size={20} />
      ) : (
        <Collapse in={hasTokens}>
          <Stack spacing={1} sx={{ mb: 2 }}>
            {(tokens ?? []).map((t) => (
              <PreviewTokenRow
                key={t.id}
                token={t}
                previewURLHTTPS={previewURLHTTPS}
                onOpen={() => window.open(sandboxPreviewUrl(t.url!, '/', previewURLHTTPS), '_blank', 'noopener')}
                onCopy={() => {
                  navigator.clipboard.writeText(sandboxPreviewUrl(t.url!, '/', previewURLHTTPS))
                  snackbar.success('URL copied')
                }}
                onEmbed={() => setEmbedOpenFor(t)}
                onRotate={() => rotateMutation.mutate(t.id!, {
                  onSuccess: () => snackbar.success('Preview URL rotated — the old link no longer works'),
                  onError: (error) => snackbar.error(`Rotate failed: ${error instanceof Error ? error.message : error}`),
                })}
                onDelete={() => deleteMutation.mutate(t.id!, {
                  onSuccess: () => snackbar.success('Preview revoked'),
                  onError: (error) => snackbar.error(`Revoke failed: ${error instanceof Error ? error.message : error}`),
                })}
                disabled={rotateMutation.isPending || deleteMutation.isPending}
              />
            ))}
          </Stack>
        </Collapse>
      )}

      <Stack direction="row" spacing={1} alignItems="center">
        <TextField
          size="small"
          label="Port"
          value={portInput}
          onChange={(e) => setPortInput(e.target.value)}
          sx={{ width: 110 }}
          inputProps={{ inputMode: 'numeric', pattern: '[0-9]*' }}
        />
        <Button
          variant="contained"
          size="small"
          startIcon={<Share2 size={18} />}
          disabled={mintMutation.isPending}
          onClick={handleMint}
          sx={{ textTransform: 'none' }}
        >
          {hasTokens ? 'Share another port' : 'Create share URL'}
        </Button>
      </Stack>

      <EmbedDialog
        token={embedOpenFor}
        previewURLHTTPS={previewURLHTTPS}
        onClose={() => setEmbedOpenFor(null)}
        onCopy={(snippet) => {
          navigator.clipboard.writeText(snippet)
          snackbar.success('Embed snippet copied')
        }}
      />
    </Box>
  )
}

const PreviewTokenRow: FC<{
  token: TypesVHostRoute
  previewURLHTTPS: boolean
  onOpen: () => void
  onCopy: () => void
  onEmbed: () => void
  onRotate: () => void
  onDelete: () => void
  disabled: boolean
}> = ({ token, previewURLHTTPS, onOpen, onCopy, onEmbed, onRotate, onDelete, disabled }) => (
  <Alert
    icon={false}
    severity="info"
    sx={{
      '& .MuiAlert-message': { width: '100%' },
    }}
  >
    <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap">
      <Typography variant="body2" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
        localhost:{token.port}
      </Typography>
      <ArrowRight size={16} aria-hidden />
      <Typography
        variant="body2"
        sx={{ fontFamily: 'monospace', wordBreak: 'break-all', flex: 1 }}
      >
        {sandboxPreviewUrl(token.url!, '/', previewURLHTTPS)}
      </Typography>
      <Tooltip title="Open in a new tab">
        <span>
          <IconButton size="small" aria-label="Open preview in new tab" onClick={onOpen} disabled={disabled}>
            <ExternalLink size={18} />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Copy URL">
        <span>
          <IconButton size="small" aria-label="Copy preview URL" onClick={onCopy} disabled={disabled}>
            <Copy size={18} />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Embed as iframe">
        <span>
          <IconButton size="small" aria-label="Embed preview" onClick={onEmbed} disabled={disabled}>
            <Code2 size={18} />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Rotate (old URL stops working)">
        <span>
          <IconButton size="small" aria-label="Rotate preview URL" onClick={onRotate} disabled={disabled}>
            <RotateCw size={18} />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Revoke">
        <span>
          <IconButton size="small" aria-label="Revoke preview URL" onClick={onDelete} disabled={disabled}>
            <Trash2 size={18} />
          </IconButton>
        </span>
      </Tooltip>
    </Stack>
  </Alert>
)

const EmbedDialog: FC<{
  token: TypesVHostRoute | null
  previewURLHTTPS: boolean
  onClose: () => void
  onCopy: (snippet: string) => void
}> = ({ token, previewURLHTTPS, onClose, onCopy }) => {
  if (!token) return null
  const url = sandboxPreviewUrl(token.url!, '/', previewURLHTTPS)
  const snippet = `<iframe src="${url}" width="100%" height="600" style="border:0" allow="clipboard-read; clipboard-write"></iframe>`
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Embed this preview</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          Paste this snippet into any HTML page, blog post, or docs site
          to embed the live preview as an iframe:
        </Typography>
        <Box
          sx={{
            fontFamily: 'monospace',
            fontSize: '0.85rem',
            p: 2,
            backgroundColor: 'rgba(0,0,0,0.15)',
            borderRadius: 1,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-all',
          }}
        >
          {snippet}
        </Box>
        <Typography variant="body2" sx={{ mt: 2, mb: 1 }}>
          Live preview:
        </Typography>
        <Box
          sx={{
            border: '1px solid rgba(255,255,255,0.1)',
            borderRadius: 1,
            overflow: 'hidden',
            height: 360,
          }}
        >
          <iframe
            src={url}
            title={token.hostname}
            width="100%"
            height="100%"
            style={{ border: 0 }}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onCopy(snippet)} startIcon={<Copy size={18} />}>
          Copy snippet
        </Button>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  )
}

export default SharePreviewSection
