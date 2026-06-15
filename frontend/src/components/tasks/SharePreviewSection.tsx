import React, { FC, useState } from 'react'
import {
  Alert,
  Box,
  Button,
  Chip,
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
import LaunchIcon from '@mui/icons-material/Launch'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import DeleteIcon from '@mui/icons-material/Delete'
import RefreshIcon from '@mui/icons-material/Refresh'
import CodeIcon from '@mui/icons-material/Code'
import ShareIcon from '@mui/icons-material/Share'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { TypesVHostRoute } from '../../api/api'

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
  const api = useApi()
  const apiClient = api.getApiClient()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()

  const [portInput, setPortInput] = useState('8080')
  const [embedOpenFor, setEmbedOpenFor] = useState<TypesVHostRoute | null>(null)

  const queryKey = ['session-preview-tokens', sessionId]

  const { data: tokens, isLoading } = useQuery<TypesVHostRoute[]>({
    queryKey,
    enabled: !!sessionId,
    queryFn: async () => {
      const res = await apiClient.v1SessionsPreviewTokensDetail(sessionId)
      return res.data ?? []
    },
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey })

  const mintMutation = useMutation({
    mutationFn: async (port: number) => {
      const res = await apiClient.v1SessionsPreviewTokensCreate(sessionId, { port } as any)
      return res.data
    },
    onSuccess: () => invalidate(),
    onError: (e: any) =>
      snackbar.error(`Couldn't create preview: ${e?.response?.data ?? e?.message ?? e}`),
  })

  const rotateMutation = useMutation({
    mutationFn: async (tokenId: string) => {
      await apiClient.v1SessionsPreviewTokensRotateCreate(sessionId, tokenId)
    },
    onSuccess: () => {
      snackbar.success('Preview URL rotated — the old link no longer works')
      invalidate()
    },
    onError: (e: any) => snackbar.error(`Rotate failed: ${e?.message ?? e}`),
  })

  const deleteMutation = useMutation({
    mutationFn: async (tokenId: string) => {
      await apiClient.v1SessionsPreviewTokensDelete(sessionId, tokenId)
    },
    onSuccess: () => {
      snackbar.success('Preview revoked')
      invalidate()
    },
    onError: (e: any) => snackbar.error(`Revoke failed: ${e?.message ?? e}`),
  })

  if (!sessionId) {
    return null
  }

  const handleMint = () => {
    const n = parseInt(portInput, 10)
    if (!Number.isInteger(n) || n < 1 || n > 65535) {
      snackbar.error('Port must be a whole number 1..65535')
      return
    }
    mintMutation.mutate(n)
  }

  const hasTokens = !!tokens && tokens.length > 0

  return (
    <Box sx={{ mb: 3 }}>
      <Stack direction="row" alignItems="center" spacing={1} mb={1}>
        <ShareIcon fontSize="small" />
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
                onOpen={() => window.open(`https://${t.hostname}/`, '_blank', 'noopener')}
                onCopy={() => {
                  navigator.clipboard.writeText(`https://${t.hostname}/`)
                  snackbar.success('URL copied')
                }}
                onEmbed={() => setEmbedOpenFor(t)}
                onRotate={() => rotateMutation.mutate(t.id!)}
                onDelete={() => deleteMutation.mutate(t.id!)}
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
          startIcon={<ShareIcon />}
          disabled={mintMutation.isPending}
          onClick={handleMint}
        >
          {hasTokens ? 'Share another port' : 'Create share URL'}
        </Button>
      </Stack>

      <EmbedDialog
        token={embedOpenFor}
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
  onOpen: () => void
  onCopy: () => void
  onEmbed: () => void
  onRotate: () => void
  onDelete: () => void
  disabled: boolean
}> = ({ token, onOpen, onCopy, onEmbed, onRotate, onDelete, disabled }) => (
  <Alert
    icon={false}
    severity="info"
    sx={{
      '& .MuiAlert-message': { width: '100%' },
    }}
  >
    <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap">
      <Chip size="small" label={`port ${token.port}`} />
      <Typography
        variant="body2"
        sx={{ fontFamily: 'monospace', wordBreak: 'break-all', flex: 1 }}
      >
        https://{token.hostname}/
      </Typography>
      <Tooltip title="Open in a new tab">
        <span>
          <IconButton size="small" onClick={onOpen} disabled={disabled}>
            <LaunchIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Copy URL">
        <span>
          <IconButton size="small" onClick={onCopy} disabled={disabled}>
            <ContentCopyIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Embed as iframe">
        <span>
          <IconButton size="small" onClick={onEmbed} disabled={disabled}>
            <CodeIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Rotate (old URL stops working)">
        <span>
          <IconButton size="small" onClick={onRotate} disabled={disabled}>
            <RefreshIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Revoke">
        <span>
          <IconButton size="small" onClick={onDelete} disabled={disabled}>
            <DeleteIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
    </Stack>
  </Alert>
)

const EmbedDialog: FC<{
  token: TypesVHostRoute | null
  onClose: () => void
  onCopy: (snippet: string) => void
}> = ({ token, onClose, onCopy }) => {
  if (!token) return null
  const url = `https://${token.hostname}/`
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
        <Button onClick={() => onCopy(snippet)} startIcon={<ContentCopyIcon />}>
          Copy snippet
        </Button>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  )
}

export default SharePreviewSection
