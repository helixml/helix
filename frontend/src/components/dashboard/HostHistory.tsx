import React, { useState } from 'react'
import { Alert, Box, Card, CardContent, MenuItem, Stack, TextField, Typography, useTheme } from '@mui/material'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useHostHistory } from '../../services/systemSettingsService'

export default function HostHistory() {
  const [days, setDays] = useState(1)
  const { data, isLoading, isError } = useHostHistory(days)
  const theme = useTheme()
  const date = (timestamp: number) => new Date(timestamp * 1000)
  const colors = [theme.palette.primary.main, theme.palette.success.main, theme.palette.warning.main]
  const points = (data ?? []).flatMap((sample, i, samples) => {
    const previous = samples[i - 1]?.timestamp
    return previous && sample.timestamp && sample.timestamp - previous > 600
      ? [{ timestamp: previous + 300 }, sample] : [sample]
  })
  return <Card><CardContent>
    <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
      <Typography variant="h6">Host resources</Typography>
      <TextField select label="Time range" size="small" value={days} onChange={e => setDays(Number(e.target.value))}>
        {[1, 7, 30].map(n => <MenuItem key={n} value={n}>{n === 1 ? '24 hours' : n + ' days'}</MenuItem>)}
      </TextField>
    </Stack>
    <Typography variant="body2" color="text.secondary" sx={{ my: 2 }}>
      API host CPU and memory usage, and space used on the local filestore filesystem. Sampled every five minutes.
    </Typography>
    {isError ? <Alert severity="error">Could not load host history.</Alert> : isLoading ? 'Loading host history…'
      : !points.length ? 'No samples in this period.' : <Box height={320}>
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={points}>
            <CartesianGrid stroke={theme.palette.divider} />
            <XAxis type="number" dataKey="timestamp" domain={['dataMin','dataMax']} tickFormatter={t => days === 1 ? date(t).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}) : date(t).toLocaleDateString()} />
            <YAxis domain={[0, 100]} unit="%" />
            <Tooltip labelFormatter={t => date(Number(t)).toLocaleString()} formatter={value => Number(value).toFixed(1) + '%'} />
            <Legend />
            {['cpu','memory','disk'].map((key,i) => <Line key={key} name={['CPU','Memory','Disk'][i]} dataKey={key}
              stroke={colors[i]} dot={points.length === 1} isAnimationActive={false} />)}
          </LineChart>
        </ResponsiveContainer>
      </Box>}
  </CardContent></Card>
}
