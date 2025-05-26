import React, { FC, useMemo, useCallback, useState, useEffect } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import GitHubIcon from '@mui/icons-material/GitHub'
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
  APP_SOURCE_GITHUB,
  APP_SOURCE_HELIX,
} from '../../types'

import {
  getAppImage,
  getAppAvatar,
  getAppName,
  getAppDescription,
} from '../../utils/apps'

// Import the Helix icon
import HelixIcon from '../../../assets/img/logo.png'

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
      const gptScripts = assistant?.gptscripts || []
      const apiTools = (assistant?.tools || []).filter(t => t.tool_type == 'api')      

      

      // Get usage data for this app with proper typing
      const appUsage = usageData[app.id] || []
      // const last7DaysData = appUsage?.slice(-7) || []
      const usageValues = appUsage.map((day: TypesAggregatedUsageMetric) => day.total_tokens || 0)
      const usageLabels = appUsage.map((day: TypesAggregatedUsageMetric) => {
        const date = new Date(day.date)
        return date.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' }) // e.g., "Mon 3"
      })
      const todayTokens = appUsage[appUsage.length - 1]?.total_tokens || 0
      const totalTokens = appUsage.reduce((sum: number, day: TypesAggregatedUsageMetric) => sum + (day.total_tokens || 0), 0)

      const gptscriptsElem = gptScripts.length > 0 ? (
        <>
          <Box sx={{mb: 2}}>
            <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
              GPTScripts
            </Typography>
          </Box>
          {
            // TODO: support more than 1 assistant
            gptScripts.map((gptscript, index) => {
              return (
                <Box
                  key={index}
                >
                  <Row>
                    <Cell sx={{width:'50%'}}>
                      <Chip color="secondary" size="small" label={gptscript.name} />
                    </Cell>
                    <Cell sx={{width:'50%'}}>
                      <Typography variant="body2" sx={{color: '#999', fontSize: '0.8rem'}}>
                        {gptscript.content?.split('\n').filter(r => r)[0] || ''}
                      </Typography>
                      <Typography variant="body2" sx={{color: '#999', fontSize: '0.8rem'}}>
                        {gptscript.content?.split('\n').filter(r => r)[1] || ''}
                      </Typography>
                      <JsonWindowLink
                        sx={{textDecoration: 'underline'}}
                        data={gptscript.content}
                      >
                        expand
                      </JsonWindowLink>
                    </Cell>
                  </Row>
                </Box>
              )
            })
          }
        </>
      ) : null

      const apisElem = apiTools.length > 0 ? (
        <>
          {
            // TODO: support more than 1 assistant
            apiTools.map((apiTool, index) => {
              return (
                <ToolDetail
                  key={ index }
                  tool={ apiTool }
                />
              )
            })
          }
        </>
      ) : null

      // TODO: add the list of apis also to this display
      // we can grab stuff from the tools package that already renders endpoints    

      return {
        id: app.id,
        _data: app,
        name: (
          <Row>
            <Cell sx={{pr: 2,}}>
              {app.app_source === APP_SOURCE_GITHUB ? (
                <GitHubIcon />
              ) : (
                <img src={HelixIcon} alt="Helix" style={{ width: '24px', height: '24px' }} />
              )}
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
              <Typography variant="caption">
                { app.config.github?.repo }
              </Typography>
            </Cell>
          </Row>
        ),
        actions: (
          <>
            { gptscriptsElem }
            { apisElem }
          </>
        ),
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
      <Box
        sx={{
          width: '100%',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'flex-end',
          justifyContent: 'flex-end',
          pl: 2,
          pr: 2,
        }}
      >
        <ClickLink
          sx={{mr:2}}
          onClick={ () => {
            onDelete(app._data)
          }}
        >
          <Tooltip title="Delete">
            <DeleteIcon />
          </Tooltip>
        </ClickLink>
      
        <ClickLink
          onClick={ () => {
            onEdit(app._data)
          }}
        >
          <Tooltip title="Edit">
            <EditIcon />
          </Tooltip>
        </ClickLink>
      </Box>
    )
  }, [
    onDelete,
    onEdit,
  ])

  return (
    <SimpleTable
      fields={[
      {
          name: 'name',
          title: 'Name',
      }, {
        name: 'actions',
        title: 'Skills',
      }, {
        name: 'usage',
        title: 'Token Usage',
      }]}
      data={ tableData }
      getActions={ getActions }
    />
  )
}

export default AppsDataGrid