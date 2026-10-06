import { FC } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import { alpha } from '@mui/material/styles'

type Props = {
  visibility?: string
  privateLabel?: string
}

const ArtifactVisibilityBadge: FC<Props> = ({ visibility, privateLabel = 'Project' }) => {
  const isPublic = visibility === 'public'

  return (
    <Chip
      icon={<Box />}
      label={isPublic ? 'Public' : privateLabel}
      size="small"
      sx={(theme) => {
        const color = isPublic ? theme.palette.success.main : theme.palette.text.secondary
        return {
          height: 20,
          color,
          backgroundColor: alpha(color, 0.1),
          border: `1px solid ${alpha(color, 0.3)}`,
          borderRadius: '999px',
          fontWeight: 500,
          typography: 'caption',
          '& .MuiChip-icon': {
            width: 6,
            height: 6,
            marginLeft: '6px',
            marginRight: '-2px',
            borderRadius: '50%',
            color: 'inherit',
            backgroundColor: 'currentColor',
          },
          '& .MuiChip-label': {
            paddingLeft: '6px',
            paddingRight: '7px',
            lineHeight: 1,
          },
        }
      }}
    />
  )
}

export default ArtifactVisibilityBadge
