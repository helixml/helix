import React, { useState, useEffect } from 'react'
import Dialog from '@mui/material/Dialog'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import { DialogTitle, DialogActions, InputAdornment, IconButton } from '@mui/material'
import SearchIcon from '@mui/icons-material/Search'
import RefreshIcon from '@mui/icons-material/Refresh'
import CloseIcon from '@mui/icons-material/Close'
import useThemeConfig from '../../hooks/useThemeConfig'

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

type VerifiedSourceType = {
  id: string
  name: string
  url: string
}

interface AskQuestionModalProps {
  verifiedSources: VerifiedSourceType[]
  onSubmitQuestion: (question: string) => Promise<AnswerType>
  open: boolean
  handleClose: () => void
  answer: AnswerType
}

export default ({ verifiedSources, onSubmitQuestion, open, handleClose, answer }: AskQuestionModalProps) => {
  const themeConfig = useThemeConfig()
  const [question, setQuestion] = useState('')
  const [showAnswer, setShowAnswer] = useState(false)

  useEffect(() => {
    if (answer) setShowAnswer(true)
  }, [answer])

  const handleQuestionChange = (event: React.ChangeEvent<HTMLInputElement>) => setQuestion(event.target.value)
  const handleSubmitQuestion = async () => {
    if (question.trim() !== '') {
      try {
        const submissionResult = await onSubmitQuestion(question)
        if (submissionResult) {
          setShowAnswer(true)
        }
      } catch (error) {
        console.error('Error submitting question:', error)
        alert('Submission failed. Please attempt again later.')
      }
    }
  }

  const resetToInitialState = () => {
    setQuestion('')
    setShowAnswer(false)
  }

  const handleKeyPress = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter') {
      handleSubmitQuestion()
    }
  }

  return (
    <Dialog
      open={open}
      onClose={() => {
        handleClose()
        setShowAnswer(false)
      }}
      aria-labelledby="ask-question-modal-title"
      aria-describedby="ask-question-modal-description"
      maxWidth="md"
      fullWidth
    >
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexDirection: 'row', justifyContent: 'flex-start' }}>
          {themeConfig.logo()}Ask Helix
          <Box sx={{flexGrow: 1,}}></Box>
          {showAnswer && (
            <Box sx={{alignSelf: 'flex-end'}}>
              <IconButton
                onClick={resetToInitialState}
                aria-label="reset"
              >
                <CloseIcon
                  color="action"
                  sx={{
                    color: themeConfig.secondary,
                  }}
                />
              </IconButton>
            </Box>
          )}
        </Box>
      </DialogTitle>
      {!showAnswer ? (
        <Box
          sx={{
            minWidth: '100%',
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
          <TextField
            autoFocus
            margin="dense"
            id="question"
            label="Your Query"
            type="text"
            fullWidth
            variant="standard"
            placeholder="How to deploy my application?"
            value={question}
            onChange={handleQuestionChange}
            onKeyPress={handleKeyPress}
            InputProps={{
              disableUnderline: true,
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={handleSubmitQuestion} aria-label="submit question" type="button">
                    <SearchIcon
                      color="primary"
                      fontSize="small"
                    />
                  </IconButton>
                </InputAdornment>
              ),
            }}
          />
        </Box>
      ) : (
        <Box
          sx={{
            minWidth: '100%',
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
          {/* Displaying verified sources and answer */}
          <Box sx={{ typography: 'body1', my: 2 }}>Verified Sources</Box>
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
          <Box sx={{ typography: 'body1', my: 2 }}>Answer</Box>
          <Box sx={{ typography: 'body2', mt: 2, mb: 4, }}>{answer?.choices[0]?.message?.content}</Box>
        </Box>
      )}
      <DialogActions>
        <Button
          variant="outlined"
          sx={{ color: themeConfig.secondary }}
          onClick={() => {
            handleClose()
            setShowAnswer(false)
          }}
        >
          Close
        </Button>
      </DialogActions>
    </Dialog>
  )
}
