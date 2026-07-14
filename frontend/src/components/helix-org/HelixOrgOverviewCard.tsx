import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

export type HelixOrgOverviewCardProps = {
  title: string
  id?: string
  icon: ReactNode
  status?: ReactNode
  idAction?: ReactNode
  children?: ReactNode
}

const HelixOrgOverviewCard: FC<HelixOrgOverviewCardProps> = ({ title, id, icon, status, idAction, children }) => (
  <Box
    sx={{
      p: 2,
      borderRadius: 1.5,
      color: 'common.white',
      background: 'linear-gradient(135deg, #123b4a 0%, #0b6073 52%, #087c93 100%)',
      boxShadow: '0 6px 18px rgba(8, 112, 135, 0.16)',
    }}
  >
    <Stack direction="row" alignItems="flex-start" justifyContent="space-between" spacing={2}>
      <Stack direction="row" spacing={1.25} alignItems="center" sx={{ minWidth: 0 }}>
        <Box sx={{ p: 1, borderRadius: 1.5, backgroundColor: 'rgba(255,255,255,0.14)', display: 'flex' }}>
          {icon}
        </Box>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="h6" sx={{ fontWeight: 650, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {title}
          </Typography>
          {id && (
            <Stack direction="row" spacing={0.25} alignItems="center" sx={{ minWidth: 0 }}>
              <Typography variant="caption" sx={{ opacity: 0.78, fontFamily: 'monospace', overflowWrap: 'anywhere' }}>
                {id}
              </Typography>
              {idAction}
            </Stack>
          )}
        </Box>
      </Stack>
      {status}
    </Stack>
    {children && (
      <Stack direction="row" spacing={1} sx={{ mt: 2, flexWrap: 'wrap', rowGap: 1 }}>
        {children}
      </Stack>
    )}
  </Box>
)

export default HelixOrgOverviewCard
