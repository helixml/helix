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
      <Grid container spacing={3}>
        <Grid item xs={12} md={12}>
          <Typography>Run cowsay...</Typography>
        </Grid>
        <Grid item xs={12} md={12}>
          <TextField
            fullWidth
            label="Type something here"
            value={inputValue}
            disabled={loading}
            onChange={handleInputChange}
          />
        </Grid>
        <Grid item xs={12} md={12}>
          <Button
            variant='contained'
            disabled={loading}
            onClick={ runJob }
          >
            Run
          </Button>
        </Grid>
      </Grid>
    </Container>
  )
}

export default Dashboard