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
}> = ({
  onChange,
  layout = 'horizontal',
  header = true,
  conversationStarters = [],
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
        <Grid container spacing={0.5} justifyContent="center" alignItems="center" sx={{ width: 'auto', margin: 0 }}>
          {conversationStarters.map((prompt, index) => (
            <Grid item xs={12} sm={6} md={3} key={index} sx={{ display: 'flex', justifyContent: 'center' }}>
              <Box
                sx={{
                  minWidth: 180,
                  minHeight: 80,
                  width: 180,
                  height: 80,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  border: '1px solid rgba(255,255,255,0.2)',
                  borderRadius: '12px',
                  padding: 2,
                  fontSize: '0.92rem',
                  lineHeight: 1.4,
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
                    fontSize: 'inherit',
                    lineHeight: 'inherit',
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
            </Grid>
          ))}
        </Grid>
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
