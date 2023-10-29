import React, { FC, useState, useCallback } from 'react'
import axios from 'axios'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import useAccount from '../hooks/useAccount'

const Dashboard: FC = () => {
  const account = useAccount()
  const [loading, setLoading] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [selectedModule, setSelectedModule] = useState('sdxl')

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
        <Grid container spacing={2} sx={{mb:6, maxWidth:1250, marginLeft: "auto", marginRight: "auto"}}>
          <Grid item xs={12} sm={4} sx={{backgroundColor: selectedModule == "sdxl" ? "lightblue" : "" }}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => setSelectedModule("sdxl")}>
              <img src="/img/sdxl.jpeg" alt="Stable Diffusion XL" style={{width:"250px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Stable Diffusion XL
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4} sx={{backgroundColor: selectedModule == "mistral7b" ? "lightblue" : "" }}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => setSelectedModule("mistral7b")}>
              <img src="/img/mistral7b.jpeg" alt="Mistral-7B" style={{width:"250px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Mistral-7B-Instruct
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4} sx={{backgroundColor: selectedModule == "cowsay" ? "lightblue" : "" }}>
            <Button style={{marginLeft: "auto", marginRight: "auto", display: "block", width:"320px"}} onClick={() => setSelectedModule("cowsay")}>
              <img src="/img/cowsay.png" alt="Cowsay" style={{width:"250px"}} />
            </Button>
            <Typography variant="subtitle1" align="center">
              Cowsay (this is not AI)
            </Typography>
          </Grid>
        </Grid>
        <iframe
          src={"/gradio/" + selectedModule + "?__theme=light&userApiToken=" + account.apiKeys[0]?.key}
          title="Gradio"
          style={{
            width: '100%',
            height: '610px',
            border: 'none',
            overflow: 'hidden',
            marginTop: "-20px"
          }}
        />
    </Container>
  )
}

export default Dashboard