import React, { FC, useState, useCallback } from 'react'
import axios from 'axios'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import InputLabel from '@mui/material/InputLabel'
import FormControl from '@mui/material/FormControl'

const Dashboard: FC = () => {
  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [chatHistory, setChatHistory] = useState<Array<{user: string, message: string}>>([])
  const [selectedMode, setSelectedMode] = useState('Create')
  const [selectedCreateType, setSelectedCreateType] = useState('Text')
  const [selectedFineTuneType, setSelectedFineTuneType] = useState('Text')

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const runJob = useCallback(async () => {
    setLoading(true)
    try {
      const statusResult = await axios.post('/api/v1/jobs', {
        module: 'cowsay:v0.0.1',
        inputs: {
          Message: inputValue,
        }
      })
      setChatHistory([...chatHistory, {user: 'User', message: inputValue}, {user: 'ChatGPT', message: statusResult.data}])
    } catch(e: any) {
      alert(e.message)
    }
    setInputValue('')
    setLoading(false)
  }, [
    inputValue,
    chatHistory
  ])

  return (
    <Container sx={{ mt: 4, mb: 4, display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', overflowX: 'hidden' }}>
      <Grid container spacing={3} direction="row" justifyContent="flex-start">
        <Grid item xs={2} md={2}>
        </Grid>
        <Grid item xs={4} md={4}>
          <Button variant={selectedMode === 'Create' ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setSelectedMode('Create')}>
            Create
            <FormControl sx={{ minWidth: 120, marginLeft: 2 }}>
              <Select variant="standard"
                labelId="create-type-select-label"
                id="create-type-select"
                value={selectedCreateType}
                onChange={(event) => setSelectedCreateType(event.target.value)}
              >
                <MenuItem value="Text">Text</MenuItem>
                <MenuItem value="Images">Images</MenuItem>
              </Select>
            </FormControl>
          </Button>
        </Grid>
        <Grid item xs={4} md={4}>
          <Button variant={selectedMode === 'Finetune' ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setSelectedMode('Finetune')}>
            Finetune
            <FormControl sx={{minWidth: 120, marginLeft: 2}}>
              <Select variant="standard"
                labelId="fine-tune-type-select-label"
                id="fine-tune-type-select"
                value={selectedFineTuneType}
                onChange={(event) => setSelectedFineTuneType(event.target.value)}
              >
                <MenuItem value="Text">Text</MenuItem>
                <MenuItem value="Images">Images</MenuItem>
              </Select>
            </FormControl>
          </Button>
        </Grid>
        <Grid item xs={2} md={2}>
        </Grid>
        <Grid item xs={12} md={12}>
          {chatHistory.map((chat, index) => (
            <Typography key={index}><strong>{chat.user}:</strong> {chat.message}</Typography>
          ))}
        </Grid>
      </Grid>
      <Grid container item xs={12} md={8} direction="row" justifyContent="space-between" alignItems="center" sx={{ mt: 'auto', position: 'absolute', bottom: '5em', maxWidth: '800px' }}>
        <Grid item xs={12} md={11}>
            <TextField
              fullWidth
              label={selectedMode === 'Create' && selectedCreateType === 'Text' ? 'Start a chat with a base Mistral-7B model' : selectedMode === 'Create' && selectedCreateType === 'Images' ? 'Describe an image to create it with a base SDXL model' : selectedMode === 'Finetune' && selectedFineTuneType === 'Text' ? 'Enter question-answer pairs to fine tune a language model' : 'Upload images and label them to fine tune an image model'}
              value={inputValue}
              disabled={loading}
              onChange={handleInputChange}
              name="ai_submit"
              multiline={true}
            />
          </Grid>
        <Grid item xs={12} md={1}>
          <Button
            variant='contained'
            disabled={loading}
            onClick={ runJob }
            sx={{ ml: 2 }}
          >
            Send
          </Button>
        </Grid>
      </Grid>
    </Container>
  )
}

export default Dashboard