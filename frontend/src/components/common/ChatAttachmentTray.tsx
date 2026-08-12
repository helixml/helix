import { FC, useMemo, useState } from 'react'
import { Box, CircularProgress, IconButton, Tooltip, Typography, alpha } from '@mui/material'
import { CircleAlert, FileText, Paperclip, RefreshCw, X } from 'lucide-react'

import { prettyBytes } from '../../utils/format'
import ImageLightbox, { LightboxImage } from '../session/ImageLightbox'
import { getChatColors } from '../session/chatStyles'
import { PendingChatAttachment } from './chatAttachments'

interface ChatAttachmentTrayProps {
  attachments: PendingChatAttachment[]
  onRemove: (id: string) => void
  onRetry: (id: string) => void
}

const ChatAttachmentTray: FC<ChatAttachmentTrayProps> = ({ attachments, onRemove, onRetry }) => {
  const [selectedImageIndex, setSelectedImageIndex] = useState<number | null>(null)
  const images = useMemo(
    () => attachments.filter(
      (attachment): attachment is PendingChatAttachment & { previewUrl: string } =>
        attachment.type === 'image' && !!attachment.previewUrl,
    ),
    [attachments],
  )
  const lightboxImages: LightboxImage[] = images.map((attachment) => ({
    src: attachment.previewUrl,
    name: attachment.name,
  }))

  if (attachments.length === 0) return null

  return (
    <>
      <Box
        data-chat-attachment-tray
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'stretch',
          gap: 1,
          mb: 1.5,
        }}
      >
        {attachments.map((attachment) => {
          const imageIndex = images.findIndex((image) => image.id === attachment.id)
          const isWorking = attachment.uploadStatus === 'pending' || attachment.uploadStatus === 'uploading'
          const failed = attachment.uploadStatus === 'failed'

          if (attachment.type === 'image' && attachment.previewUrl) {
            return (
              <Box
                key={attachment.id}
                data-chat-attachment={attachment.name}
                sx={{
                  width: 64,
                  height: 64,
                  position: 'relative',
                  overflow: 'hidden',
                  borderRadius: 2,
                  border: '1px solid',
                  borderColor: failed ? 'error.main' : (theme) => getChatColors(theme).borderStrong,
                  bgcolor: (theme) => getChatColors(theme).canvas,
                }}
              >
                <Box
                  component="button"
                  type="button"
                  onClick={() => setSelectedImageIndex(imageIndex)}
                  aria-label={`Preview ${attachment.name}`}
                  sx={{ width: '100%', height: '100%', p: 0, border: 0, background: 'transparent', cursor: 'zoom-in' }}
                >
                  <Box
                    component="img"
                    src={attachment.previewUrl}
                    alt={attachment.name}
                    sx={{ width: '100%', height: '100%', display: 'block', objectFit: 'cover' }}
                  />
                </Box>
                {isWorking && (
                  <Box
                    aria-label={`Uploading ${attachment.name}`}
                    sx={{
                      position: 'absolute',
                      inset: 0,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      bgcolor: 'rgba(0,0,0,0.48)',
                      color: '#fff',
                      pointerEvents: 'none',
                    }}
                  >
                    <CircularProgress size={18} sx={{ color: 'inherit' }} />
                  </Box>
                )}
                {failed && (
                  <Tooltip title={attachment.error || 'Upload failed'}>
                    <IconButton
                      size="small"
                      onClick={() => onRetry(attachment.id)}
                      aria-label={`Retry upload ${attachment.name}`}
                      sx={{
                        position: 'absolute',
                        left: 3,
                        bottom: 3,
                        width: 24,
                        height: 24,
                        bgcolor: 'rgba(0,0,0,0.72)',
                        color: '#fff',
                        '&:hover': { bgcolor: 'rgba(0,0,0,0.9)' },
                      }}
                    >
                      <RefreshCw size={13} />
                    </IconButton>
                  </Tooltip>
                )}
                <IconButton
                  size="small"
                  onClick={() => onRemove(attachment.id)}
                  aria-label={`Remove ${attachment.name}`}
                  sx={{
                    position: 'absolute',
                    top: 3,
                    right: 3,
                    width: 20,
                    height: 20,
                    color: '#fff',
                    bgcolor: 'rgba(0,0,0,0.64)',
                    '&:hover': { bgcolor: 'rgba(0,0,0,0.84)' },
                  }}
                >
                  <X size={13} />
                </IconButton>
              </Box>
            )
          }

          return (
            <Box
              key={attachment.id}
              data-chat-attachment={attachment.name}
              sx={{
                minWidth: 0,
                width: { xs: '100%', sm: 210 },
                height: 64,
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                px: 1.25,
                pr: 0.5,
                border: '1px solid',
                borderColor: failed ? 'error.main' : (theme) => getChatColors(theme).border,
                borderRadius: 2,
                bgcolor: (theme) => alpha(getChatColors(theme).canvas, 0.62),
              }}
            >
              <Box sx={{ display: 'flex', color: failed ? 'error.main' : 'text.secondary', flexShrink: 0 }}>
                {failed ? <CircleAlert size={20} /> : attachment.type === 'text' ? <FileText size={20} /> : <Paperclip size={20} />}
              </Box>
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Tooltip title={attachment.name} disableHoverListener={attachment.name.length < 24}>
                  <Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', lineHeight: 1.35 }}>
                    {attachment.name}
                  </Typography>
                </Tooltip>
                <Typography variant="caption" noWrap sx={{ display: 'block', color: failed ? 'error.main' : 'text.secondary', fontSize: '0.68rem' }}>
                  {failed ? attachment.error || 'Upload failed' : isWorking ? 'Uploading…' : prettyBytes(attachment.sizeBytes)}
                </Typography>
              </Box>
              {isWorking ? (
                <CircularProgress size={15} sx={{ mr: 0.5, flexShrink: 0 }} />
              ) : failed ? (
                <Tooltip title="Retry upload">
                  <IconButton size="small" onClick={() => onRetry(attachment.id)} aria-label={`Retry upload ${attachment.name}`}>
                    <RefreshCw size={15} />
                  </IconButton>
                </Tooltip>
              ) : null}
              <IconButton size="small" onClick={() => onRemove(attachment.id)} aria-label={`Remove ${attachment.name}`}>
                <X size={15} />
              </IconButton>
            </Box>
          )
        })}
      </Box>

      <ImageLightbox
        images={lightboxImages}
        initialIndex={selectedImageIndex ?? 0}
        open={selectedImageIndex !== null}
        onClose={() => setSelectedImageIndex(null)}
      />
    </>
  )
}

export default ChatAttachmentTray
