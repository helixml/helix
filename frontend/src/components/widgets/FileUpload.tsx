import React, {FC,useCallback} from 'react'
import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'
import { useDropzone } from 'react-dropzone'

const FileUpload: FC<{
  sx?: SxProps,
  onUpload: (files: File[]) => Promise<void>,
}> = ({
  children,
  sx = {},
  onUpload,
}) => {
  const {getRootProps, getInputProps, isDragActive} = useDropzone({onDrop: onUpload})

  return (
    <Box {...getRootProps()} sx={ sx }>
      <input {...getInputProps()} />
      {
        children
      }
    </Box>
  )
}

export default FileUpload