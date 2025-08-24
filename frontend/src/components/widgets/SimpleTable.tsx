import React, { FC } from 'react'

import Box from '@mui/material/Box'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TableContainer from '@mui/material/TableContainer'
import Paper from '@mui/material/Paper'
import CircularProgress from '@mui/material/CircularProgress'

export interface ITableField {
  name: string,
  title: string,
  numeric?: boolean,
  style?: React.CSSProperties,
  className?: string,
}

const SimpleTable: FC<{
  authenticated: boolean,
  fields: ITableField[],
  data: Record<string, any>[],
  compact?: boolean,
  withContainer?: boolean,
  hideHeader?: boolean,
  hideHeaderIfEmpty?: boolean,
  actionsTitle?: string,
  actionsFieldClassname?: string,
  loading?: boolean,
  onRowClick?: {
    (row: Record<string, any>): void,
  },
  getActions?: {
    (row: Record<string, any>): JSX.Element,
  },
}> = ({
  authenticated = true,
  fields,
  data,
  compact = false,
  withContainer = false,
  hideHeader = false,
  hideHeaderIfEmpty = false,
  actionsTitle = 'Actions',
  actionsFieldClassname,
  loading = false,
  onRowClick,
  getActions, 
}) => {

  const table = (
    <Table size={ compact ? 'small' : 'medium' }>
      {
        (!hideHeader && (!hideHeaderIfEmpty || data.length > 0)) && (
          <TableHead>
            <TableRow>
              {
                fields.map((field, i) => {
                  return (
                    <TableCell key={ i } align={ field.numeric ? 'right' : 'left' }>
                      { field.title }
                    </TableCell>
                  )
                })
              }
              {
                getActions ? (
                  <TableCell align='right'>
                    { actionsTitle }
                  </TableCell>
                ) : null
              }
            </TableRow>
          </TableHead>
        )
      }
      <TableBody>
        {loading ? (
          <TableRow>
            <TableCell colSpan={fields.length + (getActions ? 1 : 0)} align="center" sx={{ py: 3 }}>
              <CircularProgress size={24} />
            </TableCell>
          </TableRow>
        ) : !authenticated ? (
          <TableRow>            
            <TableCell colSpan={fields.length + (getActions ? 1 : 0)} align="center" sx={{ py: 3 }}>
              Login to view your agents
            </TableCell>
          </TableRow>          
        ) : data.length === 0 ? (
          <TableRow>
            <TableCell colSpan={fields.length + (getActions ? 1 : 0)} align="center" sx={{ py: 3 }}>
              No data to display
            </TableCell>
          </TableRow>
        ) : data.map((dataRow, i) => {
          return (
            <TableRow
              hover
              onClick={e => {
                if(!onRowClick) return
                onRowClick(dataRow)
              }}
              tabIndex={-1}
              key={ i }
            >
              {
                fields.map((field, i) => {
                  return (
                    <TableCell 
                      key={ i } 
                      align={ field.numeric ? 'right' : 'left' } 
                      className={ field.className }
                      style={ field.style }
                    >
                      { dataRow[field.name] }
                    </TableCell>
                  )
                })
              }
              {
                getActions ? (
                  <TableCell align='right' className={ actionsFieldClassname || '' }>
                    { getActions(dataRow) }
                  </TableCell>
                ) : null
              }
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  )

  const renderTable = withContainer ? (
    <TableContainer component={Paper}>
      { table }
    </TableContainer>
  ) : table
  
  return (
    <Box component="div" sx={{ width: '100%' }}>
      <Box component="div" sx={{ overflowX: 'auto' }}>
        { renderTable }
      </Box>
    </Box>
  )
}

export default SimpleTable