import React, { FC, useMemo } from 'react'
import DataGrid2, { IDataGrid2_Column } from './DataGrid'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import DeleteIcon from '@mui/icons-material/Delete'
import CodeIcon from '@mui/icons-material/Code'

import {
  IApiKey,
} from '../../types'
import useSnackbar from '../../hooks/useSnackbar'

const AppAPIKeysDataGrid: FC<React.PropsWithChildren<{
  data: IApiKey[],
  onDeleteKey: (key: string) => void,
}>> = ({
  data,
  onDeleteKey,
}) => {
  const snackbar = useSnackbar()
  const columns = useMemo<IDataGrid2_Column<IApiKey>[]>(() => {
    return [
      {
        name: 'name',
        header: 'Name',
        defaultFlex: 0,
        render: ({ data }) => {
          return data.name
        }
      },
      {
        name: 'key',
        header: 'Key',
        defaultFlex: 1,
        render: ({ data }) => {
          return (
            <Typography variant="caption">
              { data.key }
            </Typography>
          )
        }
      },
      {
        name: 'actions',
        header: '',
        defaultWidth: 120,
        sx: {
          textAlign: 'right',
        },
        render: ({ data }) => {
          return (
            <Box sx={{
              width: '100%',
              textAlign: 'right',
            }}>
              <CopyToClipboard
                text={ `<script src="https://cdn.jsdelivr.net/npm/@helixml/chat-embed"></script>
<script>
  ChatWidget({
    url: '${window.location.origin}/v1/chat/completions',
    model: 'mistral:7b-instruct',
    bearerToken: '${data.key}',
  })
</script>` }
                onCopy={ () => {
                  snackbar.success('embed code copied to clipboard')
                }}
              >
                <Tooltip title="Copy Embed Code">
                  <IconButton size="small">
                    <CodeIcon sx={{width: '16px', height: '16px'}} />
                  </IconButton>
                </Tooltip>
              </CopyToClipboard>
              <CopyToClipboard
                text={ data.key }
                onCopy={ () => {
                  snackbar.success('api key copied to clipboard')
                }}
              >
                <Tooltip title="Copy API Key">
                  <IconButton size="small" sx={{ml: 2}}>
                    <ContentCopyIcon sx={{width: '16px', height: '16px'}} />
                  </IconButton>
                </Tooltip>
              </CopyToClipboard>
              <Tooltip title="Delete API Key">
                <IconButton size="small" sx={{ml: 2}} onClick={() => {
                  onDeleteKey(data.key)
                }}>
                  <DeleteIcon sx={{width: '16px', height: '16px'}} />
                </IconButton>
              </Tooltip>
            </Box>
          )
        }
      },
    ]
  }, [
    onDeleteKey,
  ])

  return (
    <DataGrid2
      autoSort
      userSelect
      rows={ data }
      columns={ columns }
      rowHeight={ 70 }
      minHeight={ 300 }
      loading={ false }
    />
  )
}

export default AppAPIKeysDataGrid