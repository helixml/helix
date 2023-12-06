import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import EditIcon from '@mui/icons-material/Edit'
import FileCopyIcon from '@mui/icons-material/FileCopy'
import ViewIcon from '@mui/icons-material/Visibility'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ConversationEditor from './ConversationEditor'
import useInteractionQuestions from '../../hooks/useInteractionQuestions'

export const ForkFineTunedInteraction: FC<{
  
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
          endIcon={<ViewIcon />}
          onClick={ () => {} }
        >
          View Questions
        </Button>
        <Button
          variant="contained"
          color="secondary"
          endIcon={<FileCopyIcon />}
          onClick={ () => {} }
        >
          Clone From Here
        </Button>
      </Cell>

    </Row>
  )  
}

export default ForkFineTunedInteraction