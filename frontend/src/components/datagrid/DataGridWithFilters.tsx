import React, { FC } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

const DataGridWithFilters: FC<React.PropsWithChildren<{
  autoScroll?: boolean,
  filterWidth?: number,
  filters?: React.ReactNode,
  datagrid?: React.ReactNode,
  pagination?: React.ReactNode,
}>> = ({
  autoScroll = false,
  filterWidth = 300,
  filters,
  datagrid,
  pagination,
}) => {

  return (
    <Container
      disableGutters
      maxWidth="xl"
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'row',
        px: 1,
        pt: 1,
      }}
    >
      <Box
        className="data"
        sx={{
          flexGrow: 1,
          height: '100%',
          flexBasis: `calc(100% - ${filterWidth}px)`,
          display: 'flex',
          flexDirection: 'column',
          pr: 1,
          mr: 1,
        }}
      >
        <Box
          className="grid"
          sx={{
            display: 'flex',
            ...(autoScroll
              ? {
                  height: '1px',
                  flexGrow: 1,
                  overflowY: 'auto',
                  mb: 1,
                }
              : {
                  flexGrow: 1,
                  mb: 1,
                })
          }}
        >
          { datagrid }
        </Box>
        {
          pagination && (
            <Box
              className="pagination"
              sx={{
                flexGrow: 0,
                mt: 1,
                mb: 1,
              }}
            >
              { pagination }
            </Box>
          )
        }
      </Box>
      {
        filters && (
          <Box
            className="filters"
            sx={{
              flexGrow: 0,
              display: 'flex',
              flexDirection: 'column',
              justifyContent: 'flex-start',
              alignItems: 'center',
              width: `${filterWidth}px`,
              maxWidth: `${filterWidth}px`,
              minWidth: `${filterWidth}px`,
              height: '100%',
            }}
          >
            { filters }
          </Box>
        )
      }
    </Container>
  )
}

export default DataGridWithFilters