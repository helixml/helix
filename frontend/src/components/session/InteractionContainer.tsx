import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import { useTheme } from '@mui/material/styles'
import { getChatColors } from './chatStyles'
export const InteractionContainer: FC<{    
  background?: boolean,
  buttons?: React.ReactNode,
  children?: React.ReactNode,
  align?: 'left' | 'right',
  border?: boolean,
  isAssistant?: boolean,
  messageRole?: 'user' | 'assistant',
}> = ({
  background = false,
  buttons,
  children,
  align = 'left',
  border = false,
  isAssistant = false,
  messageRole,
}) => {
  const theme = useTheme()
  const chatColors = getChatColors(theme)
  const isChatMessage = !!messageRole
  const assistant = messageRole === 'assistant' || isAssistant

  return (
    <Box
      data-chat-message-role={messageRole}
      sx={{
        px: isChatMessage ? (assistant ? 0 : 1.5) : 2,
        py: isChatMessage ? (assistant ? 0 : 1.25) : 0.5,
        borderRadius: assistant ? 0 : 2,
        backgroundColor: isChatMessage && !assistant
          ? chatColors.userBubble
          : background
            ? assistant && theme.palette.mode === 'dark'
              ? '#0d0d0d'
              : theme.palette.background.default
            : 'transparent',
        color: isChatMessage
          ? assistant
            ? chatColors.assistantForeground
            : chatColors.foreground
          : undefined,
        border: isChatMessage ? 'none' : border ? '1px solid #33373a' : 'none',
        // User messages: fit content but don't exceed container width
        // Assistant messages: take full width
        maxWidth: assistant ? '100%' : isChatMessage ? '80%' : 'min(100%, 700px)',
        minWidth: 0,
        width: assistant ? '100%' : 'fit-content',
        ml: align === 'left' ? 0 : 'auto',
        mr: align === 'right' ? 0 : 'auto',
        boxShadow: isChatMessage ? 'none' : border ? '0 1px 2px rgba(0,0,0,0.03)' : 'none',
        // Ensure text wraps properly
        wordBreak: 'break-word',
        overflowWrap: 'anywhere',
        boxSizing: 'border-box',
      }}
    >
      <Row>
        <Cell grow />
        <Cell>
          {buttons}
        </Cell>
      </Row>

      {children}
    </Box>
  );
};

export default InteractionContainer;
