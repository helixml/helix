import {
  Box,
  Button,
  Dialog,
  DialogContent,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined'
import { FC, useMemo, useRef, useState } from 'react'
import {
  attachmentContentURL,
  SPEC_TASK_ATTACHMENT_ACCEPTED_MIME,
  SPEC_TASK_ATTACHMENT_MAX_BYTES,
  SPEC_TASK_ATTACHMENT_MAX_PER_TASK,
  useDeleteSpecTaskAttachment,
  useSpecTaskAttachments,
  useUploadSpecTaskAttachments,
} from '../../services/specTaskAttachmentsService'
import { TypesSpecTaskStatus } from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'

// Statuses past which attachments are read-only — the server also enforces this
// (returns 409) but we hide the buttons up-front for a nicer UX.
const READ_ONLY_STATUSES: Partial<Record<TypesSpecTaskStatus, true>> = {
  spec_approved: true,
  implementation_queued: true,
  implementation: true,
  implementation_review: true,
  pull_request: true,
  done: true,
  implementation_failed: true,
} as any

const ACCEPT_ATTR = Object.entries(SPEC_TASK_ATTACHMENT_ACCEPTED_MIME)
  .flatMap(([mime, exts]) => [mime, ...exts])
  .join(',')

function humanSize(n: number): string {
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  if (n >= 1024) return `${Math.round(n / 1024)} KB`
  return `${n} B`
}

interface TaskAttachmentsPanelProps {
  taskId: string
  status?: TypesSpecTaskStatus
}

const TaskAttachmentsPanel: FC<TaskAttachmentsPanelProps> = ({ taskId, status }) => {
  const { data: attachments = [], isLoading } = useSpecTaskAttachments(taskId)
  const upload = useUploadSpecTaskAttachments()
  const deleteMut = useDeleteSpecTaskAttachment()
  const fileInput = useRef<HTMLInputElement | null>(null)
  const snackbar = useSnackbar()
  const [lightboxURL, setLightboxURL] = useState<string | null>(null)

  const readOnly = !!status && READ_ONLY_STATUSES[status]
  const remaining = useMemo(
    () => SPEC_TASK_ATTACHMENT_MAX_PER_TASK - attachments.length,
    [attachments.length],
  )

  const onPickFiles = async (files: File[]) => {
    if (!files.length) return
    if (files.length > remaining) {
      snackbar.error(`Can only attach ${remaining} more file(s) — limit is ${SPEC_TASK_ATTACHMENT_MAX_PER_TASK}`)
      return
    }
    const valid: File[] = []
    for (const f of files) {
      if (f.size > SPEC_TASK_ATTACHMENT_MAX_BYTES) {
        snackbar.error(`${f.name} is too large (max ${humanSize(SPEC_TASK_ATTACHMENT_MAX_BYTES)})`)
        continue
      }
      valid.push(f)
    }
    if (!valid.length) return
    try {
      await upload.mutateAsync({ taskId, files: valid })
    } catch (e: any) {
      snackbar.error(e?.response?.data || e?.message || 'Upload failed')
    }
  }

  const onDelete = async (attId: string, filename: string) => {
    if (!window.confirm(`Remove ${filename}?`)) return
    try {
      await deleteMut.mutateAsync({ taskId, attachmentId: attId })
    } catch (e: any) {
      snackbar.error(e?.response?.data || e?.message || 'Delete failed')
    }
  }

  if (isLoading) return null
  if (!attachments.length && readOnly) return null

  return (
    <Box
      sx={{
        border: '1px solid rgba(255,255,255,0.08)',
        borderRadius: 1,
        p: 2,
        mb: 2,
        background: 'rgba(255,255,255,0.02)',
      }}
    >
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1 }}>
        <Typography variant="subtitle2">
          Attachments {attachments.length > 0 && `(${attachments.length}/${SPEC_TASK_ATTACHMENT_MAX_PER_TASK})`}
        </Typography>
        {!readOnly && (
          <Button
            component="label"
            role={undefined}
            tabIndex={-1}
            size="small"
            variant="outlined"
            disabled={upload.isPending || remaining <= 0}
          >
            {upload.isPending ? 'Uploading…' : 'Add files'}
            <input
              ref={fileInput}
              type="file"
              multiple
              accept={ACCEPT_ATTR}
              style={{
                clip: 'rect(0 0 0 0)',
                clipPath: 'inset(50%)',
                height: 1,
                overflow: 'hidden',
                position: 'absolute',
                bottom: 0,
                left: 0,
                whiteSpace: 'nowrap',
                width: 1,
              }}
              onChange={(e) => {
                const files = Array.from(e.target.files || [])
                e.target.value = '' // reset so re-picking the same file fires onChange
                void onPickFiles(files)
              }}
            />
          </Button>
        )}
      </Stack>

      {attachments.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          Drop screenshots, PDFs, or text documents here so the agent has context.
          Limit: {SPEC_TASK_ATTACHMENT_MAX_PER_TASK} files, {humanSize(SPEC_TASK_ATTACHMENT_MAX_BYTES)} each.
        </Typography>
      )}

      <Stack direction="row" flexWrap="wrap" spacing={1} useFlexGap>
        {attachments.map((a) => {
          const isImage = (a.mime_type || '').startsWith('image/') && a.mime_type !== 'image/svg+xml'
          const url = attachmentContentURL(taskId, a.id || '')
          return (
            <Box
              key={a.id}
              sx={{
                width: 220,
                border: '1px solid rgba(255,255,255,0.06)',
                borderRadius: 1,
                p: 1,
                display: 'flex',
                flexDirection: 'column',
                gap: 0.5,
              }}
            >
              <Box
                sx={{
                  width: '100%',
                  height: 100,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  overflow: 'hidden',
                  background: 'rgba(0,0,0,0.2)',
                  borderRadius: 0.5,
                  cursor: 'pointer',
                }}
                onClick={() => {
                  if (isImage) setLightboxURL(url)
                  else window.open(url, '_blank')
                }}
              >
                {isImage ? (
                  <img
                    src={url}
                    alt={a.filename || ''}
                    style={{ maxWidth: '100%', maxHeight: '100%', objectFit: 'contain' }}
                  />
                ) : (
                  <InsertDriveFileOutlinedIcon sx={{ fontSize: 56, opacity: 0.6 }} />
                )}
              </Box>
              <Tooltip title={a.filename || ''}>
                <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>
                  {a.filename}
                </Typography>
              </Tooltip>
              <Stack direction="row" alignItems="center" justifyContent="space-between">
                <Typography variant="caption" color="text.secondary">
                  {humanSize(a.size_bytes || 0)}
                </Typography>
                {!readOnly && (
                  <IconButton
                    size="small"
                    onClick={() => onDelete(a.id || '', a.filename || '')}
                    disabled={deleteMut.isPending}
                  >
                    <DeleteOutlineIcon fontSize="small" />
                  </IconButton>
                )}
              </Stack>
            </Box>
          )
        })}
      </Stack>

      <Dialog open={!!lightboxURL} onClose={() => setLightboxURL(null)} maxWidth="lg">
        <IconButton
          onClick={() => setLightboxURL(null)}
          sx={{ position: 'absolute', top: 8, right: 8, color: 'white', zIndex: 1 }}
        >
          <CloseIcon />
        </IconButton>
        <DialogContent sx={{ p: 0, background: '#000' }}>
          {lightboxURL && (
            <img src={lightboxURL} alt="" style={{ maxWidth: '90vw', maxHeight: '90vh', display: 'block' }} />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  )
}

export default TaskAttachmentsPanel
