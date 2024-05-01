import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import FileUpload from '../widgets/FileUpload'

import Caption from '../widgets/Caption'
import FileIcon from './FileIcon'

import useLightTheme from '../../hooks/useLightTheme'

import {
  IUploadFile,
} from '../../types'

export const FileUploadArea: FC<{
  files: IUploadFile[],
  height?: number,
  onlyImages?: boolean,
  onlyDocuments?: boolean,
  onAddFiles: (files: IUploadFile[]) => void,
}> = ({
  files,
  height = 120,
  onlyImages = false,
  onlyDocuments = false,
  onAddFiles,
}) => {
  const lightTheme = useLightTheme()

  const onDropFiles = (newFiles: File[]) => {
    onAddFiles(newFiles.map(f => ({
      drawerLabel: f.name,
      file: f,
    })))
  }
  
  return (
    <>
      <FileUpload
        onlyImages={ onlyImages }
        onlyDocuments={ onlyDocuments }
        onUpload={ onDropFiles }
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            justifyContent: 'center',
            height: `${height}px`,
            minHeight: `${height}px`,
            cursor: 'pointer',
            backgroundColor: '#000',
            borderRadius: 0,
            border: `1px solid ${lightTheme.icon}`,
          }}
          onClick={ () => {} }
        >
          <Typography
            sx={{
              color: lightTheme.textColorFaded,
              cursor: 'pointer',
            }}
          >
            Drag files here to upload (or&nbsp;
            <span
              style={{
                textDecoration: 'underline',
                color: lightTheme.textColor,
              }}
            >
              upload manually
            </span>
            )
          </Typography>
        </Box>
      </FileUpload>
      <Box
        sx={{
          display: 'flex',
          flexWrap: 'wrap',
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
          mt: 3,
        }}
       >
        {files.length > 0 && files.map((file, index) => {
          return (
            <Box
              key={file.file.name}
              sx={{
                display: 'inline-flex',
                alignItems: 'center',
                mr: 5,
                mb: 2,
              }}
            >
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                  justifyContent: 'flex-start',
                  color: '#999',
                }}
              >
                <FileIcon
                  name={ file.file.name }
                  sx={{ mr: 1 }}
                />
                <Caption sx={{ maxWidth: '100%', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {file.file.name}
                </Caption>
              </Box>
            </Box>
          )
        })}
      </Box>
    </>
  )
}

export default FileUploadArea