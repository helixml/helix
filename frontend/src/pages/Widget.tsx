import React, { useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Embed from '../components/widgets/Embed'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import IconButton from '@mui/material/IconButton'
import SearchIcon from '@mui/icons-material/Search'

// Assuming this is the structure of your answer type based on your initial code
type AnswerType = {
  id: string
  object: string
  created: number
  model: string
  usage: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
  choices: {
    message: {
      role: string
      content: string
    }
    finish_reason: string
    index: number
  }[]
} | null

const Widget = () => {
  const [open, setOpen] = useState(false)
  const [answer, setAnswer] = useState<AnswerType>(null)

  const handleClose = () => {
    setOpen(false)
    setAnswer(null) // Optionally reset the answer when closing the modal
  }

  const handleSubmitQuestion = async (question: string): Promise<AnswerType> => {
    const mockAnswer: AnswerType = {
      id: "mockId",
      object: "chat.completion",
      created: Date.now(),
      model: "gpt-3.5-turbo",
      usage: {
        prompt_tokens: 10,
        completion_tokens: 20,
        total_tokens: 30
      },
      choices: [{
        message: {
          role: "assistant",
          content: "This is a mock answer to demonstrate how you might structure the response from your API."
        },
        finish_reason: "length",
        index: 0
      }]
    }
    setAnswer(mockAnswer)
    return mockAnswer // Return the mockAnswer as a resolved value
  }

  // Example verified sources data
  const verifiedSources = [
    {
      id: "verified-source-1",
      name: "OpenAI",
      url: "https://openai.com",
    },
    {
      id: "verified-source-2",
      name: "Wikipedia",
      url: "https://wikipedia.org",
    },
  ]

  return (
    <Box
      sx={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100vh', // Adjust the height as needed
        width: '100%',
      }}
    >
      <TextField
        variant="outlined"
        placeholder="Ask a question..."
        onClick={() => setOpen(true)}
        fullWidth
        sx={{ maxWidth: 400 }}
        InputProps={{
          endAdornment: (
            <InputAdornment position="end">
              <IconButton>
                <SearchIcon />
              </IconButton>
            </InputAdornment>
          ),
        }}
      />
      <Embed
        verifiedSources={verifiedSources}
        onSubmitQuestion={handleSubmitQuestion}
        open={open}
        handleClose={handleClose}
        answer={answer}
      />
    </Box>
  )
}

export default Widget