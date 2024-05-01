import React, { FC, useState } from 'react'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'

import AddIcon from '@mui/icons-material/Add'

import FileUploadArea from './FileUploadArea'

import useLightTheme from '../../hooks/useLightTheme'
import useSnackbar from '../../hooks/useSnackbar'
import useEnterPress from '../../hooks/useEnterPress'

import {
  IUploadFile,
} from '../../types'

export const AddDocumentsForm: FC<{
  files: IUploadFile[],
  onAddFiles: (files: IUploadFile[]) => void,
}> = ({
  files,
  onAddFiles,
}) => {
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const [ manualTextFile, setManualTextFile ] = useState('')
  const [ manualURL, setManualURL ] = useState('')
  
  const onAddURL = () => {
    if (!manualURL.match(/^https?:\/\//i)) {
      snackbar.error(`Please enter a valid URL`)
      return
    }
    const title = decodeURIComponent(manualURL.replace(/\/$/i, '')).replace(/^https?:\/\//i, '').replace(/^www\./i, '')
    onAddFiles([{
      label: manualURL,
      file: new File([new Blob([manualURL], { type: 'text/html' })], `${title}.url`),
    }])
    setManualURL('')
  }

  const onAddTextFile = () => {
    if (!manualTextFile) {
      snackbar.error(`Please enter some text`)
      return
    }
    const counter = files.reduce((acc, file) => {
      return acc + (file.file.name.match(/\.txt$/i) ? 1 : 0)
    }, 0)
    
    const title = `textfile-${counter}.txt`
    onAddFiles([{
      label: manualTextFile,
      file: new File([new Blob([manualTextFile], { type: 'text/plain' })], title)
    }])
    setManualTextFile('')
  }

  const handleKeyDownURL = useEnterPress({
    value: manualURL,
    updateHandler: setManualURL,
    triggerHandler: onAddURL,
  })

  return (
    <>
      <Typography
        sx={{
          fontWeight: 'bold',
          mt: 3,
        }}
        className="interactionMessage"
      >
        Add URLs, paste some text or upload some files you want your model to learn from:
      </Typography>
      <Typography
        sx={{
          width: '100%',
          pb: 1,
          mt: 3,
          mb: 0.5,
          fontSize: '1rem',
          fontWeight: 'bold',
          color: lightTheme.textColorFaded,
        }}
      >
        Links
      </Typography>
      <TextField
        fullWidth
        label="Type or paste a link (eg https://google.com)"
        value={ manualURL }
        onChange={(e) => setManualURL(e.target.value)}
        onKeyDown={ handleKeyDownURL }
        sx={{
          backgroundColor: '#000',
          borderRadius: 0,
          borderWidth: 1,
          '& .MuiOutlinedInput-root': {
            '& fieldset': {
              borderColor: lightTheme.icon,
            },
            '&:hover fieldset': {
              borderColor: lightTheme.icon,
            },
            '&.Mui-focused fieldset': {
              borderColor: lightTheme.icon,
            },
          },
        }}
        InputLabelProps={{
          sx: {
            color: lightTheme.textColorFaded,
            '&.Mui-focused': {
              color: lightTheme.textColorFaded,
            } 
          }
        }}
        InputProps={{
          style: { borderRadius: 0 },
          endAdornment: manualURL && (
            <IconButton
              onClick={onAddURL}
              sx={{
                marginLeft: 'auto',
                height: '40px',
                backgroundColor: 'transparent',
              }}
            >
              <AddIcon sx={{ color: '#ffff00' }} />
            </IconButton>
          ),
        }}
      />
      <Typography
        sx={{
          width: '100%',
          pb: 1,
          mt: 3,
          mb: 0.5,
          fontSize: '1rem',
          fontWeight: 'bold',
          color: lightTheme.textColorFaded,
        }}
      >
        Text
      </Typography>
      <TextField
        fullWidth
        label="Paste some text here"
        value={ manualTextFile }
        onChange={(e) => setManualTextFile(e.target.value)}
        multiline
        rows={3}
        sx={{
          backgroundColor: '#000',
          borderRadius: 0,
          borderWidth: 1,
          '& .MuiOutlinedInput-root': {
            '& fieldset': {
              borderColor: lightTheme.icon,
            },
            '&:hover fieldset': {
              borderColor: lightTheme.icon,
            },
            '&.Mui-focused fieldset': {
              borderColor: lightTheme.icon,
            },
          },
        }}
        InputLabelProps={{
          sx: {
            color: lightTheme.textColorFaded,
            '&.Mui-focused': {
              color: lightTheme.textColorFaded,
            } 
          }
        }}
        InputProps={{
          style: { borderRadius: 0 },
          endAdornment: manualTextFile && (
            <IconButton
              onClick={onAddTextFile}
              sx={{
                marginLeft: 'auto',
                height: '40px',
                backgroundColor: 'transparent',
              }}
            >
              <AddIcon sx={{ color: '#ffff00' }} />
            </IconButton>
          ),
        }}
      />
      <Typography
        sx={{
          width: '100%',
          pb: 1,
          mt: 3,
          mb: 0.5,
          fontSize: '1rem',
          fontWeight: 'bold',
          color: lightTheme.textColorFaded,
        }}
      >
        Files
      </Typography>
      <FileUploadArea
        onlyDocuments
        files={ files }
        height={ 120 }
        onAddFiles={ onAddFiles }
      />
    </>
  )
}

export default AddDocumentsForm