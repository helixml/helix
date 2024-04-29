import React, { FC } from 'react'
import Button from '@mui/material/Button'

import SwapHorizIcon from '@mui/icons-material/SwapHoriz'

import useThemeConfig from '../../hooks/useThemeConfig'

import {
  ISessionType,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE
} from '../../types'

const SessionTypeSwitch: FC<{
  type: ISessionType,
  onSetType: (type: ISessionType) => void,
}> = ({
  type,
  onSetType,
}) => {
  const themeConfig = useThemeConfig()
  return (
    <Button
      variant="contained"
      size="small"
      sx={{
        bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowRoot : themeConfig.greenRoot, // Green for image, Yellow for text
        ":hover": {
          bgcolor: type == SESSION_TYPE_TEXT ? themeConfig.yellowLight : themeConfig.greenLight, // Lighter on hover
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
      onClick={() => onSetType((type == SESSION_TYPE_TEXT ? SESSION_TYPE_IMAGE : SESSION_TYPE_TEXT))}
    >
      {type == SESSION_TYPE_TEXT ? "TEXT" : "IMAGE"}
    </Button>
  )
}

export default SessionTypeSwitch
