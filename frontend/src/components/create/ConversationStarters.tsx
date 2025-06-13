import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Stack from '@mui/material/Stack'
import Divider from '@mui/material/Divider'
import LightbulbOutlinedIcon from '@mui/icons-material/LightbulbOutlined'

import useLightTheme from '../../hooks/useLightTheme'

type LayoutType = 'horizontal' | 'vertical'

const ConversationStarters: FC<{  
  onChange: (prompt: string) => void,
  layout?: LayoutType,
  header?: boolean,
  conversationStarters?: string[],
  mini?: boolean,
}> = ({
  onChange,
  layout = 'horizontal',
  header = true,
  conversationStarters = [],
  mini = false,
}) => {
  const lightTheme = useLightTheme()
  
  if (conversationStarters.length === 0) {
    return null
  }
  
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        width: '100%',
      }}
    >
      {header && (
        <Typography variant="body2" sx={{mb: 1}}>
          Try an example
        </Typography>
      )}
      {layout === 'horizontal' ? (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            gap: 1.5,
            flexWrap: 'wrap',
            width: '100%',
            mb: 1,
          }}
        >
          {conversationStarters.map((prompt, index) => (
            <Box
              key={index}
              sx={{
                width: mini ? 120 : 160,
                height: mini ? 90 : 120,
                minWidth: 0,
                minHeight: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                cursor: 'pointer',
                border: '1px solid rgba(255,255,255,0.2)',
                borderRadius: '12px',
                padding: mini ? 1 : 1.5,
                fontSize: mini ? '0.8rem' : '0.95rem',
                lineHeight: 1.3,
                backgroundColor: 'rgba(255, 255, 255, 0.05)',
                color: lightTheme.textColorFaded,
                textAlign: 'center',
                boxShadow: '0 2px 8px 0 rgba(0,0,0,0.04)',
                overflow: 'hidden',
              }}
              onClick={() => onChange(prompt)}
            >
              <Typography
                sx={{
                  width: '100%',
                  fontSize: mini ? '0.8rem' : '0.95rem',
                  lineHeight: '1.3',
                  color: 'inherit',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  display: '-webkit-box',
                  WebkitLineClamp: 3,
                  WebkitBoxOrient: 'vertical',
                  whiteSpace: 'normal',
                }}
              >
                {prompt}
              </Typography>
            </Box>
          ))}
        </Box>
      ) : (
        <Stack spacing={0} divider={<Divider />} alignItems="stretch">
          {conversationStarters.map((prompt, index) => (
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
                justifyContent: 'center',
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

export default ConversationStarters
