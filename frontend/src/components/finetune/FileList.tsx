import React, { FC } from 'react'
import Box from '@mui/material/Box'

import Caption from '../widgets/Caption'
import FileIcon from './FileIcon'

import {
  IUploadFile,
} from '../../types'

export const FileList: FC<{
  files: IUploadFile[],
}> = ({
  files,
}) => {
  return (
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
  )
}

export default FileList