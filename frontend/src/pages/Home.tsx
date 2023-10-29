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
        <Grid container spacing={2} sx={{mb:6, maxWidth:1200, marginLeft: "auto", marginRight: "auto"}}>
          <Grid item xs={12} sm={4}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}}>
              <img src="/img/sdxl.jpeg" alt="Stable Diffusion XL" style={{width:"300px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Stable Diffusion XL
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => {console.log("here"); document.getElementById("component-45-button")?.click()}}>
              <img src="/img/mistral7b.jpeg" alt="Mistral-7B" style={{width:"300px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Mistral-7B-Instruct
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}}>
              <img src="/img/cowsay.png" alt="Cowsay" style={{width:"300px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Cowsay (this is not AI)
            </Typography>
          </Grid>
        </Grid>
        <iframe
          src="/gradio?__theme=light"
          title="Gradio"
          style={{
            width: '100%',
            height: '700px',
            border: 'none',
            overflow: 'hidden',
          }}
        />
    </Container>
  )
}

export default Dashboard