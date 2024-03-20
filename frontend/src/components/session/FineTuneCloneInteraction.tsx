import React, { FC, useState, useEffect, useCallback } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'

import AddIcon from '@mui/icons-material/Add'
import FileCopyIcon from '@mui/icons-material/FileCopy'
import ViewIcon from '@mui/icons-material/Visibility'
import WrapTextIcon from '@mui/icons-material/WrapText';
import TextsmsIcon from '@mui/icons-material/Textsms';

import TextureIcon from '@mui/icons-material/Texture';


import Window from '../widgets/Window'
import FineTuneTextQuestionEditor from './FineTuneTextQuestionEditor'

import useInteractionQuestions from '../../hooks/useInteractionQuestions'

import {
  ISessionType,
  ICloneInteractionMode,
  CLONE_INTERACTION_MODE_JUST_DATA,
  CLONE_INTERACTION_MODE_WITH_QUESTIONS,
  CLONE_INTERACTION_MODE_ALL,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

export const FineTuneCloneInteraction: FC<{
  type: ISessionType,
  sessionID: string,
  userInteractionID: string,
  systemInteractionID: string,
  onClone: (mode: ICloneInteractionMode, interactionID: string) => Promise<boolean>,
  onAddDocuments?: () => void,
}> = ({
  type,
  sessionID,
  userInteractionID,
  systemInteractionID,
  onClone,
  onAddDocuments,
}) => {
  const interactionQuestions = useInteractionQuestions();
  const [viewMode, setViewMode] = useState(false);
  const [cloneMode, setCloneMode] = useState(false);
  const [selectedCloneMode, setSelectedCloneMode] = useState<ICloneInteractionMode | null>(null);

  const colSize = type === SESSION_TYPE_IMAGE ? 6 : 4;

  const handleClone = useCallback(async (mode: ICloneInteractionMode, interactionID: string) => {
    const result = await onClone(mode, interactionID);
    if (!result) return;
    setCloneMode(false);
  }, [onClone]);

  const handleCardClick = (mode: ICloneInteractionMode) => {
    setSelectedCloneMode(mode);
  };

  const handleCloneSelectedMode = useCallback(async () => {
    if (selectedCloneMode && systemInteractionID) {
      const result = await onClone(selectedCloneMode, systemInteractionID);
      if (result) {
        setCloneMode(false); // Close the clone mode if successful
        setSelectedCloneMode(null); // Reset the selected clone mode
      }
    }
  }, [selectedCloneMode, systemInteractionID, onClone]);

  useEffect(() => {
    if (!viewMode) {
      interactionQuestions.setQuestions([]);
      return;
    }
    interactionQuestions.loadQuestions(sessionID, userInteractionID);
  }, [viewMode, sessionID, userInteractionID]);

  return (
    <>
     
  <Grid item sm={12} md={6}>
    <Typography>
      You have completed a fine tuning session on these {type === SESSION_TYPE_IMAGE ? 'images' : 'documents'}.
    </Typography>
    <Typography>
      You can now chat to your model, add some more documents and re-train or you can "Clone" from this point in time.
    </Typography>
   </Grid>
   <Grid item sm={12} md={6} sx={{ mt: 2 }}>
    {/* If you want the buttons to be under the text, they should be in their own Grid item */}
    {onAddDocuments && (
      <Button
        variant='contained'
        size="small"
        sx={{ mb: 1, mr: 1, textTransform: 'none', bgcolor: '#3BF959', color: 'black', fontWeight: 800 }}
        onClick={onAddDocuments}
      >
        Add more {type === SESSION_TYPE_TEXT ? 'Documents' : 'Images'}
      </Button>
    )}
    {type === SESSION_TYPE_TEXT && (
      <Button
        variant="contained"
        color="primary"
        size="small"
        sx={{ mb: 1, mr: 1, textTransform: 'none', bgcolor: '#B4FDC0', color: 'black', fontWeight: 800 }}
        onClick={() => setViewMode(true)}
      >
        View questions
      </Button>
    )}
    <Button
      variant="contained"
      color="primary"
      size="small"
      sx={{ mb: 1,  textTransform: 'none', bgcolor: '#ffffff', color: 'black', fontWeight: 800 }}
      onClick={() => setCloneMode(true)}
    >
      Clone
    </Button>
   
  </Grid>


  
      {viewMode && interactionQuestions.loaded && (
        <FineTuneTextQuestionEditor
          // title="View Questions"
          cancelTitle="Close"
          readOnly
          initialQuestions={interactionQuestions.questions}
          onCancel={() => setViewMode(false)}
        />
      )}
  
      {cloneMode && (
        <Window
          
          size="lg"
          open={cloneMode}
          withCancel
          onCancel={() => setCloneMode(false)}
          rightButtons={
            
            <Button
              variant="contained"
              sx={{ bgcolor: '#fcdb05', color: 'black',  mr: 3, }}
              onClick={handleCloneSelectedMode}
              disabled={!selectedCloneMode}
            >
              Clone with selected
            </Button>
          }
        >
   <Grid container spacing={2}>
            {/* Card for "Just Data" */}
    <Grid item xs={12} md={colSize} onClick={() => handleCardClick(CLONE_INTERACTION_MODE_JUST_DATA)}>
     <Card
                sx={{
                  height: '100%',
                  display: 'flex',
                  mt: 3,
                  ml: 3,
                  flexDirection: 'column',
                  backgroundColor: selectedCloneMode === CLONE_INTERACTION_MODE_JUST_DATA ? '#fcdb05' : 'default',
                  borderRadius: '10px',
                  
                }}
                onClick={() => handleCardClick(CLONE_INTERACTION_MODE_JUST_DATA)}
              >
              <CardContent
      sx={{
        flexGrow: 1,
        // This applies the color conditionally to all child elements
        color: selectedCloneMode === CLONE_INTERACTION_MODE_JUST_DATA ? 'black' : 'text.secondary',
      }}
     >    
      <TextureIcon
        fontSize="large"
        sx={{ color: selectedCloneMode === CLONE_INTERACTION_MODE_JUST_DATA ? 'black' : 'text.secondary' }}
      />
      <Typography gutterBottom variant="h5" component="div">
        Just Data
      </Typography>
      <Typography gutterBottom variant="body2">
        Start again with the original data. Both the trained model and question answer pairs will be removed.
      </Typography>
      {/* Conditional rendering based on session type is removed since it's redundant */}
       </CardContent>
   </Card>
   </Grid>
  
            {/* Card for "With Questions" */}
  {type === SESSION_TYPE_TEXT && (
   <Grid item xs={12} md={colSize} onClick={() => handleCardClick(CLONE_INTERACTION_MODE_WITH_QUESTIONS)}>
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        mt: 3,
        flexDirection: 'column',
        backgroundColor: selectedCloneMode === CLONE_INTERACTION_MODE_WITH_QUESTIONS ? '#b4fdc0' : 'default',
        borderRadius: '10px',
      }}
     >
      <CardContent
        sx={{
          flexGrow: 1,
          color: selectedCloneMode === CLONE_INTERACTION_MODE_WITH_QUESTIONS ? 'black' : 'text.secondary',
        }}
      >
        <TextsmsIcon
          fontSize="large"
          sx={{ color: selectedCloneMode === CLONE_INTERACTION_MODE_WITH_QUESTIONS ? 'black' : 'text.secondary' }}
        />
        <Typography gutterBottom variant="h5" component="div">
          With Questions
        </Typography>
        <Typography variant="body2">
          The question & answer pairs will be retained but the trained model will be removed.
        </Typography>
      </CardContent>
    </Card>
   </Grid>
   )}
  
            {/* Card for "With Training" */}
   <Grid item xs={12} md={colSize} onClick={() => handleCardClick(CLONE_INTERACTION_MODE_ALL)}>
   <Card
    sx={{
      height: '100%',
      display: 'flex',
      mt: 3,
      mr: 3,
      flexDirection: 'column',
      backgroundColor: selectedCloneMode === CLONE_INTERACTION_MODE_ALL ? '#f0beb0' : 'default',
      borderRadius: '10px',
    }}
   >
    <CardContent
      sx={{
        flexGrow: 1,
        // Apply black text color when the card is selected
        color: selectedCloneMode === CLONE_INTERACTION_MODE_ALL ? 'black' : 'text.secondary',
      }}
    >
      <WrapTextIcon
        fontSize="large"
        // Apply black color to the icon when the card is selected
        sx={{ color: selectedCloneMode === CLONE_INTERACTION_MODE_ALL ? 'black' : 'text.secondary' }}
      />
      <Typography gutterBottom variant="h5" component="div">
        With Training
      </Typography>
      <Typography variant="body2">
        Clone everything including the trained model.
      </Typography>
    </CardContent>
   </Card>
</Grid>
          </Grid>
        </Window>
      )}
    </>
  );
};

export default FineTuneCloneInteraction;