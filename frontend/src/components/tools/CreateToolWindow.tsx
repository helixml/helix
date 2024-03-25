import React, { FC, useState } from 'react'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

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
      title="New API tool"
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
        <Typography className="interactionMessage"
          sx={{
            mt: 2,
            mb: 2,
          }}
        >
          Let your assistant retrieve information or take actions outside of Helix.          
        </Typography>
        
        <TextField
          sx={{
            mb: 2,
          }}
          error={ showErrors && !url }
          value={ url }
          onChange={(e) => setURL(e.target.value)}
          fullWidth
          label="Endpoint URL"
          placeholder="https://api.example.com/v1/"
          helperText={ showErrors && !url ? "Please enter a URL" : "URL should be in the format: https://api.example.com/v1/endpoint" }
        />
        <TextField
          error={ showErrors && !schema }
          value={ schema }
          onChange={(e) => setSchema(e.target.value)}
          fullWidth
          multiline
          rows={10}
          label="OpenAPI (Swagger) schema"
          helperText={ showErrors && !schema ? "Please enter a schema" : "" }
        />
      </Box>
    </Window>
  )  
}

export default CreateToolWindow