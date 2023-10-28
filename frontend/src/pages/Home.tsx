import React, { FC, useState, useCallback } from 'react'
import axios from 'axios'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'

const Dashboard: FC = () => {
  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')

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
      console.log('--------------------------------------------')
      console.dir(statusResult.data)
    } catch(e: any) {
      alert(e.message)
    }
    setLoading(false)
  }, [
    inputValue
  ])

  return (
    <Container maxWidth={ 'xl' } sx={{ mt: 4, mb: 4 }}>
      <iframe
        src="/gradio"
        title="Gradio"
        style={{
          width: '100%',
          height: '600px',
          border: 'none',
          overflow: 'hidden',
        }}
      />
    </Container>
  )
}

export default Dashboard