import React, { useState } from 'react'
import { Button, TextField, Typography, Box, Container } from '@mui/material'

const CreateCollection = () => {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [instructions, setInstructions] = useState('')
  const [conversationStarters, setConversationStarters] = useState('')

  const handleCreateCollection = () => {
    // Logic to create a new collection
    console.log({ name, description, instructions, conversationStarters })
    // Redirect to the collection interaction page or show success message
  }

  return (
    <Container maxWidth="sm">
      <Typography variant="h4" gutterBottom>
        Create New Collection
      </Typography>
      <Box component="form" noValidate autoComplete="off">
        <TextField
          label="Name"
          fullWidth
          value={name}
          onChange={(e) => setName(e.target.value)}
          margin="normal"
        />
        <TextField
          label="Description"
          fullWidth
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          margin="normal"
        />
        <TextField
          label="Instructions"
          fullWidth
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
          margin="normal"
        />
        <TextField
          label="Conversation starters"
          fullWidth
          value={conversationStarters}
          onChange={(e) => setConversationStarters(e.target.value)}
          margin="normal"
        />
        <Button variant="contained" color="primary" onClick={handleCreateCollection}>
          Create Collection
        </Button>
      </Box>
    </Container>
  )
}

export default CreateCollection