import React, { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import LinearProgress from '@mui/material/LinearProgress'
import useTheme from '@mui/material/styles/useTheme'

import SimpleTable from '../widgets/SimpleTable'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import { useGetQuota } from '../../services/quotaService'

interface QuotaListViewProps {
  orgId?: string
}

const QuotaListView: FC<QuotaListViewProps> = ({ orgId }) => {
  const theme = useTheme()
  const { data: quotas, isLoading } = useGetQuota(orgId)

  const tableData = useMemo(() => {
    if (!quotas) return []

    const items = [
      {
        label: 'Concurrent Desktops',
        used: quotas.active_concurrent_desktops ?? 0,
        max: quotas.max_concurrent_desktops ?? 0,
      },
      {
        label: 'Projects',
        used: quotas.projects ?? 0,
        max: quotas.max_projects ?? 0,
      },
      {
        label: 'Repositories',
        used: quotas.repositories ?? 0,
        max: quotas.max_repositories ?? 0,
      },
      {
        label: 'Spec Tasks',
        used: quotas.spec_tasks ?? 0,
        max: quotas.max_spec_tasks ?? 0,
      },
    ]

    return items.map((item) => {
      const isUnlimited = item.max === -1
      const percentage = isUnlimited ? 0 : item.max > 0 ? (item.used / item.max) * 100 : 0
      const isNearLimit = !isUnlimited && percentage >= 80
      const isAtLimit = !isUnlimited && percentage >= 100

      const progressColor = isAtLimit
        ? theme.palette.error.main
        : isNearLimit
          ? theme.palette.warning.main
          : theme.palette.primary.main

      return {
        id: item.label,
        name: (
          <Row>
            <Cell grow>
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 500,
                  color: theme.palette.text.primary,
                }}
              >
                {item.label}
              </Typography>
            </Cell>
          </Row>
        ),
        usage: (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, minWidth: 200 }}>
            <Box sx={{ flexGrow: 1 }}>
              <LinearProgress
                variant="determinate"
                value={isUnlimited ? 0 : Math.min(percentage, 100)}
                sx={{
                  height: 8,
                  borderRadius: 4,
                  backgroundColor: theme.palette.mode === 'dark'
                    ? 'rgba(255, 255, 255, 0.08)'
                    : 'rgba(0, 0, 0, 0.08)',
                  '& .MuiLinearProgress-bar': {
                    borderRadius: 4,
                    backgroundColor: progressColor,
                  },
                }}
              />
            </Box>
            <Typography
              variant="body2"
              sx={{
                minWidth: 60,
                textAlign: 'right',
                color: isAtLimit
                  ? theme.palette.error.main
                  : isNearLimit
                    ? theme.palette.warning.main
                    : theme.palette.text.secondary,
                fontWeight: isNearLimit || isAtLimit ? 600 : 400,
              }}
            >
              {item.used} / {isUnlimited ? '∞' : item.max}
            </Typography>
          </Box>
        ),
      }
    })
  }, [quotas, theme])

  const tableFields = useMemo(() => [
    { name: 'name', title: 'Name' },
    { name: 'usage', title: 'Usage' },
  ], [])

  if (!quotas && !isLoading) return null

  return (
    <SimpleTable
      authenticated
      fields={tableFields}
      data={tableData}
      loading={isLoading}
      hideHeader={false}
    />
  )
}

export default QuotaListView
