import React, { FC, useState, useEffect } from 'react'
import TextField from '@mui/material/TextField'

import useLightTheme from '../../hooks/useLightTheme'

// in it's own component to prevent re-rendering the entire page every time a keypress is made
export const LabelImagesTextField: FC<{
  value: string,
  error?: boolean,
  onChange: (value: string) => void,
}> = ({
  value,
  error,
  onChange,
}) => {
  const lightTheme = useLightTheme()
  const [label, setLabel] = useState('')

  useEffect(() => {
    if(value) {
      setLabel(value)
    }
  }, [
    value,
  ])

  return (
    <TextField
      placeholder=""
      multiline
      value={ label || '' }
      error={ error }
      rows={ 2 }
      InputProps={{
        sx: {
          color: lightTheme.textColorFaded,
        }
      }}
      sx={{
        width: '100%',
        mt: 2,
        borderColor: lightTheme.textColorFaded,
        backgroundColor: '#000',
      }}
      onChange={(e) => setLabel(e.target.value)}
      onBlur={() => onChange(label)}
    />
  )
}

export default LabelImagesTextField