import React, { FC, useState, useCallback, useEffect } from 'react'
import prettyBytes from 'pretty-bytes'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'

import AddCircleIcon from '@mui/icons-material/AddCircle'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'

import FileUpload from '../widgets/FileUpload'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import Caption from '../widgets/Caption'

import useSnackbar from '../../hooks/useSnackbar'
import useAccount from '../../hooks/useAccount'
import Interaction from './Interaction'

import {
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  buttonStates,
} from '../../types'

import {
  getSystemMessage,
} from '../../utils/session'

import {
  mapFileExtension,
} from '../../utils/filestore'

export const TextFineTuneInputs: FC<{
  initialCounter?: number,
  initialFiles?: File[],
  onChange?: {
    (counter: number, files: File[]): void
  },
  onDone: {
    (): void
  },
}> = ({
  initialCounter,
  initialFiles,
  onChange,
  onDone,
}) => {
  const snackbar = useSnackbar()
  const account = useAccount()

  const [manualTextFileCounter, setManualTextFileCounter] = useState(initialCounter || 0)
  const [manualTextFile, setManualTextFile] = useState('')
  const [manualURL, setManualURL] = useState('')
  const [files, setFiles] = useState<File[]>(initialFiles || [])

  const onAddURL = useCallback(() => {
    if(!manualURL.match(/^https?:\/\//i)) {
      snackbar.error(`Please enter a valid URL`)
      return
    }
    let useUrl = manualURL.replace(/\/$/i, '')
    useUrl = decodeURIComponent(useUrl)
    let fileTitle = useUrl
      .replace(/^https?:\/\//i, '')
      .replace(/^www\./i, '')
    const file = new File([
      new Blob([manualURL], { type: 'text/html' })
    ], `${fileTitle}.url`)
    setFiles(files.concat(file))
    setManualURL('')
  }, [
    manualURL,
    files,
  ])

  const onAddTextFile = useCallback(() => {
    const newCounter = manualTextFileCounter + 1
    setManualTextFileCounter(newCounter)
    const file = new File([
      new Blob([manualTextFile], { type: 'text/plain' })
    ], `textfile-${newCounter}.txt`)
    setFiles(files.concat(file))
    setManualTextFile('')
  }, [
    manualTextFile,
    manualTextFileCounter,
    files,
  ])

  const onDropFiles = useCallback(async (newFiles: File[]) => {
    const existingFiles = files.reduce<Record<string, string>>((all, file) => {
      all[file.name] = file.name
      return all
    }, {})
    const filteredNewFiles = newFiles.filter(f => !existingFiles[f.name])
    setFiles(files.concat(filteredNewFiles))
  }, [
    files,
  ])

  useEffect(() => {
    if(!onChange) return
    onChange(manualTextFileCounter, files)
  }, [
    manualTextFileCounter,
    files,
  ])

  return (
    <Box
      sx={{
        mt: 2,
      }}
    >
      <Box
        sx={{
          mt: 4,
          mb: 4,
        }}
      >
        <Interaction
          session_id=""
          session_name=""
          interaction={ getSystemMessage('Firstly, add URLs, paste some text or upload some files you want your model to learn from:') }
          type={ SESSION_TYPE_TEXT }
          mode={ SESSION_MODE_INFERENCE }
          serverConfig={ account.serverConfig }
        />
      </Box>
      <Row
        sx={{
          mb: 2,
          alignItems: 'flex-start',
        }}
      >
        <Cell
          sx={{
            flexGrow: 1,
            pr: 2,
          }}
        >
          <TextField
            fullWidth
            label="Add link, for example https://google.com"
            value={ manualURL }
            onChange={ (e) => {
              setManualURL(e.target.value)
            }}
          />
        </Cell>
        <Cell
          sx={{
            width: '240px',
            minWidth: '240px',
          }}
        >
          <Button
            sx={{
              width: '100%',
            }}
            variant="outlined"
            color={ buttonStates.addUrlColor }
            endIcon={<AddCircleIcon />}
            onClick={ onAddURL }
          >
            { buttonStates.addUrlLabel }
          </Button>
        </Cell>
        
      </Row>
      <Row
        sx={{
          mb: 2,
          alignItems: 'flex-start',
        }}
      >
        <Cell
          sx={{
            flexGrow: 1,
            pr: 2,
            alignItems: 'flex-start',
          }}
        >
          <TextField
            sx={{
              height: '100px',
              maxHeight: '100px'
            }}
            fullWidth
            label="or paste some text here"
            value={ manualTextFile }
            multiline
            rows={ 3 }
            onChange={ (e) => {
              setManualTextFile(e.target.value)
            }}
          />
        </Cell>
        <Cell
          sx={{
            flexGrow: 0,
            width: '240px',
            minWidth: '240px',
          }}
        >
          <Button
            sx={{
              width: '100%',
            }}
            variant="outlined"
            color={ buttonStates.addTextColor }
            endIcon={<AddCircleIcon />}
            onClick={ onAddTextFile }
          >
            { buttonStates.addTextLabel }
          </Button>
        </Cell>
        
      </Row>


      <FileUpload
        sx={{
          width: '100%',
        }}
        onlyDocuments
        onUpload={ onDropFiles }
      >
        <Row
          sx={{
            alignItems: 'flex-start',
          }}
        >
          <Cell
            sx={{
              flexGrow: 1,
              pr: 2,
            }}
          >
            <Box
              sx={{
                border: '1px solid #555',
                borderRadius: '4px',
                p: 2,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'flex-start',
                height: '120px',
                minHeight: '120px',
                cursor: 'pointer',
              }}
            >
              
              <Typography
                sx={{
                  color: '#bbb',
                  width: '100%',
                }}
              >
                drop files here to upload them ...
              </Typography>
              
            </Box>
          </Cell>
          <Cell
            sx={{
              flexGrow: 0,
              width: '240px',
              minWidth: '240px',
            }}
          >
            <Button
              sx={{
                width: '100%',
              }}
              variant="outlined"
              color={ buttonStates.uploadFilesColor }
              endIcon={<CloudUploadIcon />}
            >
              { buttonStates.uploadFilesLabel }
            </Button>
          </Cell>
          
        </Row>

        
      </FileUpload>

      <Box
        sx={{
          mt: 2,
          mb: 2,
        }}
      >
        <Grid container spacing={3} direction="row" justifyContent="flex-start">
          {
            files.length > 0 && files.map((file) => {
              return (
                <Grid item xs={12} md={2} key={file.name}>
                  <Box
                    sx={{
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      justifyContent: 'center',
                      color: '#999'
                    }}
                  >
                    <span className={`fiv-viv fiv-size-md fiv-icon-${mapFileExtension(file.name)}`}></span>
                    <Caption sx={{ maxWidth: '100%'}}>
                      {file.name}
                    </Caption>
                    <Caption>
                      ({prettyBytes(file.size)})
                    </Caption>
                  </Box>
                </Grid>
              )
            })
              
          }
        </Grid>
      </Box>
      {
        files.length > 0 && (
          <Button
            sx={{
              width: '100%',
            }}
            variant="contained"
            color="secondary"
            endIcon={<ArrowCircleRightIcon />}
            onClick={ onDone }
          >
            Next Step
          </Button>
        )
      }
    </Box>
  )   
}

export default TextFineTuneInputs