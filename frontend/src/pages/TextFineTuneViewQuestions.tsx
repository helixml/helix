import React, { FC, useState } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import BackgroundImageWrapper from '../components/widgets/BackgroundImageWrapper'

const TextFineTuneViewQuestions : FC = () => {
  return (
    <BackgroundImageWrapper>
      <Container
        maxWidth="xl"
        sx={{
          mt: 12,
          height: 'calc(100% - 100px)',
        }}
        >
        <Typography variant="h6" component="h2">
          Question
        </Typography>
        <TextField
          label="http://mylinkhasbeenpastedin.com"
          variant="outlined"
          fullWidth
          margin="normal"
        />
        <Typography variant="h6" component="h2" sx={{ mt: 4 }}>
          Answer
        </Typography>
        <TextField
          variant="outlined"
          fullWidth
          margin="normal"
        />
        <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 48 }}>
          <Button
            variant="outlined"
            sx={{
              mr: 1,
              bgcolor: 'white',
              color: 'black',
              textTransform: 'none',
              '&:hover': {
                bgcolor: 'white', // button color on hover
              },
            }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            sx={{
              bgcolor: '#00D5FF',
              color: 'black',
              textTransform: 'none',
            }}
          >
            Add Question & Answer
          </Button>
        </Box>
      </Container>
    </BackgroundImageWrapper>
  )
}

export default TextFineTuneViewQuestions