import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Stack from '@mui/material/Stack'
import Divider from '@mui/material/Divider'
import LightbulbOutlinedIcon from '@mui/icons-material/LightbulbOutlined'

import useLightTheme from '../../hooks/useLightTheme'

import {
  ISessionType,
} from '../../types'

import {
  EXAMPLE_PROMPTS,
} from '../../config'

type LayoutType = 'horizontal' | 'vertical'

const ExamplePrompts: FC<{
  type: ISessionType,
  onChange: (prompt: string) => void,
  layout?: LayoutType,
  header?: boolean,  
}> = ({
  type,
  onChange,
  layout = 'horizontal',
  header = true,  
}) => {
  const lightTheme = useLightTheme()

  const examplePrompts = useMemo(() => {
    const usePrompts = EXAMPLE_PROMPTS[type] || []
    return usePrompts.sort(() => Math.random() - 0.5).slice(0, 3)
  }, [
    type,    
  ])
  
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        maxWidth: layout === 'horizontal' ? '800px' : '100%',
        width: '100%',
      }}
    >
      {header && (
        <Typography variant="body2" sx={{mb: 1}}>
          Try an example
        </Typography>
      )}
      {layout === 'horizontal' ? (
        <Grid container spacing={2}>
          {examplePrompts.map((prompt, index) => (
            <Grid item xs={4} key={index}>
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
                  backgroundColor: `${lightTheme.isLight ? '#ADD8E630' : '#000020A0'}`
                }}
                onClick={() => onChange(prompt)}
              >
                {prompt}
              </Box>
            </Grid>
          ))}
        </Grid>
      ) : (
        <Stack spacing={0} divider={<Divider />}>
          {examplePrompts.map((prompt, index) => (
            <Box
              key={index}
              sx={{
                width: '100%',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                py: 1.5,
                fontSize: 'small',
                lineHeight: 1.4,
                color: lightTheme.textColor,
                '&:hover': {
                  backgroundColor: 'rgba(0, 0, 0, 0.04)'
                }
              }}
              onClick={() => onChange(prompt)}
            >
              <LightbulbOutlinedIcon sx={{ fontSize: 20, opacity: 0.7, color: lightTheme.textColorFaded }} />
              <Typography
                noWrap
                sx={{
                  fontSize: 'inherit',
                  lineHeight: 'inherit',
                  color: `${lightTheme.textColorFaded} !important`,
                }}
              >
                {prompt}
              </Typography>
            </Box>
          ))}
        </Stack>
      )}
    </Box>
  )
}

export default ExamplePrompts
