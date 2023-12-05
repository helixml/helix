import React, { FC, useState } from 'react'
import TextField from '@mui/material/TextField'

// this is it's own component because it turns out that rendering the images from
// the seriliazed file uploads was re-rendering slowly
export const ImageFineTuneLabel: FC<{
  value: string,
  filename: string,
  error?: boolean,
  onChange: {
    (value: string): void
  },
}> = ({
  value,
  filename,
  error = false,
  onChange,
}) => {
  const [label, setLabel] = useState(value)

  return (
    <TextField
      fullWidth
      hiddenLabel
      value={ label }
      error={ error }
      onChange={ (event) => {
        setLabel(event.target.value)
      }}
      onBlur={ () => {
        onChange(label)
      }}
      helperText={ `Enter a label for ${filename}` }
    />
  )   
}

export default ImageFineTuneLabel