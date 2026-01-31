import React, { FC, useState } from 'react'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'

const CopyButtonWithCheck: FC<{ text: string, alwaysVisible?: boolean }> = ({ text }) => {
  const [copied, setCopied] = useState(false)
  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
    
    const textToCopy = sanitizeTextForCopy(text)

    // Fallback method for older browsers or when navigator.clipboard is not available/permission denied
    const fallbackCopy = () => {
      const textArea = document.createElement('textarea')
      textArea.value = textToCopy
      textArea.style.position = 'fixed'
      textArea.style.left = '-9999px'
      document.body.appendChild(textArea)
      textArea.select()
      document.execCommand('copy')
      document.body.removeChild(textArea)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }

    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(textToCopy).then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      }).catch(fallbackCopy)
    } else {
      fallbackCopy()
    }
  }
  return (
    <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="bottom">
      <IconButton
        onClick={handleCopy}
        size="small"
        className="copy-btn"
        sx={theme => ({
          mt: 0.5,
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

const sanitizeTextForCopy = (text: string) => {
  // Remove citation data
  text = removeCitationData(text)
  // Remove document IDs
  text = removeDocumentIds(text)

  return text
}

const removeCitationData = (text: string) => {
  // If 'text' contains citation data, it will have <excerpts> tags. We need to
  // only copy the text up to the first <excerpts> tag.
  const excerptsIndex = text.indexOf('<excerpts>')
  return excerptsIndex !== -1 ? text.substring(0, excerptsIndex) : text
}

const removeDocumentIds = (text: string) => {
  // Remove all [DOC_ID:<id>] entries
  return text.replace(/\[DOC_ID:[^\]]*\]/g, '')
}
