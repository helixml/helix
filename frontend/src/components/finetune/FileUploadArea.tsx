import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import FileUpload from '../widgets/FileUpload'

import useLightTheme from '../../hooks/useLightTheme'

import {
  IUploadFile,
} from '../../types'

export const FileUploadArea: FC<{
  height?: number,
  onlyImages?: boolean,
  onlyDocuments?: boolean,
  onAddFiles: (files: IUploadFile[]) => void,
}> = ({
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
    </>
  )
}

export default FileUploadArea