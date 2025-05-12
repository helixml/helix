import React, { FC, useState } from 'react'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'

const CopyButtonWithCheck: FC<{ text: string, alwaysVisible?: boolean }> = ({ text, alwaysVisible }) => {
  const [copied, setCopied] = useState(false)
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      // Optionally handle error
    }
  }
  return (
    <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="top">
      <IconButton
        onClick={handleCopy}
        size="small"
        className="copy-btn"
        sx={theme => ({
          mt: 0.5,
          mr: 1,
          opacity: alwaysVisible ? 1 : 0,
          transition: 'opacity 0.2s',
          position: alwaysVisible ? 'static' : 'absolute',
          left: alwaysVisible ? undefined : -36,
          top: alwaysVisible ? undefined : 14,
          padding: '2px',
          background: 'none',
          color: theme.palette.mode === 'light' ? '#222' : '#bbb',
          '&:hover': {
            background: 'none',
            color: theme.palette.mode === 'light' ? '#000' : '#fff',
          },
        })}
        aria-label="copy"
      >
        {copied ? <CheckIcon sx={{ fontSize: 18 }} /> : <ContentCopyIcon sx={{ fontSize: 18 }} />}
      </IconButton>
    </Tooltip>
  )
}

export default CopyButtonWithCheck 