import React, { FC, useState } from 'react'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'

import Window from '../widgets/Window'

export const CreateToolWindow: FC<{
  onCreate: (url: string, schema: string) => void,
  onCancel: () => void,
}> = ({
  onCreate,
  onCancel,
}) => {
  const [url, setURL] = useState('')
  const [schema, setSchema] = useState('')
  const [showErrors, setShowErrors] = useState(false)

  const submitValues = () => {
    if(!url || !schema) {
      setShowErrors(true)
      return
    }
    setShowErrors(false)
    onCreate(url, schema)
  }

  return (
    <Window
      title="Create Tool"
      size="md"
      open
      withCancel
      cancelTitle="Cancel"
      onCancel={ onCancel }
      onSubmit={ submitValues }
    >
      <Box
        sx={{
          p: 2,
        }}
      >
        <TextField
          sx={{
            mb: 2,
          }}
          error={ showErrors && !url }
          value={ url }
          onChange={(e) => setURL(e.target.value)}
          fullWidth
          label="API URL"
          placeholder="Enter API URL"
          helperText={ showErrors && !url ? "Please enter a URL" : "URL should be in the format: https://api.example.com/v1/endpoint" }
        />
        <TextField
          error={ showErrors && !schema }
          value={ schema }
          onChange={(e) => setSchema(e.target.value)}
          fullWidth
          multiline
          rows={10}
          label="Enter openAI schema (base64 encoded or escaped JSON/yaml)"
          helperText={ showErrors && !schema ? "Please enter a schema" : "base64 encoded or escaped JSON/yaml" }
        />
      </Box>
    </Window>
  )  
}

export default CreateToolWindow