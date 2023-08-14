import React, { FC } from 'react'
import { navigate } from 'hookrouter'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import useThemeConfig from '../hooks/useThemeConfig'
import Card from '@mui/material/Card'
import CardActions from '@mui/material/CardActions'
import CardContent from '@mui/material/CardContent'

const Dashboard: FC = () => {
  return (
    <Container maxWidth={ 'xl' } sx={{ mt: 4, mb: 4 }}>
      <Grid container spacing={3}>
        <Grid item xs={12} md={12}>
          <Typography>Welcome to the website...</Typography>
        </Grid>
      </Grid>
    </Container>
  )
}

export default Dashboard