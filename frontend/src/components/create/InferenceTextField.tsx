import React, { FC, ReactNode, useRef } from 'react'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import IconButton from '@mui/material/IconButton'

import SendIcon from '@mui/icons-material/Send'

import useLightTheme from '../../hooks/useLightTheme'

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
  const textFieldRef = useRef<HTMLTextAreaElement>()
  
  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    onUpdate(event.target.value)
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      if (event.shiftKey) {
        onUpdate(value + "\n")
      } else {
        onInference()
      }
    }
  }

  return (
    <TextField
      id="textEntry"
      fullWidth
      inputRef={ textFieldRef }
      autoFocus
      label={ `${PROMPT_LABELS[type]} (shift+enter to add a newline)` }
      value={ value }
      disabled={ disabled }
      onChange={handleInputChange}
      name="ai_submit"
      multiline={true}
      onKeyDown={handleKeyDown}
      InputProps={{
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
