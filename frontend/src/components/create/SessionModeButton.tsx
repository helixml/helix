import React, { FC } from 'react'
import Button from '@mui/material/Button'

import SwapHorizIcon from '@mui/icons-material/SwapHoriz'

import useThemeConfig from '../../hooks/useThemeConfig'

import {
  ISessionMode,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
} from '../../types'

const SessionModeButton: FC<{
  mode: ISessionMode,
  onSetMode: (mode: ISessionMode) => void,
}> = ({
  mode,
  onSetMode,
}) => {
  const themeConfig = useThemeConfig()
  return (
    <Button
      variant="contained"
      size="small"
      sx={{
        bgcolor: mode == SESSION_MODE_INFERENCE ? themeConfig.tealRoot : themeConfig.magentaRoot, // Green for image, Yellow for text
        ":hover": {
          bgcolor: mode == SESSION_MODE_INFERENCE ? themeConfig.tealLight : themeConfig.magentaLight, // Lighter on hover
        },
        color: 'black',
        mr: 2,
        borderRadius: 1,
        textTransform: 'none',
        fontSize: "medium",
        fontWeight: 800,
        pt: '1px',
        pb: '1px',
        m: 0.5,
        display: "inline-flex", // Changed from "inline" to "inline-flex" to align icon with text
        alignItems: "center", // Added to vertically center the text and icon
        justifyContent: "center", // Added to horizontally center the text and icon
      }}
      endIcon={<SwapHorizIcon />}
      onClick={() => onSetMode((mode == SESSION_MODE_INFERENCE ? SESSION_MODE_FINETUNE : SESSION_MODE_INFERENCE))}
    >
      {mode == SESSION_MODE_INFERENCE ? "INFERENCE" : "FINE TUNE"}
    </Button>
  )
}

export default SessionModeButton
