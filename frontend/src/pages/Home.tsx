import React, { FC, useState, useCallback } from 'react'
import axios from 'axios'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

const Dashboard: FC = () => {
  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [chatHistory, setChatHistory] = useState<Array<{user: string, message: string}>>([
    {user: 'User', message: 'Hello!'},
    {user: 'ChatGPT', message: 'Hi there! How can I assist you today?'}
  ])
  const [selectedMode, setSelectedMode] = useState('Create')

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
    <Container maxWidth={ 'xl' } sx={{ mt: 4, mb: 4 }}>
      <Grid container spacing={3} direction="row" justifyContent="flex-start" style={{ height: '78vh' }}>
        <Grid item xs={12} md={12}>
          <Button variant={selectedMode === 'Create' ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35, mr: 2 }} onClick={() => setSelectedMode('Create')}>
            Create
          </Button>
          <Button variant={selectedMode === 'Fine-tune' ? "contained" : "outlined"} color="primary" sx={{ borderRadius: 35 }} onClick={() => setSelectedMode('Fine-tune')}>
            Fine-tune
          </Button>
        </Grid>
        <Grid item xs={12} md={12}>
          {chatHistory.map((chat, index) => (
            <Typography key={index}><strong>{chat.user}:</strong> {chat.message}</Typography>
          ))}
        </Grid>
        <Box sx={{ flexGrow: 1 }} />
      </Grid>
      <Grid container item xs={12} md={12} direction="row" justifyContent="space-between" alignItems="center">
        <Grid item xs={12} md={8}>
          <TextField
            fullWidth
            label="Type something here"
            value={inputValue}
            disabled={loading}
            onChange={handleInputChange}
          />
        </Grid>
        <Grid item xs={12} md={4}>
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