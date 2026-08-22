import React, { FC, useMemo, useState } from 'react'
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Menu,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  Code2,
  Copy,
  EllipsisVertical,
  ExternalLink,
  RotateCw,
  Share2,
  Trash2,
} from 'lucide-react'
import SimpleTable from '../widgets/SimpleTable'
import { NEUTRAL_ACTION_BUTTON_SX } from '../../styles/actionButtons'

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
import { copyTextToClipboard } from '../../utils/clipboard'

const PREVIEW_TABLE_FIELDS = [
  { name: 'port', title: 'Port' },
  { name: 'url', title: 'Preview URL' },
]

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
  const [menu, setMenu] = useState<{ anchor: HTMLElement; token: TypesVHostRoute } | null>(null)

  const { data: tokens, isLoading } = useSessionPreviewTokens(sessionId)
  const mintMutation = useCreateSessionPreviewToken(sessionId)
  const rotateMutation = useRotateSessionPreviewToken(sessionId)
  const deleteMutation = useDeleteSessionPreviewToken(sessionId)

  const copyWithFeedback = async (text: string, successMessage: string) => {
    try {
      await copyTextToClipboard(text)
      snackbar.success(successMessage)
    } catch (error) {
      snackbar.error(`Copy failed: ${error instanceof Error ? error.message : error}`)
    }
  }

  // One row per shared port. The URL is deliberately single-line with an
  // ellipsis: these hostnames are ~50 chars and this panel is a narrow sidebar
  // column, so letting it wrap breaks it into a one-character-wide ribbon.
  // The full value is in the tooltip and one click away on the clipboard.
  const rows = useMemo(
    () =>
      (tokens ?? []).map((token) => {
        const url = sandboxPreviewUrl(token.url!, '/', previewURLHTTPS)
        return {
          id: token.id!,
          _data: token,
          port: (
            <Typography
              variant="body2"
              sx={{ fontFamily: 'var(--helix-font-mono)', color: 'text.secondary', whiteSpace: 'nowrap' }}
            >
              {token.port}
            </Typography>
          ),
          url: (
            <Tooltip title={`${url} — click to copy`}>
              <Typography
                component="button"
                variant="body2"
                onClick={() => void copyWithFeedback(url, 'URL copied')}
                sx={{
                  display: 'block',
                  width: '100%',
                  maxWidth: 260,
                  textAlign: 'left',
                  border: 0,
                  p: 0,
                  background: 'none',
                  cursor: 'pointer',
                  color: 'text.primary',
                  fontFamily: 'var(--helix-font-mono)',
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  '&:hover': { textDecoration: 'underline' },
                }}
              >
                {url.replace(/^https?:\/\//, '')}
              </Typography>
            </Tooltip>
          ),
        }
      }),
    [tokens, previewURLHTTPS],
  )

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
        Map a port in this agent's container to an unguessable public URL.
        Rotate or revoke any time.
      </Typography>

      {(isLoading || rows.length > 0) && (
        <Box sx={{ mb: 2 }}>
          <SimpleTable
            authenticated
            compact
            loading={isLoading}
            fields={PREVIEW_TABLE_FIELDS}
            data={rows}
            getActions={(row) => (
              <IconButton
                size="small"
                aria-label="Preview URL actions"
                onClick={(event) => setMenu({ anchor: event.currentTarget, token: row._data })}
              >
                <EllipsisVertical size={18} />
              </IconButton>
            )}
          />
        </Box>
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
          variant="text"
          size="small"
          startIcon={<Share2 size={16} />}
          disabled={mintMutation.isPending}
          onClick={handleMint}
          sx={NEUTRAL_ACTION_BUTTON_SX}
        >
          {hasTokens ? 'Share another port' : 'Create share URL'}
        </Button>
      </Stack>

      <Menu
        anchorEl={menu?.anchor ?? null}
        open={!!menu}
        onClose={() => setMenu(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        <MenuItem
          onClick={() => {
            if (menu) window.open(sandboxPreviewUrl(menu.token.url!, '/', previewURLHTTPS), '_blank', 'noopener')
            setMenu(null)
          }}
        >
          <ExternalLink size={20} style={{ marginRight: 8 }} />
          Open in new tab
        </MenuItem>
        <MenuItem
          onClick={() => {
            if (menu) void copyWithFeedback(sandboxPreviewUrl(menu.token.url!, '/', previewURLHTTPS), 'URL copied')
            setMenu(null)
          }}
        >
          <Copy size={20} style={{ marginRight: 8 }} />
          Copy URL
        </MenuItem>
        <MenuItem
          onClick={() => {
            setEmbedOpenFor(menu?.token ?? null)
            setMenu(null)
          }}
        >
          <Code2 size={20} style={{ marginRight: 8 }} />
          Embed as iframe
        </MenuItem>
        <MenuItem
          disabled={rotateMutation.isPending}
          onClick={() => {
            if (menu) {
              rotateMutation.mutate(menu.token.id!, {
                onSuccess: () => snackbar.success('Preview URL rotated — the old link no longer works'),
                onError: (error) => snackbar.error(`Rotate failed: ${error instanceof Error ? error.message : error}`),
              })
            }
            setMenu(null)
          }}
        >
          <RotateCw size={20} style={{ marginRight: 8 }} />
          Rotate URL
        </MenuItem>
        <MenuItem
          disabled={deleteMutation.isPending}
          onClick={() => {
            if (menu) {
              deleteMutation.mutate(menu.token.id!, {
                onSuccess: () => snackbar.success('Preview revoked'),
                onError: (error) => snackbar.error(`Revoke failed: ${error instanceof Error ? error.message : error}`),
              })
            }
            setMenu(null)
          }}
        >
          <Trash2 size={20} style={{ marginRight: 8 }} />
          Revoke
        </MenuItem>
      </Menu>

      <EmbedDialog
        token={embedOpenFor}
        previewURLHTTPS={previewURLHTTPS}
        onClose={() => setEmbedOpenFor(null)}
        onCopy={(snippet) => {
          void copyWithFeedback(snippet, 'Embed snippet copied')
        }}
      />
    </Box>
  )
}

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
