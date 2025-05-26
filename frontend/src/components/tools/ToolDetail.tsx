import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Chip from '@mui/material/Chip'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import Tooltip from '@mui/material/Tooltip'


import {
  ITool,
} from '../../types'
import useLightTheme from '../../hooks/useLightTheme'

const ToolDetail: FC<React.PropsWithChildren<{
  tool: ITool,
}>> = ({
  tool,
}) => {
  const lightTheme = useLightTheme()
  let details: any = ''
  if(tool.config.api) {
    details = (
      <Box sx={{ border: '1px solid #757575', borderRadius: 2, p: 2, mb: 1 }}>
        <Box sx={{mb: 0, mt: 0}}>          
          <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', color: lightTheme.textColorFaded}}>
            API - { tool.config.api.url }
          </Typography>
        </Box>
        <Box component="ul" sx={{ listStyle: 'disc', pl: 2, mt: 1 }}>
          {
            tool.config.api.actions?.map((action, index) => {
              return (
                <Tooltip 
                  key={index}
                  title={action.description || ''}
                >
                  <Box component="li">
                    <Typography sx={{ color: lightTheme.textColorFaded }}>
                      {action.name}
                    </Typography>
                  </Box>
                </Tooltip>
              )
            })
          }
        </Box>
      </Box>
    )
  }
  if(tool.config.gptscript) {
    details = (
      <Box sx={{ border: '1px solid #757575', borderRadius: 2, p: 2, mb: 2 }}>
        <Box sx={{mb: 2}}>
          {
            tool.config.gptscript.script_url && (
              <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline', color: 'rgba(255, 255, 255, 0.7)'}}>
                { tool.config.gptscript.script_url }
              </Typography>
            )
          }
          <Typography variant="caption" gutterBottom sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>
            { tool.description }
          </Typography>
        </Box>
      </Box>
    )
  }

  return details
}

export default ToolDetail