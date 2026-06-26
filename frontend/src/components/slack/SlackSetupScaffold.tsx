// Shared building blocks for Slack app setup instructions. Both the
// per-agent trigger setup (TriggerSlackSetup) and the deployment-wide
// global app setup (dashboard/SlackAppSetup) render the same paradigm —
// numbered steps, an expandable copy-paste manifest, copyable URLs — so
// the markup lives here once.

import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemIcon from '@mui/material/ListItemIcon'
import Divider from '@mui/material/Divider'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'
import CloseIcon from '@mui/icons-material/Close'
import DarkDialog from '../dialog/DarkDialog'

// SetupStep is one numbered instruction. `image` is a screenshot shown
// under the step (click to enlarge); `below` is any other indented
// content (a manifest block, a token input, a copy field, …).
export interface SetupStep {
  step: number
  text: React.ReactNode
  link?: string
  linkLabel?: string
  image?: string
  below?: React.ReactNode
}

// StepImage renders a step's screenshot, clickable to enlarge.
const StepImage: FC<{ src: string; step: number; onClick: (src: string) => void }> = ({ src, step, onClick }) => (
  <Box sx={{ position: 'relative', display: 'inline-block' }}>
    <Box
      component="img"
      src={src}
      alt={`Step ${step} screenshot`}
      onClick={() => onClick(src)}
      sx={{
        width: '80%', maxWidth: '80%', maxHeight: '200px', height: 'auto',
        borderRadius: 1, border: '1px solid rgba(128,128,128,0.25)',
        boxShadow: '0 2px 8px rgba(0,0,0,0.3)', cursor: 'pointer',
        transition: 'transform 0.2s ease-in-out',
        '&:hover': { transform: 'scale(1.02)', boxShadow: '0 4px 12px rgba(0,0,0,0.4)' },
      }}
    />
  </Box>
)

// StepNumber is the filled circle holding a step's number.
export const StepNumber: FC<{ n: number }> = ({ n }) => (
  <Box sx={{
    width: 24, height: 24, borderRadius: '50%', mt: 0.4,
    backgroundColor: 'primary.main', color: 'white',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    fontSize: '0.875rem', fontWeight: 'bold',
  }}>{n}</Box>
)

// useCopied gives a copy handler that briefly flips to a "copied" state
// for button/icon feedback.
function useCopied(): [boolean, (text: string) => void] {
  const [copied, setCopied] = useState(false)
  const copy = (text: string) => {
    navigator.clipboard.writeText(text).then(
      () => { setCopied(true); setTimeout(() => setCopied(false), 1500) },
      () => { /* ignore */ },
    )
  }
  return [copied, copy]
}

// CopyField is a read-only value with a one-click copy button — used for
// the redirect / events URLs the operator pastes into Slack.
export const CopyField: FC<{ label: string; value: string }> = ({ label, value }) => {
  const [copied, copy] = useCopied()
  return (
    <TextField
      fullWidth
      size="small"
      label={label}
      value={value}
      InputProps={{
        readOnly: true,
        sx: { fontFamily: 'monospace', fontSize: '0.8rem' },
        endAdornment: (
          <InputAdornment position="end">
            <IconButton size="small" edge="end" onClick={() => copy(value)}>
              {copied ? <CheckIcon fontSize="small" color="success" /> : <ContentCopyIcon fontSize="small" />}
            </IconButton>
          </InputAdornment>
        ),
      }}
    />
  )
}

