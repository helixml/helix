import React, { FC } from 'react'
import CircularProgress, {
  CircularProgressProps,
} from '@mui/material/CircularProgress'

interface SmallSpinnerProps {
  color?: CircularProgressProps["color"],
  size?: number
}

const SmallSpinner: FC<SmallSpinnerProps> = ({
  color = 'primary',
  size = 20
}) => {
  return (
    <CircularProgress 
      color={color}
      size={size}
    />
  )
}

export default SmallSpinner 