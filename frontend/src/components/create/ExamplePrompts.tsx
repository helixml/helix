import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'

import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'

import {
  ISessionType,
} from '../../types'

import {
  EXAMPLE_PROMPTS,
} from '../../config'

const ExamplePrompts: FC<{
  type: ISessionType,
  onChange: (prompt: string) => void,
}> = ({
  type,
  onChange,
}) => {
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()

  const examplePrompts = useMemo(() => {
    const usePrompts = EXAMPLE_PROMPTS[type] || []
    return usePrompts.sort(() => Math.random() - 0.5).slice(0, isBigScreen ? 3 : 2)
  }, [
    isBigScreen,
    type,
  ])
  
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Typography variant="body2" sx={{mb: 1}}>
        Try an example
      </Typography>
      <Grid container spacing={2}>
        {examplePrompts.map((prompt, index) => (
          <Grid item xs={12} sm={4} key={index}>
            <Box
              sx={{
                width: '100%',
                height: '100%',
                cursor: 'pointer',
                border: lightTheme.border,
                borderRadius: 3,
                padding: 1.5,
                fontSize: 'small',
                lineHeight: 1.4,
              }}
              onClick={() => onChange(prompt)}
            >
              {prompt}
            </Box>
          </Grid>
        ))}
      </Grid>
    </Box>
  )
}

export default ExamplePrompts
