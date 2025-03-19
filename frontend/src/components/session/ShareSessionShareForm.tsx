import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import { FC } from 'react'
import useSnackbar from '../../hooks/useSnackbar'
import Cell from '../widgets/Cell'
import Row from '../widgets/Row'

import {
  ISession,
} from '../../types'

export const ShareSessionShareForm: FC<{
  session: ISession,
}> = ({
  session,
}) => {
    const snackbar = useSnackbar()
    const url = `${window.location.protocol}//${window.location.hostname}/session/${session.id}`

    const handleCopy = () => {
      navigator.clipboard.writeText(url)
        .then(() => {
          snackbar.success('Copied to clipboard')
        })
        .catch((error) => {
          console.error('Failed to copy:', error)
          snackbar.error('Failed to copy to clipboard')
        })
    }

    return (
      <Box
        sx={{
          p: 1,
        }}
      >
        {
          (
            <Row>
              <Cell
                sx={{
                  pr: 0.5,
                  pb: 2.5,
                }}
                flexGrow={1}
              >
                <Typography variant="h6" sx={{
                  backgroundColor: '#000',
                  p: 2,
                  border: '1px solid #999',
                  borderRadius: 5,
                }}>
                  {url}
                </Typography>
              </Cell>
              <Cell
                sx={{
                  ml: 0.5,
                  pb: 3,
                }}
              >
                <Button
                  variant="outlined"
                  color="primary"
                  onClick={handleCopy}
                >
                  Copy URL
                </Button>
              </Cell>
            </Row>
          )
        }
      </Box>
    )
  }

export default ShareSessionShareForm