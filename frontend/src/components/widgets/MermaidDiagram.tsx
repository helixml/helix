import React, { useEffect, useRef, useState, FC } from 'react'
import mermaid from 'mermaid'
import {
  Box,
  CircularProgress,
  Alert,
  useTheme,
  Dialog,
  DialogContent,
  IconButton,
  Slider,
  Typography,
  Stack,
} from '@mui/material'
import { X as CloseIcon, ZoomIn, ZoomOut, RotateCcw } from 'lucide-react'

// Initialize mermaid with dark theme settings
mermaid.initialize({
  startOnLoad: false,
  theme: 'dark',
  themeVariables: {
    primaryColor: '#00d5ff',
    primaryTextColor: '#fff',
    primaryBorderColor: '#00d5ff',
    lineColor: '#00d5ff',
    secondaryColor: '#1e1e1e',
    tertiaryColor: '#2d2d2d',
    background: '#121212',
    mainBkg: '#1e1e1e',
    nodeBorder: '#00d5ff',
    clusterBkg: '#2d2d2d',
    clusterBorder: '#00d5ff',
    titleColor: '#fff',
    edgeLabelBackground: '#1e1e1e',
    nodeTextColor: '#fff',
  },
  fontFamily: 'system-ui, -apple-system, sans-serif',
  securityLevel: 'loose',
})

interface MermaidDiagramProps {
  code: string
  compact?: boolean // For the small preview version
  onClick?: () => void
  enableFullscreen?: boolean // Enable click-to-fullscreen (default: true)
}

// Generate a unique ID for each diagram
let diagramId = 0
const generateId = () => `mermaid-diagram-${++diagramId}`

