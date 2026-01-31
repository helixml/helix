import Box from '@mui/material/Box'
import { SxProps } from '@mui/system'
import { FC, ReactNode } from 'react'
import { DropzoneOptions, useDropzone } from 'react-dropzone'

const FileUpload: FC<{
  sx?: SxProps,
  onlyImages?: boolean,
  onlyDocuments?: boolean,
  onUpload: (files: File[]) => void,
  children?: ReactNode,
}> = ({
  children,
  sx = {},
  onlyImages = false,
  onlyDocuments = false,
  onUpload,
}) => {
    const opts: DropzoneOptions = {
      onDrop: onUpload,
    }
    if (onlyImages) {
      opts.accept = {
        'image/jpeg': [],
        'image/png': [],
        'image/gif': [],
      }
    }
    if (onlyDocuments) {
      opts.accept = {
        'text/plain': [],
        'text/html': [],
        'text/css': [],
        'text/csv': [],
        'text/javascript': [],
        'application/javascript': [],
        'application/json': ['finetune_dataset.jsonl'],
        'application/xml': [],
        'application/pdf': [],
        'application/msword': [],
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document': [],
        'application/vnd.ms-excel': [],
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': [],
        'application/vnd.ms-powerpoint': [],
        'application/vnd.openxmlformats-officedocument.presentationml.presentation': [],
        'application/rtf': [],
        'application/vnd.oasis.opendocument.text': [],
        'application/vnd.oasis.opendocument.spreadsheet': [],
        'application/vnd.oasis.opendocument.presentation': [],
        'application/jsonl': ['.jsonl'],
      }
    }
    const { getRootProps, getInputProps, isDragActive } = useDropzone(opts)

    return (
      <Box {...getRootProps()} sx={sx}>
        <input {...getInputProps()} />
        {
          children
        }
      </Box>
    )
  }

export default FileUpload