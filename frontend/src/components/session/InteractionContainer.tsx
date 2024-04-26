import React, { FC } from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Row from '../widgets/Row';
import Cell from '../widgets/Cell';
import { Button } from '@mui/material';

export const InteractionContainer: FC<{
   name: string,
  buttons?: React.ReactNode,
}> = ({
   name,
  buttons,
  children,
}) => {
  return (
    <Box sx={{ mb: 3 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
        <Button
          variant="contained"
          color="primary"
          size="small"
          sx={{
            textTransform: 'none',
            bgcolor: '#B4FDC0',
            color: 'black',
            fontWeight: 800,
            padding: '2px 8px',
            minWidth: 'auto',
            height: 'auto'
          }}
         >
          AI
        </Button>
        <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>
          Helix System
        </Typography>
      </Box>
      
      <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
        <Row>
          <Cell flexGrow={1}>
            {/* <Typography
              className="interactionName"
              variant="subtitle2"
              sx={{ color: '#aaa' }}
            >
              {name}
            </Typography> */}
          </Cell>
          <Cell>
            {buttons}
          </Cell>
        </Row>
      </Box>
      {children}
    </Box>
  );
};

export default InteractionContainer;