const MermaidDiagram: FC<MermaidDiagramProps> = ({
  code,
  compact = false,
  onClick,
  enableFullscreen = true,
}) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const [svg, setSvg] = useState<string>('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [zoom, setZoom] = useState(100)
  const theme = useTheme()

  useEffect(() => {
    const renderDiagram = async () => {
      if (!code) {
        setLoading(false)
        return
      }

      setLoading(true)
      setError(null)

      try {
        const id = generateId()
        const { svg: renderedSvg } = await mermaid.render(id, code.trim())
        setSvg(renderedSvg)
      } catch (err) {
        console.error('[MermaidDiagram] Rendering error:', err)
        setError(err instanceof Error ? err.message : 'Failed to render diagram')
      } finally {
        setLoading(false)
      }
    }

    renderDiagram()
  }, [code])

  const handleClick = () => {
    if (onClick) {
      onClick()
    } else if (enableFullscreen && !compact) {
      setModalOpen(true)
    }
  }

  const handleCloseModal = () => {
    setModalOpen(false)
    setZoom(100)
  }

  const handleZoomIn = () => {
    setZoom(prev => Math.min(prev + 25, 500))
  }

  const handleZoomOut = () => {
    setZoom(prev => Math.max(prev - 25, 10))
  }

  const handleResetZoom = () => {
    setZoom(100)
  }

  if (loading) {
    return (
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: compact ? 100 : 200,
          bgcolor: 'rgba(0, 0, 0, 0.1)',
          borderRadius: 2,
        }}
      >
        <CircularProgress size={compact ? 20 : 32} />
      </Box>
    )
  }

  if (error) {
    return (
      <Alert severity="warning" sx={{ fontSize: compact ? '0.75rem' : '0.875rem' }}>
        {compact ? 'Diagram error' : `Diagram rendering error: ${error}`}
      </Alert>
    )
  }

  if (!svg) {
    return null
  }

  const isClickable = onClick || (enableFullscreen && !compact)

  return (
    <>
      <Box
        ref={containerRef}
        onClick={handleClick}
        sx={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          overflow: 'auto',
          bgcolor: compact ? 'transparent' : 'rgba(0, 0, 0, 0.2)',
          borderRadius: 2,
          p: compact ? 1 : 3,
          cursor: isClickable ? 'pointer' : 'default',
          transition: 'all 0.2s ease-in-out',
          position: 'relative',
          ...(isClickable && {
            '&:hover': {
              bgcolor: 'rgba(0, 213, 255, 0.1)',
              boxShadow: '0 0 20px rgba(0, 213, 255, 0.2)',
            },
            '&:hover .zoom-hint': {
              opacity: 1,
            },
          }),
          '& svg': {
            maxWidth: '100%',
            height: 'auto',
            ...(compact && {
              maxHeight: 160,
            }),
          },
        }}
        dangerouslySetInnerHTML={{ __html: svg }}
      />

      {/* Fullscreen Modal */}
      <Dialog
        open={modalOpen}
        onClose={handleCloseModal}
        maxWidth={false}
        fullWidth
        PaperProps={{
          sx: {
            bgcolor: '#121212',
            width: '80vw',
            height: '80vh',
            minWidth: '80vw',
            minHeight: '80vh',
            maxWidth: '95vw',
            maxHeight: '95vh',
          },
        }}
      >
        <DialogContent
          sx={{
            p: 0,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            position: 'relative',
          }}
        >
          {/* Controls bar */}
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              p: 2,
              borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
              bgcolor: 'rgba(0, 0, 0, 0.3)',
            }}
          >
            <Stack direction="row" spacing={2} alignItems="center">
              <IconButton onClick={handleZoomOut} size="small" sx={{ color: '#00d5ff' }}>
                <ZoomOut size={20} />
              </IconButton>
              <Box sx={{ width: 150, display: 'flex', alignItems: 'center', gap: 1 }}>
                <Slider
                  value={zoom}
                  onChange={(_, value) => setZoom(value as number)}
                  min={10}
                  max={500}
                  step={10}
                  sx={{
                    color: '#00d5ff',
                    '& .MuiSlider-thumb': {
                      width: 16,
                      height: 16,
                    },
                  }}
                />
                <Typography variant="caption" sx={{ minWidth: 45, color: 'text.secondary' }}>
                  {zoom}%
                </Typography>
              </Box>
              <IconButton onClick={handleZoomIn} size="small" sx={{ color: '#00d5ff' }}>
                <ZoomIn size={20} />
              </IconButton>
              <IconButton onClick={handleResetZoom} size="small" sx={{ color: 'text.secondary' }}>
                <RotateCcw size={18} />
              </IconButton>
            </Stack>

            <IconButton onClick={handleCloseModal} size="small" sx={{ color: 'text.secondary' }}>
              <CloseIcon size={24} />
            </IconButton>
          </Box>

          {/* Diagram container */}
          <Box
            sx={{
              flex: 1,
              overflow: 'auto',
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              p: 2,
            }}
          >
            <Box
              sx={{
                width: '100%',
                height: '100%',
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                transform: `scale(${zoom / 100})`,
                transformOrigin: 'center center',
                transition: 'transform 0.2s ease-in-out',
                '& svg': {
                  maxWidth: '100%',
                  maxHeight: '100%',
                  width: 'auto',
                  height: 'auto',
                },
              }}
              dangerouslySetInnerHTML={{ __html: svg }}
            />
          </Box>
        </DialogContent>
      </Dialog>
    </>
  )
}

/**
 * Extract Mermaid code blocks from markdown content
 * Returns array of mermaid code strings
 */
export function extractMermaidDiagrams(content: string): string[] {
  if (!content) return []

  const mermaidRegex = /```mermaid\s*([\s\S]*?)```/gi
  const matches: string[] = []
  let match

  while ((match = mermaidRegex.exec(content)) !== null) {
    if (match[1]?.trim()) {
      matches.push(match[1].trim())
    }
  }

  return matches
}

/**
 * Check if content contains any Mermaid diagrams
 */
export function hasMermaidDiagram(content: string): boolean {
  if (!content) return false
  return /```mermaid/i.test(content)
}

export default MermaidDiagram
