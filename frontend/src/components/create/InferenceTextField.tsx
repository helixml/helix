import SendIcon from '@mui/icons-material/Send'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import TextField from '@mui/material/TextField'
import React, { FC, ReactNode, useEffect, useRef } from 'react'
import useEnterPress from '../../hooks/useEnterPress'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'
import ContextMenuModal from '../widgets/ContextMenuModal'
import LoadingSpinner from '../widgets/LoadingSpinner'

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
}) => {
    const lightTheme = useLightTheme()
    const isBigScreen = useIsBigScreen()
    const textFieldRef = useRef<HTMLTextAreaElement>()
    const handleKeyDown = useEnterPress({
      value,
      updateHandler: onUpdate,
      triggerHandler: onInference,
    })

    const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
      onUpdate(event.target.value)
    }

    const handleInsertText = (text: string) => {
      onUpdate(value + text)
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

    return (
      <>
        <ContextMenuModal
          appId={appId}
          textAreaRef={textFieldRef}
          onInsertText={handleInsertText}
        />
        <TextField
          id="textEntry"
          fullWidth
          inputRef={textFieldRef}
          autoFocus
          label={isBigScreen ? `${usePromptLabel} (shift+enter to add a newline)` : ''}
          value={value}
          disabled={disabled}
          onChange={handleInputChange}
          name="ai_submit"
          multiline={true}
          onKeyDown={handleKeyDown}
          InputProps={{
            sx: {
              backgroundColor: lightTheme.backgroundColor,
            },
            startAdornment: startAdornment ? (
              <InputAdornment position="start">
                {startAdornment}
              </InputAdornment>
            ) : null,
            endAdornment: (
              <InputAdornment position="end">
                {
                  loading ? (
                    <LoadingSpinner />
                  ) : (
                    <IconButton
                      id="sendButton"
                      aria-label="send"
                      disabled={disabled}
                      onClick={onInference}
                      sx={{
                        color: lightTheme.icon,
                      }}
                    >
                      <SendIcon />
                    </IconButton>
                  )
                }
              </InputAdornment>
            ),
          }}
        />
      </>
    )
  }

export default InferenceTextField
