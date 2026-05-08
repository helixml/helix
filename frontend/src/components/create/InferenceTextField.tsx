import AttachFileIcon from '@mui/icons-material/AttachFile'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import React, { FC, ReactNode, useEffect, useRef, useState } from 'react'
import useEnterPress from '../../hooks/useEnterPress'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'
import ContextMenuModal from '../widgets/ContextMenuModal'
import LoadingSpinner from '../widgets/LoadingSpinner'
import { useTheme } from '@mui/material/styles'

import {
  ISessionType,
} from '../../types'

import { PROMPT_LABELS } from '../../config'

const InferenceTextField: FC<{
  type: ISessionType,
  value: string,
  disabled?: boolean,
  loading?: boolean,
  // changing this string will re-focus the text field
  // e.g. when the assistant changes
  focus?: string,
  startAdornment?: ReactNode,
  promptLabel?: string,
  onUpdate: (value: string) => void,
  onInference: () => void,
  appId: string,
  attachedImages?: File[],
  onAttachedImagesChange?: (files: File[]) => void,
  filterMap?: Record<string, string>,
  onFilterMapUpdate?: (filterMap: Record<string, string>) => void,
}> = ({
  type,
  value,
  disabled = false,
  loading = false,
  focus = '',
  startAdornment,
  promptLabel,
  onUpdate,
  onInference,
  appId,
  attachedImages = [],
  onAttachedImagesChange,
  filterMap: externalFilterMap,
  onFilterMapUpdate,
}) => {
    const lightTheme = useLightTheme()
    const theme = useTheme()
    const isBigScreen = useIsBigScreen()
    const textFieldRef = useRef<HTMLTextAreaElement>()
    const imageInputRef = useRef<HTMLInputElement>(null)
    const [selectedImage, setSelectedImage] = useState<string | null>(null)
    const [selectedImageName, setSelectedImageName] = useState<string | null>(null)
    const [internalFilterMap, setInternalFilterMap] = useState<Record<string, string>>({})

    const handleKeyDown = useEnterPress({
      value,
      updateHandler: onUpdate,
      triggerHandler: onInference,
    })

    const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
      onUpdate(event.target.value)
    }

    const handleTextareaChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const textarea = e.target
      onUpdate(textarea.value)
      
      // Reset height to auto to get the correct scrollHeight
      textarea.style.height = 'auto'
      
      // Calculate new height based on content
      const lineHeight = parseFloat(getComputedStyle(textarea).lineHeight) || 24
      const maxLines = 5
      const maxHeight = lineHeight * maxLines
      
      // Set height to scrollHeight, but cap at maxHeight
      const newHeight = Math.min(textarea.scrollHeight, maxHeight)
      textarea.style.height = `${newHeight}px`
    }

    const handleInsertText = (text: string) => {
      const filterRegex = /@filter\(\[DOC_NAME:([^\]]+)\]\[DOC_ID:([^\]]+)\]\)/;
      const match = text.match(filterRegex);
      
      if (match) {
        const fullPath = match[1];
        const filename = fullPath.split('/').pop() || fullPath;
        const displayText = `@${filename}`;
        
        const newFilterMap = {
          ...internalFilterMap,
          [displayText]: text
        };
        setInternalFilterMap(newFilterMap);
        
        if (onFilterMapUpdate) {
          onFilterMapUpdate(newFilterMap);
        }
        
        // Find the last @ in the text and replace it with the display text
        const lastAtIndex = value.lastIndexOf('@');
        if (lastAtIndex !== -1) {
          // Replace from @ to the end with the display text
          const newValue = value.substring(0, lastAtIndex) + displayText;
          onUpdate(newValue);
        } else {
          // Fallback: just append if @ not found
          onUpdate(value + displayText);
        }
      } else {
        onUpdate(value + text);
      }
    }

    const attachImageFile = (file: File) => {
      const reader = new FileReader()
      reader.onloadend = () => {
        setSelectedImage(reader.result as string)
        setSelectedImageName(file.name)
        if (onAttachedImagesChange) {
          onAttachedImagesChange([...attachedImages, file])
        }
      }
      reader.readAsDataURL(file)
    }

    const handleImageFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0]
      if (file) {
        attachImageFile(file)
      }
    }

    // Intercept Cmd+V / Ctrl+V image pastes from the clipboard (e.g. screenshot
    // copies on macOS, "Copy image" from another browser tab) and attach them
    // the same way the file picker does. We only swallow the paste when the
    // clipboard actually contains an image — text pastes still fall through.
    const handlePaste = (event: React.ClipboardEvent<HTMLTextAreaElement>) => {
      if (!onAttachedImagesChange) return
      const items = event.clipboardData?.items
      if (!items || items.length === 0) return

      const imageFiles: File[] = []
      for (let i = 0; i < items.length; i++) {
        const item = items[i]
        if (item.kind !== 'file') continue
        if (!item.type.startsWith('image/')) continue
        const file = item.getAsFile()
        if (file) imageFiles.push(file)
      }

      if (imageFiles.length === 0) return

      event.preventDefault()
      const next = [...attachedImages, ...imageFiles]
      onAttachedImagesChange(next)

      // Update the preview to show the most recently pasted image so the user
      // gets immediate feedback that the paste landed.
      const last = imageFiles[imageFiles.length - 1]
      const reader = new FileReader()
      reader.onloadend = () => {
        setSelectedImage(reader.result as string)
        setSelectedImageName(last.name || `pasted-image-${Date.now()}.png`)
      }
      reader.readAsDataURL(last)
    }

    const usePromptLabel = promptLabel || PROMPT_LABELS[type]

    useEffect(() => {
      if (textFieldRef.current && !textFieldRef.current?.matches(':focus')) {
        textFieldRef.current.focus()
      }
    }, [
      value,
      focus,
    ])

    useEffect(() => {
      if (!value && textFieldRef.current) {
        textFieldRef.current.style.height = 'auto'
      }
    }, [value])

    return (
      <>
        <ContextMenuModal
          appId={appId}
          textAreaRef={textFieldRef}
          onInsertText={handleInsertText}
        />
        <Box
          sx={{
            width: '95%',
            margin: '0 auto',
            border: `1px solid ${lightTheme.isLight ? 'rgba(0, 0, 0, 0.28)' : 'rgba(255, 255, 255, 0.2)'}`,
            borderRadius: '12px',
            backgroundColor: lightTheme.isLight ? '#fff' : 'rgba(255, 255, 255, 0.05)',
            p: 2,
            display: 'flex',
            flexDirection: 'column',
            gap: 1,
            bgcolor: theme.palette.background.default,
          }}
        >
          {/* Top row: textarea */}
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
            <textarea
              ref={textFieldRef as React.RefObject<HTMLTextAreaElement>}
              value={value}
              onChange={handleTextareaChange}
              onKeyDown={handleKeyDown as any}
              onPaste={handlePaste}
              rows={1}
              style={{
                width: '100%',
                backgroundColor: 'transparent',
                border: 'none',
                color: lightTheme.textColor,
                opacity: 0.7,
                resize: 'none',
                outline: 'none',
                fontFamily: 'inherit',
                fontSize: 'inherit',
                lineHeight: '1.5',
                overflowY: 'auto',
              }}
              placeholder={usePromptLabel}
              disabled={disabled}
            />
          </Box>
          {/* Bottom row: attachment icon, image name, send button */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, justifyContent: 'space-between', flexWrap: 'wrap' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Tooltip title="Attach Image" placement="top">
                <Box
                  sx={{
                    width: 32,
                    height: 32,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    cursor: 'pointer',
                    border: `2px solid ${lightTheme.isLight ? 'rgba(0, 0, 0, 0.5)' : 'rgba(255, 255, 255, 0.7)'}`,
                    borderRadius: '50%',
                    '&:hover': {
                      borderColor: lightTheme.textColor,
                      '& svg': { color: lightTheme.textColor }
                    }
                  }}
                  onClick={() => {
                    if (imageInputRef.current) imageInputRef.current.click();
                  }}
                >
                  <AttachFileIcon sx={{ color: lightTheme.textColorFaded, fontSize: '20px' }} />
                </Box>
              </Tooltip>
              {selectedImage && (
                <Box
                  sx={{
                    position: 'relative',
                    ml: 0.5,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.75,
                  }}
                >
                  <Tooltip title={selectedImageName || 'Attached image'} placement="top">
                    <Box
                      component="img"
                      src={selectedImage}
                      alt={selectedImageName || 'Attached image'}
                      sx={{
                        width: 36,
                        height: 36,
                        objectFit: 'cover',
                        borderRadius: '6px',
                        border: `1px solid ${lightTheme.isLight ? 'rgba(0, 0, 0, 0.15)' : 'rgba(255, 255, 255, 0.2)'}`,
                      }}
                    />
                  </Tooltip>
                  <Typography sx={{ color: lightTheme.textColorFaded, fontSize: '0.8rem', maxWidth: '120px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {selectedImageName}
                  </Typography>
                  <Box
                    role="button"
                    aria-label="Remove attached image"
                    onClick={() => {
                      setSelectedImage(null)
                      setSelectedImageName(null)
                      if (imageInputRef.current) imageInputRef.current.value = ''
                      if (onAttachedImagesChange) onAttachedImagesChange([])
                    }}
                    sx={{
                      width: 18,
                      height: 18,
                      borderRadius: '50%',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      cursor: 'pointer',
                      fontSize: '12px',
                      color: lightTheme.textColorFaded,
                      border: `1px solid ${lightTheme.isLight ? 'rgba(0, 0, 0, 0.2)' : 'rgba(255, 255, 255, 0.3)'}`,
                      '&:hover': {
                        color: lightTheme.textColor,
                        borderColor: lightTheme.isLight ? 'rgba(0, 0, 0, 0.4)' : 'rgba(255, 255, 255, 0.6)',
                      },
                    }}
                  >
                    ×
                  </Box>
                </Box>
              )}
              <input
                type="file"
                ref={imageInputRef}
                style={{ display: 'none' }}
                accept="image/*"
                onChange={handleImageFileChange}
              />
            </Box>
            <Tooltip title="Send Prompt" placement="top">
              <Box
                onClick={() => onInference()}
                sx={{
                  width: 32,
                  height: 32,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: loading || disabled ? 'default' : 'pointer',
                  border: `1px solid ${lightTheme.isLight ? 'rgba(0, 0, 0, 0.5)' : 'rgba(255, 255, 255, 0.7)'}`,
                  borderRadius: '8px',
                  opacity: loading || disabled ? 0.5 : 1,
                  '&:hover': loading || disabled ? {} : {
                    borderColor: lightTheme.textColor,
                    '& svg': { color: lightTheme.textColor }
                  }
                }}
              >
                {loading ? (
                  <LoadingSpinner />
                ) : (
                  <ArrowUpwardIcon sx={{ color: lightTheme.textColorFaded, fontSize: '20px' }} />
                )}
              </Box>
            </Tooltip>
          </Box>
        </Box>
      </>
    )
  }

export default InferenceTextField
