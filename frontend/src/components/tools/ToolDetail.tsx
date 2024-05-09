import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Chip from '@mui/material/Chip'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  ITool,
} from '../../types'

const ToolDetail: FC<React.PropsWithChildren<{
  tool: ITool,
}>> = ({
  tool,
}) => {
  let details: any = ''
  if(tool.config.api) {
    details = (
      <>
        <Box sx={{mb: 2}}>
          <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
            { tool.config.api.url }
          </Typography>
          <Typography variant="caption" gutterBottom>
            { tool.description }
          </Typography>
        </Box>
        {
          tool.config.api.actions.map((action, index) => {
            return (
              <Box key={index}>
                <Row>
                  <Cell sx={{width:'50%'}}>
                    <Typography>
                      {action.name}
                    </Typography>
                  </Cell>
                  <Cell sx={{width:'50%'}}>
                    <Row>
                      <Cell sx={{width: '70px'}}>
                        <Chip color="secondary" size="small" label={action.method.toUpperCase()} />
                      </Cell>
                      <Cell>
                        <Typography>
                          {action.path}
                        </Typography>
                      </Cell>
                    </Row>
                  </Cell>
                </Row>
                <Row sx={{mt: 0.5, mb: 2}}>
                  <Cell>
                    <Typography variant="caption" sx={{color: '#999'}}>
                      {action.description}
                    </Typography>
                  </Cell>
                </Row>
              </Box>
            )
          })
        }
      </>
    )
  }
  if(tool.config.gptscript) {
    details = (
      <>
        <Box sx={{mb: 2}}>
          {
            tool.config.gptscript.script_url && (
              <Typography variant="body1" gutterBottom sx={{fontWeight: 'bold', textDecoration: 'underline'}}>
                { tool.config.gptscript.script_url }
              </Typography>
            )
          }
          <Typography variant="caption" gutterBottom>
            { tool.description }
          </Typography>
        </Box>
      </>
    )
  }

  return details
}

export default ToolDetail