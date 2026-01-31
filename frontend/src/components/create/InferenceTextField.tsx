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

    const handleImageFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0]
      if (file) {
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
            border: '1px solid rgba(255, 255, 255, 0.2)',
            borderRadius: '12px',
            backgroundColor: 'rgba(255, 255, 255, 0.05)',
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
              rows={1}
              style={{
                width: '100%',
                backgroundColor: 'transparent',
                border: 'none',
                color: '#fff',
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
                    border: '2px solid rgba(255, 255, 255, 0.7)',
                    borderRadius: '50%',
                    '&:hover': {
                      borderColor: 'rgba(255, 255, 255, 0.9)',
                      '& svg': { color: 'rgba(255, 255, 255, 0.9)' }
                    }
                  }}
                  onClick={() => {
                    if (imageInputRef.current) imageInputRef.current.click();
                  }}
                >
                  <AttachFileIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                </Box>
              </Tooltip>
              {selectedImageName && (
                <Typography sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '0.8rem', ml: 0.5, maxWidth: '100px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {selectedImageName}
                </Typography>
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
                  border: '1px solid rgba(255, 255, 255, 0.7)',
                  borderRadius: '8px',
                  opacity: loading || disabled ? 0.5 : 1,
                  '&:hover': loading || disabled ? {} : {
                    borderColor: 'rgba(255, 255, 255, 0.9)',
                    '& svg': { color: 'rgba(255, 255, 255, 0.9)' }
                  }
                }}
              >
                {loading ? (
                  <LoadingSpinner />
                ) : (
                  <ArrowUpwardIcon sx={{ color: 'rgba(255, 255, 255, 0.7)', fontSize: '20px' }} />
                )}
              </Box>
            </Tooltip>
          </Box>
        </Box>
      </>
    )
  }

export default InferenceTextField
