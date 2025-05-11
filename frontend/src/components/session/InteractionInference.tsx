import React, { FC, useState } from 'react'
import { styled } from '@mui/system'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import ReplayIcon from '@mui/icons-material/Replay'
import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import Markdown from './Markdown'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

import {
  emitEvent,
} from '../../utils/analytics'

import {
  ISession,
  IServerConfig,
} from '../../types'

const GeneratedImage = styled('img')({
  cursor: 'pointer',
  transition: 'transform 0.2s ease-in-out',
  '&:hover': {
    transform: 'scale(1.05)',
  },
})

const ImagePreview = styled('img')({
  height: '150px',
  width: '150px',
  objectFit: 'cover',
  border: '1px solid #000000',
  borderRadius: '4px',
  cursor: 'pointer',
  transition: 'transform 0.2s ease-in-out',
  '&:hover': {
    transform: 'scale(1.05)',
  },
})

export const InteractionInference: FC<{
  imageURLs?: string[],
  message?: string,
  error?: string,
  serverConfig?: IServerConfig,
  session: ISession,
  onRestart?: () => void,
  upgrade?: boolean,
  isFromAssistant?: boolean,
  onFilterDocument?: (docId: string) => void,
}> = ({
  imageURLs = [],
  message,
  error,
  serverConfig,
  session,
  onRestart,
  upgrade,
  isFromAssistant: isFromAssistant,
  onFilterDocument,
}) => {
    const account = useAccount()
    const router = useRouter()
    const [viewingError, setViewingError] = useState(false)
    const [selectedImage, setSelectedImage] = useState<string | null>(null)

    if (!serverConfig || !serverConfig.filestore_prefix) return null

    const getFileURL = (url: string) => {
      if (!url) return ''
      if (!serverConfig) return ''
      if (url.startsWith('data:')) return url
      return `${serverConfig.filestore_prefix}/${url}?redirect_urls=true`
    }

    return (
      <>
        {
          serverConfig?.filestore_prefix && imageURLs
            .filter(file => {
              return account.token ? true : false
            })
            .map((imageURL: string) => {
              const useURL = getFileURL(imageURL)
              return (
                <Box
                  sx={{
                    mb: 2,
                    display: 'flex',
                    gap: 1,
                  }}
                  key={useURL}
                >
                  <ImagePreview
                    src={useURL}
                    onClick={() => setSelectedImage(useURL)}
                    alt="Preview"
                  />
                </Box>
              )
            })
        }
        {
          message && (
            <Box
              sx={{
                my: 0.5,
                display: 'flex',
                alignItems: 'flex-start',
                position: 'relative',
                ':hover .copy-btn': { opacity: 1 },
              }}
            >
              {isFromAssistant && (
                <CopyButtonWithCheck text={message} />
              )}
              <Box sx={{ flex: 1 }}>
                <Markdown
                  text={message}
                  session={session}
                  getFileURL={getFileURL}
                  showBlinker={false}
                  isStreaming={false}
                  onFilterDocument={onFilterDocument}
                />
              </Box>
            </Box>
          )
        }
        {
          error && (
            <Row>
              <Cell grow>
                <Alert severity="error">
                  The system has encountered an error -
                  <ClickLink
                    sx={{
                      pl: 0.5,
                      pr: 0.5,
                    }}
                    onClick={() => {
                      setViewingError(true)
                    }}
                  >
                    click here
                  </ClickLink>
                  to view the details.
                </Alert>
              </Cell>
              {
                !upgrade && onRestart && (
                  <Cell
                    sx={{
                      ml: 2,
                    }}
                  >
                    <Button
                      variant="contained"
                      color="secondary"
                      size="small"
                      endIcon={<ReplayIcon />}
                      onClick={onRestart}
                    >
                      Retry
                    </Button>
                  </Cell>
                )
              }
              {
                upgrade && (
                  <Cell
                    sx={{
                      ml: 2,
                    }}
                  >
                    <Button
                      variant="contained"
                      color="secondary"
                      size="small"
                      onClick={() => {
                        emitEvent({
                          name: 'queue_upgrade_clicked'
                        })
                        router.navigate('account')
                      }}
                    >
                      Upgrade
                    </Button>
                  </Cell>
                )
              }
            </Row>
          )
        }
        {
          viewingError && (
            <TerminalWindow
              open
              title="Error"
              data={error}
              onClose={() => {
                setViewingError(false)
              }}
            />
          )
        }
        {
          selectedImage && (
            <Box
              sx={{
                position: 'fixed',
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                bgcolor: 'rgba(0, 0, 0, 0.8)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                zIndex: 9999,
              }}
              onClick={() => setSelectedImage(null)}
            >
              <GeneratedImage
                src={selectedImage}
                sx={{
                  maxHeight: '90vh',
                  maxWidth: '90vw',
                  objectFit: 'contain',
                }}
                onClick={(e) => e.stopPropagation()}
              />
            </Box>
          )
        }
      </>
    )
  }

const CopyButtonWithCheck: FC<{ text: string }> = ({ text }) => {
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
          opacity: 0,
          transition: 'opacity 0.2s',
          position: 'absolute',
          left: -36,
          top: 14,
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

export default InteractionInference