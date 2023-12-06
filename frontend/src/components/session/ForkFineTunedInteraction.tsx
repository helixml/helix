import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import EditIcon from '@mui/icons-material/Edit'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

export const ForkFineTunedInteration: FC<{
  
}> = ({
  
}) => {

  return (
    <Row>
      <Cell grow>
        <Typography gutterBottom>
          You have completed a fine tuning session on these documents.
        </Typography>
        <Typography gutterBottom>
          You can "Clone" from this point in time to create a new session and continue training from here.
        </Typography>
      </Cell>
      <Cell>
        <Button
          variant="contained"
          color="primary"
          sx={{
            mr: 2,
          }}
          endIcon={<EditIcon />}
          onClick={ () => {} }
        >
          Edit Questions
        </Button>
        <Button
          variant="contained"
          color="secondary"
          endIcon={<NavigateNextIcon />}
          onClick={ () => {} }
        >
          Start Training
        </Button>
      </Cell>

    </Row>
  )  
}

export default ForkFineTunedInteration