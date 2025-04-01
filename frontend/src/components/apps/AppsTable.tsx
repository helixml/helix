import React, { FC, useMemo, useCallback, useState, useEffect } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import GitHubIcon from '@mui/icons-material/GitHub'
import { SparkLineChart } from '@mui/x-charts';

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
      const todayTokens = appUsage[appUsage.length - 1]?.total_tokens || 0
      const totalTokens = appUsage.reduce((sum: number, day: TypesAggregatedUsageMetric) => sum + (day.total_tokens || 0), 0)

      console.log(usageValues)

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

      const accessType = app.global ? 'Global' : 'Private'

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
        type: (
          <Typography variant="body1">
            {accessType}
          </Typography>
        ),
        actions: (
          <>
            { gptscriptsElem }
            { apisElem }
          </>
        ),
        updated: (
          <Box
            sx={{
              fontSize: '0.9em',
            }}
          >
            { new Date(app.updated).toLocaleString() }
          </Box>
        ),
        usage: (
          <Box sx={{ width: 200, height: 50 }}>
            <Tooltip
              title={
                <Box>
                  <Typography variant="body2">Last 7 Days Usage:</Typography>
                  {appUsage.map((day: TypesAggregatedUsageMetric, i: number) => (
                    <Typography key={i} variant="caption" component="div">
                      {new Date(day.date).toLocaleDateString()}: {day.total_tokens || 0} tokens
                    </Typography>
                  ))}
                  <Typography variant="body2" sx={{ mt: 1 }}>
                    Today: {todayTokens} tokens
                  </Typography>
                  <Typography variant="body2">
                    Total (7 days): {totalTokens} tokens
                  </Typography>
                </Box>
              }
            >
              <Box>
                <SparkLineChart
                  data={usageValues}
                  height={50}
                  width={200}
                  showTooltip={true}
                  curve="linear"
                />
              </Box>
            </Tooltip>
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
        name: 'type',
        title: 'Access Type',
      }, {
        name: 'updated',
        title: 'Updated At',
      }, {
        name: 'actions',
        title: 'Actions',
      }, {
        name: 'usage',
        title: 'Usage',
      }]}
      data={ tableData }
      getActions={ getActions }
    />
  )
}

export default AppsDataGrid