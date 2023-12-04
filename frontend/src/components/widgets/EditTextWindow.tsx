import React, { FC, useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Window from './Window'

interface EditTextWindowProps {
  value?: string,
  title?: string,
  submitTitle?: string,
  label?: string,
  onCancel: {
    (): void,
  },
  onSubmit: {
    (value: string): void,
  }
}

const EditTextWindow: FC<React.PropsWithChildren<EditTextWindowProps>> = ({
  value = '',
  title = '',
  submitTitle = 'Save',
  label = 'edit the value',
  onCancel,
  onSubmit,
}) => {
  const [currentValue, setCurrentValue] = useState(value)

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      onSubmit(currentValue)
      event.preventDefault()
    }
  }, [
    currentValue,
    onSubmit,
  ])

  return (
    <Window
      open
      size="sm"
      title={ title }
      withCancel
      submitTitle={ submitTitle }
      onCancel={ onCancel }
      onSubmit={ () => {
        onSubmit(currentValue)
      } }
    >
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'flex-start',
          width: '100%',
        }}
      >
        <Box
          sx={{
            width: '100%',
            padding:1,
          }}
        >
          <TextField
            autoFocus
            label={ label }
            value={ currentValue }
            fullWidth
            onChange={ (e) => setCurrentValue(e.target.value) }
            onKeyDown={ handleKeyDown }
          />
        </Box>
      </Box>
    </Window>
  )
}

export default EditTextWindow