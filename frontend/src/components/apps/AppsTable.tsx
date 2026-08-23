import React, { FC, useMemo, useCallback, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import { LineChart } from '@mui/x-charts';

import { Copy, Edit, EllipsisVertical, Trash } from 'lucide-react'

import SimpleTable from '../widgets/SimpleTable'
import useTheme from '@mui/material/styles/useTheme'
import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'

import {
  IApp,
} from '../../types'

import {
  TypesAggregatedUsageMetric
} from '../../api/api'

import {  
  getAppName,
} from '../../utils/apps'

// Import DuplicateDialog
import DuplicateDialog from '../app/DuplicateDialog'
import AgentHarness, { getAgentHarnessRuntime } from '../agent/AgentHarness'

const AppsDataGrid: FC<React.PropsWithChildren<{
  authenticated: boolean,
  data: IApp[],
  onEdit: (app: IApp) => void,
  onDelete: (app: IApp) => void,
  orgId: string,
}>> = ({
  authenticated,
  data,
  onEdit,
  onDelete,
  orgId,
}) => {

  const api = useApi()
  const apiClient = api.getApiClient()
  const theme = useTheme()
  const account = useAccount()
  // Add state for usage data with proper typing
  const [usageData, setUsageData] = useState<{[key: string]: TypesAggregatedUsageMetric[] | null}>({})
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [currentApp, setCurrentApp] = useState<IApp | null>(null);

  const [duplicateDialogOpen, setDuplicateDialogOpen] = useState(false);
  const [duplicatingApp, setDuplicatingApp] = useState<IApp | null>(null);

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>, app: any) => {
    setAnchorEl(event.currentTarget);
    setCurrentApp(app._data as IApp);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
    setCurrentApp(null);
  };

  const handleEdit = () => {
    if (currentApp) {
      onEdit(currentApp);
    }
    handleMenuClose();
  };

  const handleDelete = () => {
    if (currentApp) {
      onDelete(currentApp);
    }
    handleMenuClose();
  };

  const handleDuplicate = (app: IApp) => {
    setDuplicatingApp(app);
    setDuplicateDialogOpen(true);
    handleMenuClose();
  };

  const handleDuplicateDialogClose = () => {
    setDuplicateDialogOpen(false);
    setDuplicatingApp(null);
  };

  // Handle usage click
  const handleUsageClick = (app: IApp) => {
    account.orgNavigate('agent', { app_id: app.id }, { tab: 'usage' });
  };

  // Fetch usage data for all apps
  useEffect(() => {
    const fetchUsageData = async () => {
      const usagePromises = data.map(app => 
        apiClient.v1AgentsDailyUsageDetail(app.id)
          .then(response => ({ [app.id]: response.data as TypesAggregatedUsageMetric[] }))
          .catch(() => ({ [app.id]: null }))
      )
      const results = await Promise.all(usagePromises)
      const combinedData = results.reduce((acc, curr) => ({ ...acc, ...curr }), {} as {[key: string]: TypesAggregatedUsageMetric[] | null})
      setUsageData(combinedData)
    }
    fetchUsageData()
  }, [data])

  const tableData = useMemo(() => {
    return data.map(app => {
      const assistant = app.config.helix?.assistants?.[0]      

      // Get usage data for this app with proper typing
      const appUsage = usageData[app.id] || []
      // const last7DaysData = appUsage?.slice(-7) || []
      const usageValues = appUsage.map((day: TypesAggregatedUsageMetric) => day.total_tokens || 0)
      const usageLabels = appUsage.map((day: TypesAggregatedUsageMetric) => {
        const date = new Date(day.date || '')
        return date.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' }) // e.g., "Mon 3"
      })      

      const description = app.config.helix?.description || ''
      const model = assistant?.code_agent_runtime === 'claude_code'
        && assistant.code_agent_credential_type === 'subscription'
        ? assistant.claude_subscription_model || 'Default'
        : assistant?.model || assistant?.generation_model || 'Default'

      const baseRow = {
        id: app.id,
        _data: app,
        name: (
          <Box>
            <Typography variant="body1" component="div">
              <a
                style={{
                  textDecoration: 'none',
                  fontWeight: 'bold',
                  color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
                }}
                href="#"
                onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
                  e.preventDefault()
                  e.stopPropagation()
                  onEdit(app)
                }}
              >
                { getAppName(app) }
              </a>
            </Typography>
            {description && (
              <Typography
                variant="caption"
                sx={{
                  color: theme.palette.text.secondary,
                  display: 'block',
                  mt: 0.25,
                }}
              >
                {description}
              </Typography>
            )}
          </Box>
        ),
        // Only agents that actually run a harness get one here — a native chat
        // agent has no code_agent_runtime, and getAgentHarnessRuntime's
        // zed_agent fallback would otherwise label it as one.
        harness: assistant?.code_agent_runtime ? (
          <AgentHarness runtime={getAgentHarnessRuntime(app)} variant="long" size={16} />
        ) : null,
        model: (
          <Typography variant="body2" color="text.secondary">
            {model}
          </Typography>
        ),
        usage: (
          <Box 
            sx={{ 
              width: 200, 
              height: 50,
              cursor: 'pointer',
              '&:hover': {
                opacity: 0.8,
              },
            }}
            onClick={() => handleUsageClick(app)}
          >
            <Box>
              <LineChart
                xAxis={[
                  {
                    data: usageLabels,
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
                  data: usageValues,
                  area: true,
                  showMark: false,
                  color: '#00c8ff',
                }]}
                height={50}
                width={200}
                slotProps={{
                  legend: { hidden: true },
                }}
                sx={{
                  '& .MuiAreaElement-root': {
                    fill: 'url(#usageGradient)',
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
                }}
                grid={{ horizontal: false, vertical: false }}
                disableAxisListener
                margin={{ top: 0, bottom: 0, left: 0, right: 0 }}
              >
                <defs>
                  <linearGradient id="usageGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={theme.chartGradientStart} stopOpacity={theme.chartGradientStartOpacity} />
                    <stop offset="100%" stopColor={theme.chartGradientEnd} stopOpacity={theme.chartGradientEndOpacity} />
                  </linearGradient>
                </defs>
              </LineChart>
            </Box>            
          </Box>
        ),
      }

      return baseRow
    })
  }, [
    theme,
    data,
    usageData,
  ])

  const getActions = useCallback((app: any) => {
    return (
      <IconButton
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={(e) => handleMenuClick(e, app)}
      >
        <EllipsisVertical size={18} />
      </IconButton>
    )
  }, [])

  // One list for every agent. A chat agent has no harness, so that cell just
  // renders empty rather than earning the list a second column set.
  const tableFields = useMemo(() => {
    return [
      {
        name: 'name',
        title: 'Name',
      },
      {
        name: 'harness',
        title: 'Harness',
      },
      {
        name: 'model',
        title: 'Model',
      },
      {
        name: 'usage',
        title: 'Token Usage',
      },
    ]
  }, [])

  return (
    <>
      <SimpleTable
        authenticated={ authenticated }
        fields={tableFields}
        data={ tableData }
        getActions={ getActions }
      />
      <Menu
        id="long-menu"
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}        
      >
        <MenuItem onClick={handleEdit}>
          <Edit size={16} style={{
            marginRight: 5,
          }} /> 
          Edit
        </MenuItem>
        <MenuItem onClick={() => handleDuplicate(currentApp as IApp)}>
          <Copy size={16} style={{
            marginRight: 5,
          }} />
          Duplicate
        </MenuItem>
        <MenuItem onClick={handleDelete}>          
          <Trash size={16} style={{
            marginRight: 5,
          }} />
          Delete
        </MenuItem>
      </Menu>
      <DuplicateDialog
        open={duplicateDialogOpen}
        onClose={handleDuplicateDialogClose}
        app={duplicatingApp}
        orgId={orgId}
      />
    </>
  )
}

export default AppsDataGrid
