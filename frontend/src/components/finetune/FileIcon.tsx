import React, { FC } from 'react'
import { SxProps } from '@mui/material/styles'
import AttachFileIcon from '@mui/icons-material/AttachFile'
import LinkIcon from '@mui/icons-material/Link'
import TextFieldsIcon from '@mui/icons-material/TextFields'
import ImageIcon from '@mui/icons-material/Image'

export const FileIcon: FC<{
  name: string,
  sx?: SxProps,
}> = ({
  name,
  sx = {},
}) => {

  let UseIcon = AttachFileIcon

  if(name.match(/\.txt/i)) {
    UseIcon = TextFieldsIcon
  } else if(name.match(/\.url/i)) {
    UseIcon = LinkIcon
  } else if(name.match(/\.(jpe?g|png|gif|bmp|webp)$/i)) {
    UseIcon = ImageIcon
  }

  return (
    <UseIcon sx={ sx } />
  )
}

export default FileIcon