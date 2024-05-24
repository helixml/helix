import React, { FC, ReactNode, useRef, useEffect } from 'react'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import IconButton from '@mui/material/IconButton'

import SendIcon from '@mui/icons-material/Send'

import useLightTheme from '../../hooks/useLightTheme'
import useEnterPress from '../../hooks/useEnterPress'
import useIsBigScreen from '../../hooks/useIsBigScreen'

import {
  ISessionType,
} from '../../types'

import { PROMPT_LABELS } from '../../config'

const InferenceTextField: FC<{
  type: ISessionType,
  value: string,
  disabled?: boolean,
  startAdornment?: ReactNode,
  onUpdate: (value: string) => void,
  onInference: () => void,
}> = ({
  type,
  value,
  disabled = false,
  startAdornment,
  onUpdate,
  onInference,
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

  useEffect(() => {
    if (textFieldRef.current && !textFieldRef.current?.matches(':focus')) {
      textFieldRef.current.focus()
    }
  }, [value])

  return (
    <TextField
      id="textEntry"
      fullWidth
      inputRef={ textFieldRef }
      autoFocus
      label={ isBigScreen ? `${PROMPT_LABELS[type]} (shift+enter to add a newline)` : '' }
      value={ value }
      disabled={ disabled }
      onChange={ handleInputChange }
      name="ai_submit"
      multiline={ true }
      onKeyDown={ handleKeyDown }
      InputProps={{
        sx: {
          backgroundColor: lightTheme.backgroundColor,
        },
        startAdornment: startAdornment ? (
          <InputAdornment position="start">
            { startAdornment }
          </InputAdornment>
        ) : null,
        endAdornment: (
          <InputAdornment position="end">
            <IconButton
              id="sendButton"
              aria-label="send"
              disabled={ disabled }
              onClick={ onInference }
              sx={{
                color: lightTheme.icon,
              }}
            >
              <SendIcon />
            </IconButton>
          </InputAdornment>
        ),
      }}
    />
  )
}

export default InferenceTextField
