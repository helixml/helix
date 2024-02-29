import React, { FC, useState } from 'react'
import Dialog from '@mui/material/Dialog'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import { DialogTitle } from '@mui/material'
import { DialogActions } from '@mui/material'
import { InputAdornment } from '@mui/material'
import IconButton from '@mui/material/IconButton'
import SendIcon from '@mui/icons-material/Send'
import RecycleIcon from '@mui/icons-material/Refresh'

import useThemeConfig from '../hooks/useThemeConfig'
import { BoldSectionTitle } from '../components/widgets/GeneralText'

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

const AskQuestionModal: FC = () => {
  const themeConfig = useThemeConfig()
  const [open, setOpen] = useState(false)
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState<AnswerType>(null)
  const [showAnswer, setShowAnswer] = useState(false)
  const [verifiedSources, setVerifiedSources] = useState([
    {
      id: "verified-source-1",
      name: "OpenAI",
      url: "https://openai.com",
    },
    {
      id: "verified-source-2",
      name: "OpenAI",
      url: "https://openai.com",
    },
  ])

  const [selectedAnswer, setSelectedAnswer] = useState<'yes' | 'no' | null>(null)
  
  const handleOpen = () => setOpen(true)
  const handleClose = () => {
    setOpen(false)
    setAnswer(null) // Reset the answer when closing the modal
    setShowAnswer(false)
  }
  const handleQuestionChange = (event: any) => setQuestion(event.target.value)
  const handleSubmitQuestion = () => {
    // Mock request in the format of the OpenAI Chat API
    const request = {
      model: "gpt-3.5-turbo",
      messages: [
        {
          role: "user",
          content: question
        }
      ],
      max_tokens: 50,
      temperature: 0.5,
      top_p: 1,
      frequency_penalty: 0,
      presence_penalty: 0,
    }
    // Mock response in the format of the OpenAI Chat API
    const response = {
      data: {
        answer: {
          id: "chatcmpl-0HxP5X77z6w3Eub6myqg3VEU7HZ5F",
          object: "chat.completion",
          created: 1623794215,
          model: "gpt-3.5-turbo",
          usage: {
            prompt_tokens: 56,
            completion_tokens: 31,
            total_tokens: 87
          },
          choices: [
            {
              message: {
                role: "assistant",
                content: "In today's rapidly evolving tech landscape, the integration of Large Language Models (LLMs) is revolutionizing how we interact with digital systems. These models, trained on vast datasets, possess the remarkable ability to understand and generate human-like text, enabling a wide range of applications from automated customer service to sophisticated content creation. By leveraging LLMs, businesses can enhance their operations, offering personalized and efficient solutions that meet the dynamic needs of their customers."
              },
              finish_reason: "stop",
              index: 0
            }
          ]
        }
      }
    }
    setAnswer(response.data.answer) // Set the answer state with the response from the server
    setShowAnswer(true)
  }

  return (
    <Box
      sx={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100%',
        width: '100%',
      }}
    >
      <Button variant="contained" onClick={handleOpen}>
        Ask a Question
      </Button>
      <Dialog
        open={open}
        onClose={handleClose}
        aria-labelledby="ask-question-modal-title"
        aria-describedby="ask-question-modal-description"
        maxWidth="md"
        fullWidth
      >
        <DialogTitle sx={{display: 'flex', justifyContent: 'flex-start', alignItems: 'center', flexDirection: 'row'}}>{themeConfig.logo()}Ask Helix</DialogTitle>
        <Box
          sx={{
            minWidth: '100%', // Adjust width as needed
            width: '100%',
            boxShadow: 24,
            py: 2,
            px: 4,
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
            zIndex: 'modal',
          }}
        >
          <Box>
          {showAnswer && (
              <Box>
                <Box
                  sx={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    my: 2,
                  }}
                >
                  <Box sx={{ typography: 'body1' }}>
                    Verified Sources
                  </Box>
                  <IconButton aria-label="recycle">
                    <RecycleIcon />
                  </IconButton>
                </Box>
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    justifyContent: 'flex-start',
                    gap: 1,
                  }}
                >
                  {verifiedSources.map((source, index) => (
                    <Box
                      key={source.id}
                      sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        gap: 2,
                        border: 1,
                        borderColor: 'primary.main',
                        borderRadius: 3,
                        px: 2,
                        py: 1,
                        width: '33%',
                        overflow: 'hidden',
                      }}
                    >
                      <Box
                        component="a"
                        href={source.url}
                        sx={{
                          color: themeConfig.darkText,
                          textDecoration: 'none',
                          width: '100%',
                          textAlign: 'center',
                        }}
                      >
                        <Box sx={{ typography: 'body1', overflow: 'hidden', textOverflow: 'ellipsis' }}>{index + 1}. {source.name}</Box>
                        <Box sx={{ typography: 'body1', overflow: 'hidden', textOverflow: 'ellipsis' }}>{source.url}</Box>
                      </Box>
                    </Box>
                  ))}
                </Box>
                <Box sx={{ mt: 3, mb: 2, typography: 'subtitle1' }}>Your Answer</Box>
                <Box sx={{ typography: 'body1' }}>{answer?.choices[0]?.message?.content}</Box>
                <Box sx={{ mt: 3, mb: 2, typography: 'subtitle1' }}>Was this answer helpful?</Box>
                <Box sx={{ display: 'flex', flexDirection: 'row', gap: 1, mt: 2 }}>
                  <Button 
                    variant={selectedAnswer === 'yes' ? 'contained' : 'outlined'} 
                    size="small" 
                    onClick={() => setSelectedAnswer('yes')}
                    sx={{
                      borderColor: themeConfig.magentaDark, 
                      color: selectedAnswer === 'yes' ? themeConfig.darkText : themeConfig.darkText,
                      backgroundColor: selectedAnswer === 'yes' ? themeConfig.magentaDark : '',
                    }}
                  >
                    Yes
                  </Button>
                  <Button 
                    variant={selectedAnswer === 'no' ? 'contained' : 'outlined'} 
                    size="small" 
                    onClick={() => setSelectedAnswer('no')}
                    sx={{
                      borderColor: themeConfig.magentaDark, 
                      color: selectedAnswer === 'no' ? themeConfig.darkText : themeConfig.darkText,
                      backgroundColor: selectedAnswer === 'no' ? themeConfig.magentaDark : '',
                    }}
                  >
                    No
                  </Button>
                </Box>
              </Box>
            )}
            <Box
              sx={{
                mt: 2,
              }}
            >
              <TextField
                autoFocus
                margin="dense"
                id="question"
                label="Your Query"
                type="text"
                fullWidth
                variant="standard"
                placeholder="How to deploy my application?" // Add placeholder text
                value={question}
                onChange={handleQuestionChange}
                InputProps={{
                  disableUnderline: true,
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton onClick={handleSubmitQuestion}>
                        <SendIcon
                          color="primary"
                          fontSize="small"
                        />
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </Box>
          </Box>
        </Box>
        <DialogActions>
          <Button onClick={handleClose}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default AskQuestionModal
