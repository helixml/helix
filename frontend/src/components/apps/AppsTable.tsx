import React, { FC, useMemo, useCallback, useState, useEffect } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import GitHubIcon from '@mui/icons-material/GitHub'
import SchoolIcon from '@mui/icons-material/School'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import { LineChart } from '@mui/x-charts';

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import JsonWindowLink from '../widgets/JsonWindowLink'
import ToolDetail from '../tools/ToolDetail'

import useTheme from '@mui/material/styles/useTheme'
import useApi from '../../hooks/useApi'

import {
  IApp,
} from '../../types'

import {  
  getAppName,  
} from '../../utils/apps'

// Import the Helix icon
import HelixIcon from '../../../assets/img/logo.png'
import useLightTheme from '../../hooks/useLightTheme'

interface TypesAggregatedUsageMetric {
  date: string;
  total_tokens: number;
  // Add other fields if they exist in the API type
}

const AppsDataGrid: FC<React.PropsWithChildren<{
  data: IApp[],
  onEdit: (app: IApp) => void,
  onDelete: (app: IApp) => void,
}>> = ({
  data,
  onEdit,
  onDelete,
}) => {

  const api = useApi()
  const apiClient = api.getApiClient()
  const theme = useTheme()  
  // Add state for usage data with proper typing
  const [usageData, setUsageData] = useState<{[key: string]: TypesAggregatedUsageMetric[] | null}>({})
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [currentApp, setCurrentApp] = useState<IApp | null>(null);

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

  // Fetch usage data for all apps
  useEffect(() => {
    const fetchUsageData = async () => {
      const usagePromises = data.map(app => 
        apiClient.v1AppsDailyUsageDetail(app.id)
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
        const date = new Date(day.date)
        return date.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' }) // e.g., "Mon 3"
      })      

      const skills = assistant?.tools && assistant.tools.length > 0 ? (
        <>
          {
            // TODO: support more than 1 assistant
            assistant.tools.map((apiTool, index) => {
              return (
                <Chip
                  key={index}
                  label={apiTool.name || 'Unknown Tool'}
                  size="small"
                  sx={{
                    mr: 0.5,
                    mb: 0.5,
                    background: `linear-gradient(135deg, ${theme.chartGradientStart} 0%, ${theme.chartGradientEnd} 100%)`,
                    color: theme.palette.mode === 'dark' ? '#fff' : '#333',
                    border: `1px solid ${theme.palette.mode === 'dark' ? '#444' : '#ddd'}`,
                    boxShadow: theme.palette.mode === 'dark' 
                      ? '0 2px 4px rgba(0, 0, 0, 0.3)' 
                      : '0 2px 4px rgba(0, 0, 0, 0.1)',
                    '&:hover': {
                      background: `linear-gradient(135deg, ${theme.chartGradientEnd} 0%, ${theme.chartGradientStart} 100%)`,
                      boxShadow: theme.palette.mode === 'dark' 
                        ? '0 4px 8px rgba(0, 0, 0, 0.4)' 
                        : '0 4px 8px rgba(0, 0, 0, 0.15)',
                      transform: 'translateY(-1px)',
                    },
                    transition: 'all 0.2s ease-in-out',
                  }}
                />
              )
            })
          }
        </>
      ) : null

      return {
        id: app.id,
        _data: app,
        name: (
          <Row>
            <Cell sx={{pr: 2,}}>
              <img src={HelixIcon} alt="Helix" style={{ width: '24px', height: '24px' }} />
            </Cell>
            <Cell grow>
              <Typography variant="body1">                
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
            </Cell>
          </Row>
        ),
        skills: skills,
        usage: (
          <Box sx={{ width: 200, height: 50 }}>
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
                    <stop offset="0%" stopColor="#00c8ff" stopOpacity={0.5} />
                    <stop offset="100%" stopColor="#070714" stopOpacity={0.1} />
                  </linearGradient>
                </defs>
              </LineChart>
            </Box>            
          </Box>
        ),
      }
    })
  }, [
    theme,
    data,
    usageData
  ])

  const getActions = useCallback((app: any) => {
    return (
      <IconButton
        aria-label="more"
        aria-controls="long-menu"
        aria-haspopup="true"
        onClick={(e) => handleMenuClick(e, app)}
      >
        <MoreVertIcon />
      </IconButton>
    )
  }, [])

  return (
    <>
      <SimpleTable
        fields={[
        {
            name: 'name',
            title: 'Name',
        }, {
          name: 'skills',
          title: 'Skills',
        }, {
          name: 'usage',
          title: 'Token Usage',
        }]}
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
          Edit
        </MenuItem>
        <MenuItem onClick={handleDelete}>          
          Delete
        </MenuItem>
      </Menu>
    </>
  )
}

export default AppsDataGrid