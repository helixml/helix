import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import SessionTypeButton from './SessionTypeButton'

import useLightTheme from '../../hooks/useLightTheme'

import {
  ISessionType,
} from '../../types'

const CenterMessage: FC<{
  type: ISessionType,
  onSetType: (type: ISessionType) => void,
}> = ({
  type,
  onSetType,
}) => {
  const lightTheme = useLightTheme()

  return (
    <Box
      sx={{
        textAlign: 'left',
        border: lightTheme.border,
        borderRadius: 3, // Rounded corners
        padding: {
          xs: 2,
          md: 4,
        },
        backgroundColor: `${lightTheme.isLight ? '#ADD8E630' : '#000020A0'}`
      }}
    >
      <Typography
        variant="h4"
        component="h1" gutterBottom
        sx={{
          fontSize: {
            xs: '1.1rem',
            sm: '1.4rem',
            md: '1.7rem',
          },
          fontWeight: 800,
          lineHeight: 0.8,
          scale: {
            xs: 0.6,
            sm: 0.85,
            md: 1,
          },
        }}
      >
        What do you want to do?
      </Typography>
      <Typography variant="subtitle1" sx={{ mt: 2 }}>
        You are in <strong>Inference</strong> mode:
        <Box component="ul" sx={{pl:2, pr: 1, pt:1, mx: .5, my:0, lineHeight: 1.1 }}>
          <Box component="li" sx={{pl:0, pr: 1, py: .5, m: 0}}>Generate new content based on your prompt</Box>
          <Box component="li" sx={{pl:0, pr: 1, py: .5, m: 0}}>Click
            <SessionTypeButton
              type={ type }
              onSetType={ onSetType }
            />
          to change type</Box>
          <Box component="li" sx={{pl:0, pr: 1, py: .5, m: 0}}>Type a prompt into the box below and press enter to begin</Box>
        </Box>
      </Typography>
      <Typography
        variant="subtitle1"
        sx={{
          lineHeight: 1.1,
        }}
      >
        <br/>You can use the toggle at the top to switch to <strong>Fine-tuning</strong> mode:<br/>
        <Box component="ul" sx={{pl:2, pr: 1, pt: 1, mx: .5, my:0, lineHeight: 1.1 }}>
          <Box component="li" sx={{pl:0, pr: 1, py: .5, m: 0}}>Customize your own AI by training it on your own text or images</Box>
        </Box>
      </Typography>
    </Box>
  )
}

export default CenterMessage
