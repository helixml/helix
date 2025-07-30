import { FC } from 'react'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import SessionBadge from './SessionBadge'

export const SessionBadgeKey: FC = () => {
  return (
    <Grid
      container
      spacing={1}
      sx={{
        display: 'flex',
        alignItems: 'center',
        flexWrap: 'wrap',
        justifyContent: 'flex-end',
      }}
    >
      <Grid item sx={{ display: 'flex', alignItems: 'center' }}>
        <SessionBadge modelName="ollama" />
        <Typography variant="caption" sx={{ ml: 1, color: 'rgba(255, 255, 255, 0.9)' }}>
          Ollama
        </Typography>
      </Grid>

      <Grid item sx={{ display: 'flex', alignItems: 'center' }}>
        <SessionBadge modelName="vllm" />
        <Typography variant="caption" sx={{ ml: 1, color: 'rgba(255, 255, 255, 0.9)' }}>
          VLLM
        </Typography>
      </Grid>

      <Grid item sx={{ display: 'flex', alignItems: 'center' }}>
        <SessionBadge modelName="axolotl" />
        <Typography variant="caption" sx={{ ml: 1, color: 'rgba(255, 255, 255, 0.9)' }}>
          Axolotl
        </Typography>
      </Grid>

      <Grid item sx={{ display: 'flex', alignItems: 'center' }}>
        <SessionBadge modelName="diffusers" />
        <Typography variant="caption" sx={{ ml: 1, color: 'rgba(255, 255, 255, 0.9)' }}>
          Diffusers
        </Typography>
      </Grid>
    </Grid>
  )
}

export default SessionBadgeKey