import React, { useMemo } from 'react'
import { Box, Tooltip } from '@mui/material'
import useTheme from '@mui/material/styles/useTheme'
import { LineChart } from '@mui/x-charts'
import { useSpecTaskUsage } from '../../services/specTaskService'
import { TypesAggregatedUsageMetric } from '../../api/api'

interface UsagePulseChartProps {
  taskId: string
  accentColor: string
}

const UsagePulseChart: React.FC<UsagePulseChartProps> = ({ taskId, accentColor }) => {
  const theme = useTheme()

  const { data: usageData, isLoading } = useSpecTaskUsage(taskId, {
    aggregationLevel: '5min',
    enabled: !!taskId,
    refetchInterval: 60000,
  })

  const { chartData, chartLabels, totalTokens } = useMemo(() => {
    if (!usageData || usageData.length === 0) return { chartData: null, chartLabels: [], totalTokens: 0 }
    const sortedData = [...usageData].sort((a, b) => 
      new Date(a.date || '').getTime() - new Date(b.date || '').getTime()
    )
    const data = sortedData.map((d: TypesAggregatedUsageMetric) => d.total_tokens || 0)
    const labels = sortedData.map((d: TypesAggregatedUsageMetric) => {
      const date = new Date(d.date || '')
      const month = String(date.getMonth() + 1).padStart(2, '0')
      const day = String(date.getDate()).padStart(2, '0')
      const hours = String(date.getHours()).padStart(2, '0')
      const minutes = String(date.getMinutes()).padStart(2, '0')
      return `${month}/${day} ${hours}:${minutes}`
    })
    const total = data.reduce((a, b) => a + b, 0)
    return { chartData: data, chartLabels: labels, totalTokens: total }
  }, [usageData])

  if (isLoading) {
    return (
      <Box sx={{ width: '100%', height: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Box sx={{ width: 16, height: 16, borderRadius: '50%', border: '2px solid', borderColor: accentColor, borderTopColor: 'transparent', animation: 'spin 1s linear infinite', '@keyframes spin': { '0%': { transform: 'rotate(0deg)' }, '100%': { transform: 'rotate(360deg)' } } }} />
      </Box>
    )
  }

  if (!chartData || chartData.length === 0 || totalTokens === 0) return null

  const gradientId = `usageGradient-${taskId}`

  return (
    <Tooltip title={`${totalTokens.toLocaleString()} tokens used`}>
      <Box
        sx={{
          width: '100%',
          height: 50,
          mt: 0.5,
          mb: 0.5,
        }}
      >
        <LineChart
          xAxis={[
            {
              data: chartLabels,
              scaleType: 'point',
              tickLabelStyle: { fontSize: 10, fill: '#aaa' },
              label: '',
            }
          ]}
          yAxis={[
            {
              tickLabelStyle: { display: 'none' },
              label: '',
            }
          ]}
          series={[{
            data: chartData,
            area: true,
            showMark: false,
            color: accentColor,
          }]}
          height={50}
          slotProps={{
            legend: { hidden: true },
          }}
          sx={{
            width: '100%',
            '& .MuiAreaElement-root': {
              fill: `url(#${gradientId})`,
            },
            '& .MuiMarkElement-root': {
              display: 'none',
            },
            '& .MuiLineElement-root': {
              strokeWidth: 2,
            },
            '& .MuiChartsAxis-line': {
              display: 'none',
            },
            '& .MuiChartsAxis-tick': {
              display: 'none',
            },
          }}
          grid={{ horizontal: false, vertical: false }}
          disableAxisListener
          margin={{ top: 0, bottom: 0, left: 0, right: 0 }}
        >
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={theme.chartGradientStart} stopOpacity={theme.chartGradientStartOpacity} />
              <stop offset="100%" stopColor={theme.chartGradientEnd} stopOpacity={theme.chartGradientEndOpacity} />
            </linearGradient>
          </defs>
        </LineChart>
      </Box>
    </Tooltip>
  )
}

export default React.memo(UsagePulseChart)
