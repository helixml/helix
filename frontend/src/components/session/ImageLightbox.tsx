import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Dialog from '@mui/material/Dialog'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import CloseIcon from '@mui/icons-material/Close'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'

export interface LightboxImage {
  src: string
  name: string
}

interface ImageLightboxProps {
  images: LightboxImage[]
  initialIndex: number
  open: boolean
  onClose: () => void
}

const ImageLightbox: FC<ImageLightboxProps> = ({ images, initialIndex, open, onClose }) => {
  const [index, setIndex] = useState(initialIndex)

  useEffect(() => {
    if (open) setIndex(initialIndex)
  }, [initialIndex, open])

  useEffect(() => {
    if (!open) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'ArrowLeft' && images.length > 1) {
        setIndex((current) => (current - 1 + images.length) % images.length)
      }
      if (event.key === 'ArrowRight' && images.length > 1) {
        setIndex((current) => (current + 1) % images.length)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [images.length, open])

  const current = images[index]
  if (!current) return null

  const previous = () => setIndex((index - 1 + images.length) % images.length)
  const next = () => setIndex((index + 1) % images.length)

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth={false}
      aria-label={`Image preview: ${current.name}`}
      BackdropProps={{ sx: { backgroundColor: 'rgba(0, 0, 0, 0.78)' } }}
      PaperProps={{
        sx: {
          width: 'min(94vw, 1200px)',
          height: 'min(92vh, 900px)',
          m: 0,
          background: 'transparent',
          boxShadow: 'none',
          overflow: 'visible',
        },
      }}
    >
      <Box
        sx={{
          width: '100%',
          height: '100%',
          position: 'relative',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Box
          component="img"
          src={current.src}
          alt={current.name}
          sx={{
            display: 'block',
            maxWidth: '100%',
            maxHeight: 'calc(100% - 56px)',
            objectFit: 'contain',
            borderRadius: 1,
          }}
        />

        <Tooltip title="Close (Esc)">
          <IconButton
            onClick={onClose}
            aria-label="Close image preview"
            sx={{
              position: 'absolute',
              top: 0,
              right: 0,
              color: '#fff',
              backgroundColor: 'rgba(0,0,0,0.45)',
              '&:hover': { backgroundColor: 'rgba(0,0,0,0.7)' },
            }}
          >
            <CloseIcon />
          </IconButton>
        </Tooltip>

        {images.length > 1 && (
          <>
            <IconButton
              onClick={previous}
              aria-label="Previous image"
              sx={{
                position: 'absolute',
                left: 0,
                color: '#fff',
                backgroundColor: 'rgba(0,0,0,0.45)',
                '&:hover': { backgroundColor: 'rgba(0,0,0,0.7)' },
              }}
            >
              <ChevronLeftIcon />
            </IconButton>
            <IconButton
              onClick={next}
              aria-label="Next image"
              sx={{
                position: 'absolute',
                right: 0,
                color: '#fff',
                backgroundColor: 'rgba(0,0,0,0.45)',
                '&:hover': { backgroundColor: 'rgba(0,0,0,0.7)' },
              }}
            >
              <ChevronRightIcon />
            </IconButton>
          </>
        )}

        <Box
          sx={{
            position: 'absolute',
            bottom: 0,
            left: '50%',
            transform: 'translateX(-50%)',
            px: 1.5,
            py: 0.75,
            borderRadius: 999,
            color: '#fff',
            backgroundColor: 'rgba(0,0,0,0.56)',
            display: 'flex',
            gap: 1,
            maxWidth: '80%',
          }}
        >
          <Typography variant="caption" noWrap>{current.name}</Typography>
          {images.length > 1 && (
            <Typography variant="caption" sx={{ color: 'rgba(255,255,255,0.68)' }}>
              {index + 1}/{images.length}
            </Typography>
          )}
        </Box>
      </Box>
    </Dialog>
  )
}

export default ImageLightbox
