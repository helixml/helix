import React, { FC, useState } from 'react'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import Window from '../widgets/Window'

export const CreateToolWindow: FC<{
  onCreate: (name: string, description: string, script: string) => void,
  onCancel: () => void,
}> = ({
  onCreate,
  onCancel,
}) => {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [script, setScript] = useState('')
  const [showErrors, setShowErrors] = useState(false)


  const submitValues = () => {
    if(!name || !description || !script) {
      setShowErrors(true)
      return
    }
    setShowErrors(false)
    onCreate(name, description, script)
  }

  return (
    <Window
      title="Add GPTScript"
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
          error={ showErrors && !name }
          value={ name }
          onChange={(e) => setName(e.target.value)}
          fullWidth
          label="Name of your GPTScript"
          placeholder="echo.gpt"
          helperText={ showErrors && !name ? "Please enter script name" : "Ensure it's easy to understand as you may end up having a lot of scripts" }
        />
        <TextField
          sx={{
            mb: 2,
          }}
          error={ showErrors && !description }
          value={ description }
          onChange={(e) => setDescription(e.target.value)}
          fullWidth
          label="Description"
          placeholder=""
          helperText={ showErrors && !description ? "Please provide description" : "Explain the purpose of this script, i.e. 'echo tool, use it when you need to echo back the input'" }
        />
        <TextField
          error={ showErrors && !script }
          value={ script }
          onChange={(e) => setScript(e.target.value)}
          fullWidth
          multiline
          rows={10}
          label="GPTScript"
          helperText={ showErrors && !script ? "Please enter your GPTScript" : "" }
        />
      </Box>
      
      <a target="_blank" href="https://github.com/gptscript-ai/gptscript">
        <Typography
            variant="body2"
            sx={{
              color: '#B1B1D1',
              flexGrow: 1,
              display: 'flex',
              justifyContent: 'flex-start',
              textAlign: 'left',
              cursor: 'pointer',
              pl: 2,
            }}            
          >
            GPTScript documentation
          </Typography>
      </a>
      

    </Window>
  )  
}

export default CreateToolWindow