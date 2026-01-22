import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import { useTheme } from '@mui/material/styles'
export const InteractionContainer: FC<{    
  background?: boolean,
  buttons?: React.ReactNode,
  children?: React.ReactNode,
  align?: 'left' | 'right',
  border?: boolean,
  isAssistant?: boolean,
}> = ({
  background = false,
  buttons,
  children,
  align = 'left',
  border = false,
  isAssistant = false,
}) => {
  const theme = useTheme()

  return (
    <Box
      sx={{
        px: 2,
        py: 0.5,
        borderRadius: 4,
        backgroundColor: background ? theme.palette.background.default : 'transparent',
        border: border ? '1px solid #33373a' : 'none',
        // User messages: fit content but don't exceed container width
        // Assistant messages: take full width
        maxWidth: isAssistant ? '100%' : 'min(100%, 700px)',
        minWidth: 0,
        width: isAssistant ? '100%' : 'fit-content',
        ml: align === 'left' ? 0 : 'auto',
        mr: align === 'right' ? 0 : 'auto',
        boxShadow: border ? '0 1px 2px rgba(0,0,0,0.03)' : 'none',
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