import React, { FC } from 'react'
import Drawer from '@mui/material/Drawer'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Divider from '@mui/material/Divider'

import CloseIcon from '@mui/icons-material/Close'
import FileDownloadIcon from '@mui/icons-material/FileDownload'
import DeleteIcon from '@mui/icons-material/Delete'

import FileIcon from './FileIcon'

import useLightTheme from '../../hooks/useLightTheme'

import {
  IUploadFile,
} from '../../types'

export const FileDrawer: FC<{
  open: boolean,
  files: IUploadFile[],
  onUpdate: (files: IUploadFile[]) => void,
  onClose: () => void,
}> = ({
  open,
  files,
  onUpdate,
  onClose,
}) => {
  const lightTheme = useLightTheme()
  
  return (
    <Drawer
      anchor="right"
      open={ open }
      onClose={ onClose }
      sx={{
        '& .MuiDrawer-paper': {
          backgroundColor: lightTheme.backgroundColor,
          overflowY: 'auto',
        },
      }}
     >
      <Box
        sx={{
          width: '50vh', 
          maxWidth: '100%', 
        }}
        role="presentation"
      >
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '8px 16px',
            borderBottom: '1px solid #e0e0e0',
          }}
         >
          <Typography variant="h6">
            Browse files ({files.length})
          </Typography>
          <IconButton onClick={ onClose }>
            <CloseIcon /> 
          </IconButton>
        </Box>
        <List>
          {
            files.map((file, index) => {
              return (
                <React.Fragment key={file.file.name}>
                  <ListItem
                    sx={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                    }}
                    secondaryAction={
                      <Box sx={{ display: 'flex' }}>
                        <IconButton
                          edge="end"
                          onClick={() => {
                            const url = URL.createObjectURL(file.file)
                            const a = document.createElement('a')
                            a.href = url
                            a.download = file.file.name
                            a.click()
                            URL.revokeObjectURL(url)
                          }}
                          sx={{
                            ml: 1,
                            color: lightTheme.textColorFaded,
                          }}
                        >
                          <FileDownloadIcon />
                        </IconButton>
                        <IconButton
                          edge="end"
                          onClick={() => {
                            const newFiles = files.filter(f => f.file.name !== file.file.name)
                            onUpdate(newFiles)
                          }}
                          sx={{
                            ml: 1,
                            color: lightTheme.textColorFaded,
                          }}
                        >
                          <DeleteIcon />
                        </IconButton>
                      </Box>
                    }
                  >
                    <ListItemIcon sx={{ minWidth: 'auto', mr: 2 }}>
                      <FileIcon
                        name={file.file.name}
                      />
                    </ListItemIcon>
                    <ListItemText
                      primary={ file.label }
                      sx={{
                        mr: 4,
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                      }}
                      primaryTypographyProps={{
                        sx: {
                          color: lightTheme.textColorFaded,
                        }
                      }}
                    />
                  </ListItem>
                  {index < files.length - 1 && (
                    <Divider sx={{ my: 0 }} /> 
                  )}
                </React.Fragment>
              )
            })
          }
        </List>
      </Box>
    </Drawer>
  )
}

export default FileDrawer