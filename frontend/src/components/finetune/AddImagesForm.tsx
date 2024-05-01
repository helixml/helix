import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import FileUpload from '../widgets/FileUpload'

import AddIcon from '@mui/icons-material/Add'
import AttachFileIcon from '@mui/icons-material/AttachFile'

import Caption from '../widgets/Caption'
import FileIcon from './FileIcon'
import FileUploadArea from './FileUploadArea'

import useLightTheme from '../../hooks/useLightTheme'
import useSnackbar from '../../hooks/useSnackbar'
import useEnterPress from '../../hooks/useEnterPress'

import {
  IUploadFile,
} from '../../types'

export const AddImagesForm: FC<{
  files: IUploadFile[],
  onAddFiles: (files: IUploadFile[]) => void,
}> = ({
  files,
  onAddFiles,
}) => {
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  
  return (
    <>
      <Typography
        sx={{
          fontWeight: 'bold',
          mt: 3,
          mb: 3,
        }}
        className="interactionMessage"
      >
        Upload some images you want your model to learn from
      </Typography>
      <FileUploadArea
        onlyImages
        files={ files }
        height={ 200 }
        onAddFiles={ onAddFiles }
      />
    </>
  )
}

export default AddImagesForm