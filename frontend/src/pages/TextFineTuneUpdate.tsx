import React, { FC, useState } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Button from '@mui/material/Button'
import Card from '@mui/material/Card'
import CardMedia from '@mui/material/CardMedia'
import IconButton from '@mui/material/IconButton'
import ZoomOutMapIcon from '@mui/icons-material/ZoomOutMap'
import TextField from '@mui/material/TextField'
import RefreshIcon from '@mui/icons-material/Refresh'

const TextFineTuneUpdate: FC = () => {
  return (
    <Container
      maxWidth="xl"
      sx={{
        mt: 12,
        height: 'calc(100% - 100px)',
      }}
    >  
      <Box sx={{ display: 'flex', gap: '0.5rem', mt: 2  }}>
        <Button
          variant="contained"
          color="primary"
          size="small"
          sx={{
              textTransform: 'none',
              bgcolor: '#F0BEB0',
              color: 'black',
              fontWeight: 800,
              padding: '2px 8px',
              minWidth: 'auto',
              height: 'auto'
          }}
        >
          AI
        </Button>
        <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>
          Helix System
        </Typography>
        <Typography variant="subtitle1" sx={{ fontWeight: 800, marginLeft: 'auto', display: 'flex', alignItems: 'center' }}>
          <RefreshIcon sx={{ color: '#FCDB05', mr: 0 }} /> 
          <span style={{ color: '#FCDB05' }}>Restart</span>
        </Typography>
      </Box>
      
      <Box sx={{ mt: 2 }}>
        <Typography className="interactionMessage" gutterBottom>
          You have completed a fine tuning session on these documents.
        </Typography>
        <Typography className="interactionMessageOne" gutterBottom >
          Now chat to your model, add some more documents and re-train or you can "Clone" from this point
        </Typography>  
      </Box>
      <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
        <Button
          variant="contained"
          sx={{
            bgcolor: '#FCDB05',
            color: 'black',
            textTransform: 'none',
          }}
        >
          Add more images
        </Button>
        <Button
          variant="contained"
          sx={{
            bgcolor: '#00D5FF',
            color: 'black',
            textTransform: 'none',
          }}
        >
          View questions
        </Button>
        <Button
          variant="contained"
          sx={{
            bgcolor: '#F0BEB0',
            color: 'black',
            textTransform: 'none',
          }}
        >
          Clone
        </Button>
      </Box>
      <Box sx={{ mt: 58 }}> 
        <TextField
          fullWidth
          placeholder="Chat with Helix. (Shift + Enter to add new lines)"
          multiline
          InputLabelProps={{
            shrink: true, // This ensures the label does not overlap with the placeholder text
          }}
          InputProps={{
            style: {
              backgroundColor: 'transparent',
            },
          }}
        />
      </Box>    
    </Container>
  )
}

export default TextFineTuneUpdate