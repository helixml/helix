import React, { FC, useState, useCallback, useRef } from 'react'
import { Box, Typography, CircularProgress, alpha } from '@mui/material'
import { Upload } from 'lucide-react'
import useSnackbar from '../../hooks/useSnackbar'
import { useCreateOrUpdateRepositoryFile } from '../../services/gitRepositoryService'

interface ProjectDropZoneProps {
  children: React.ReactNode
  repositoryId: string | undefined
  branch?: string
  disabled?: boolean
  onUploadComplete?: () => void
}

/**
 * ProjectDropZone - Wraps content to enable drag-and-drop file upload to a project's repository
 *
 * When files are dragged over the wrapped content, shows an overlay.
 * When dropped, uploads files to the specified repository.
 *
 * Note: This should NOT be active when a desktop viewer is visible,
 * as the desktop has its own drop zone for uploading to the container.
 */
const ProjectDropZone: FC<ProjectDropZoneProps> = ({
  children,
  repositoryId,
  branch = 'main',
  disabled = false,
  onUploadComplete,
}) => {
  const snackbar = useSnackbar()
  const [isDragging, setIsDragging] = useState(false)
  const [isUploading, setIsUploading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState({ current: 0, total: 0 })
  const dragCounter = useRef(0)

  const createOrUpdateFile = useCreateOrUpdateRepositoryFile()

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()

    if (disabled || !repositoryId) return

    // Check if this is a file drag (not text or other content)
    if (!e.dataTransfer.types.includes('Files')) return

    dragCounter.current++
    if (dragCounter.current === 1) {
      setIsDragging(true)
    }
  }, [disabled, repositoryId])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()

    dragCounter.current--
    if (dragCounter.current === 0) {
      setIsDragging(false)
    }
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const handleDrop = useCallback(async (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()

    dragCounter.current = 0
    setIsDragging(false)

    if (disabled || !repositoryId) return

    const files = Array.from(e.dataTransfer.files)
    if (files.length === 0) return

    setIsUploading(true)
    setUploadProgress({ current: 0, total: files.length })

    let successCount = 0
    let errorCount = 0

    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      setUploadProgress({ current: i + 1, total: files.length })

      try {
        // Read file content as base64
        const content = await readFileAsBase64(file)

        await createOrUpdateFile.mutateAsync({
          repositoryId,
          request: {
            path: file.name,
            content,
            branch,
            message: `Upload ${file.name}`,
            is_base64: true,
          },
        })
        successCount++
      } catch (error) {
        console.error(`Failed to upload ${file.name}:`, error)
        errorCount++
      }
    }

    setIsUploading(false)

    if (successCount > 0 && errorCount === 0) {
      snackbar.success(`Uploaded ${successCount} file${successCount > 1 ? 's' : ''} successfully`)
    } else if (successCount > 0 && errorCount > 0) {
      snackbar.warning(`Uploaded ${successCount} file${successCount > 1 ? 's' : ''}, ${errorCount} failed`)
    } else if (errorCount > 0) {
      snackbar.error(`Failed to upload ${errorCount} file${errorCount > 1 ? 's' : ''}`)
    }

    onUploadComplete?.()
  }, [disabled, repositoryId, branch, createOrUpdateFile, snackbar, onUploadComplete])

  // Don't render drop zone functionality if no repository
  if (!repositoryId || disabled) {
    return <>{children}</>
  }

  return (
    <Box
      sx={{ position: 'relative', height: '100%', width: '100%' }}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      {children}

      {/* Drag overlay */}
      {isDragging && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: (theme) => alpha(theme.palette.primary.main, 0.1),
            border: (theme) => `3px dashed ${theme.palette.primary.main}`,
            borderRadius: 2,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
            pointerEvents: 'none',
          }}
        >
          <Upload size={48} color="inherit" />
          <Typography variant="h6" sx={{ mt: 2, color: 'primary.main' }}>
            Drop files to upload
          </Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary', mt: 1 }}>
            Files will be added to the project repository
          </Typography>
        </Box>
      )}

      {/* Upload progress overlay */}
      {isUploading && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: (theme) => alpha(theme.palette.background.paper, 0.9),
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1001,
          }}
        >
          <CircularProgress size={48} />
          <Typography variant="h6" sx={{ mt: 2 }}>
            Uploading files...
          </Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary', mt: 1 }}>
            {uploadProgress.current} of {uploadProgress.total}
          </Typography>
        </Box>
      )}
    </Box>
  )
}

/**
 * Read a file as base64 encoded string
 */
function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      // Remove the data URL prefix (e.g., "data:image/png;base64,")
      const base64 = result.split(',')[1] || result
      resolve(base64)
    }
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

export default ProjectDropZone
