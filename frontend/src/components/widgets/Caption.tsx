import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import { SxProps } from '@mui/system'

export type TerminalTextConfig = {
  backgroundColor?: string,
  color?: string,
}

const Caption: FC<{
  sx?: SxProps,
}> = ({
  sx = {},
  children,
}) => {
  return (
    <Typography
      component="div"
      variant="caption"
      sx={{
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        ...sx
      }}
    >
      { children }
    </Typography>
  )
}

export default Caption
