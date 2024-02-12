import React, { FC } from 'react'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import CloseIcon from '@mui/icons-material/Close'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Button from '@mui/material/Button'
import JsonView from './JsonView'
import useSnackbar from '../../hooks/useSnackbar'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Backdrop from '@mui/material/Backdrop'
import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'

interface JsonWindowProps {
  data: any,
  onClose: {
    (): void,
  },
}

const JsonWindow: FC<React.PropsWithChildren<JsonWindowProps>> = ({
  data,
  onClose,
}) => {
  const snackbar = useSnackbar()
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  return (
    <>
      <Backdrop
        open={true}
        onClick={onClose}
        sx={{
          zIndex: theme.zIndex.drawer + 1,
          opacity: 1,
          color: '#fff',
          backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
        }}
      >
      </Backdrop>
      <Drawer
        anchor='right'
        open={true}
        onClose={onClose}
        sx={{
          zIndex: theme.zIndex.drawer + 2,
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          width: '100%',
        }}
      >
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'flex-end',
            p: 1,
            backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.deepBlue,
          }}
        >
          <Typography
            variant="h5"
            component="h3"
            sx={{

              fontWeight: 'bold',
              flexGrow: 1,
              pt: .5,
              pl: 1,
            }}
          >
            Information
          </Typography>
          <IconButton
            onClick={onClose}
          >
            <CloseIcon />
          </IconButton>
        </Box>
        <Box
          sx={{
            overflowY: 'auto',
            flexGrow: 1,
            maxWidth: '100vw',
            backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkPanel,
          }}
        >
          <JsonView
            data={data}
            scrolling={true}
          />
        </Box>
        <Box
          sx={{
            p: 1,
            display: 'flex',
            justifyContent: 'flex-start',
            backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
          }}
        >
          <CopyToClipboard
            text={JSON.stringify(data, null, 4)}
            onCopy={() => {
              snackbar.success('Copied to clipboard')
            }}
          >
            <Button
              color="secondary"
              variant="text"
              endIcon={<ContentCopyIcon />}
              sx={{
                fontSize: '1.2em',
                color: theme.palette.mode === 'light' ? themeConfig.lightText : themeConfig.darkText,
                
              }}
            >
              Copy to clipboard
            </Button>
          </CopyToClipboard>
        </Box>
      </Drawer>
    </>
  )
}

export default JsonWindow