// CopyableCodeBlock is the expandable header + Copy button + <pre> body
// used for the Slack app manifest.
export const CopyableCodeBlock: FC<{ title?: string; code: string }> = ({ title = 'App Manifest', code }) => {
  const [expanded, setExpanded] = useState(false)
  const [copied, copy] = useCopied()
  return (
    <Box sx={{ border: '1px solid rgba(128,128,128,0.25)', borderRadius: 1, overflow: 'hidden' }}>
      <Box sx={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between', p: 1.5,
        borderBottom: expanded ? '1px solid rgba(128,128,128,0.25)' : 'none',
      }}>
        <Typography variant="subtitle2"
          sx={{ fontWeight: 'medium', cursor: 'pointer', '&:hover': { color: 'primary.main' } }}
          onClick={() => setExpanded(!expanded)}>
          {title}
        </Typography>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button size="small" variant="text"
            startIcon={copied ? <CheckIcon /> : <ContentCopyIcon />}
            onClick={() => copy(code)}
            sx={{ minWidth: 'auto', px: 1.5, py: 0.5, fontSize: '0.75rem' }}>
            {copied ? 'Copied' : 'Copy'}
          </Button>
          <IconButton size="small" onClick={() => setExpanded(!expanded)} sx={{ p: 0.5 }}>
            {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
          </IconButton>
        </Box>
      </Box>
      {expanded && (
        <Box sx={{ p: 2 }}>
          <Box component="pre" sx={{
            backgroundColor: 'rgba(0,0,0,0.3)', p: 2, borderRadius: 1, fontSize: '0.75rem',
            overflow: 'auto', maxHeight: 240, border: '1px solid rgba(128,128,128,0.25)',
            whiteSpace: 'pre-wrap', wordBreak: 'break-word', m: 0,
          }}>{code}</Box>
        </Box>
      )}
    </Box>
  )
}

// SetupStepList renders a numbered list of steps with per-step
// screenshot (`image`, click to enlarge) + `below` content + dividers.
// The image-enlarge modal is owned here so every consumer gets it free.
export const SetupStepList: FC<{ steps: SetupStep[] }> = ({ steps }) => {
  const [selectedImage, setSelectedImage] = useState<string | null>(null)
  return (
    <>
      <List sx={{ mb: 1 }}>
        {steps.map((s, index) => (
          <React.Fragment key={s.step}>
            <ListItem sx={{ px: 0, flexDirection: 'column', alignItems: 'flex-start' }}>
              <Box sx={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
                <ListItemIcon sx={{ minWidth: 40 }}>
                  <StepNumber n={s.step} />
                </ListItemIcon>
                <ListItemText
                  primary={
                    s.link ? (
                      <Typography>
                        {s.text}{' '}
                        <Typography component="a" href={s.link} target="_blank" rel="noopener noreferrer"
                          sx={{ color: 'primary.main', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}>
                          {s.linkLabel || 'Open ↗'}
                        </Typography>
                      </Typography>
                    ) : (
                      <Typography>{s.text}</Typography>
                    )
                  }
                />
              </Box>
              {s.image && (
                <Box sx={{ ml: 6, mt: 1.5, width: 'calc(100% - 48px)' }}>
                  <StepImage src={s.image} step={s.step} onClick={setSelectedImage} />
                </Box>
              )}
              {s.below && (
                <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>{s.below}</Box>
              )}
            </ListItem>
            {index < steps.length - 1 && <Divider sx={{ ml: 6 }} />}
          </React.Fragment>
        ))}
      </List>

      {/* Shared enlarge modal */}
      <DarkDialog
        open={!!selectedImage}
        onClose={() => setSelectedImage(null)}
        PaperProps={{ sx: { background: 'transparent', boxShadow: 'none', overflow: 'visible', display: 'inline-block', p: 0, m: 0 } }}
      >
        <Box sx={{ position: 'relative', textAlign: 'center', p: 0, m: 0 }}>
          <IconButton
            aria-label="close"
            onClick={() => setSelectedImage(null)}
            sx={{ position: 'absolute', top: 8, right: 8, zIndex: 2, color: 'white', background: 'rgba(0,0,0,0.4)', '&:hover': { background: 'rgba(0,0,0,0.7)' } }}
          >
            <CloseIcon />
          </IconButton>
          {selectedImage && (
            <Box component="img" src={selectedImage} alt="Enlarged screenshot"
              sx={{ maxWidth: '600px', maxHeight: '60vh', height: 'auto', borderRadius: 1, boxShadow: '0 4px 24px rgba(0,0,0,0.7)' }} />
          )}
        </Box>
      </DarkDialog>
    </>
  )
}
