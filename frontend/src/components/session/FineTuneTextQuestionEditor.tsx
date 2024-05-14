import React, { FC, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import Grid from '@mui/material/Grid';
import Window from '../widgets/Window';
import SimpleDeleteConfirmWindow from '../widgets/SimpleDeleteConfirmWindow';
import useSnackbar from '../../hooks/useSnackbar';
import { IQuestionAnswer } from '../../types';

export const FineTuneTextQuestionEditor: FC<{
  readOnly?: boolean,
  title?: string,
  cancelTitle?: string,
  initialQuestions: IQuestionAnswer[],
  onSubmit?: (questions: IQuestionAnswer[]) => void,
  onCancel: () => void,
}> = ({
  readOnly = false,
  cancelTitle = 'Cancel',
  initialQuestions,
  onSubmit,
  onCancel,
}) => {
  const snackbar = useSnackbar();

  const [questions, setQuestions] = useState<IQuestionAnswer[]>(initialQuestions);
  const [editQuestion, setEditQuestion] = useState<IQuestionAnswer | null>(null);
  const [deleteQuestion, setDeleteQuestion] = useState<IQuestionAnswer | null>(null);

  return (
    <Window
      size="lg"
      fullHeight
      open
      withCancel={true}
      submitTitle="Save"
      cancelTitle={cancelTitle}
      rightButtons={(
        <>
          <Button
            variant="contained"
            onClick={() => setEditQuestion({ id: 'new', question: '', answer: '' })}
            sx={{
              bgcolor: '#fcdb05',
              color: 'black',
              '&:hover': {
                bgcolor: '#e6c405',
              },
              marginRight: 1,
              mt: 2,
            }}
          >
            Add more
          </Button>
        </>
      )}
      onCancel={onCancel}
      onSubmit={readOnly ? undefined : () => onSubmit && onSubmit(questions)}
    >
      <Box sx={{ display: 'flex', height: '100%', flexGrow: 1 }}>
        <Grid container spacing={2} sx={{ flexGrow: 1, marginTop: 4,  marginLeft: 2  }}>
          <Grid item xs={6}>
            <Typography variant="h6" gutterBottom>
              Questions
            </Typography>
            {questions.map((q, index) => (
              <Typography key={q.id}>{q.question}</Typography>
            ))}
          </Grid>
          <Grid item xs={6}>
            <Typography variant="h6" gutterBottom>
              Answers
            </Typography>
            {questions.map((q, index) => (
              <Typography key={q.id}>{q.answer}</Typography>
            ))}
          </Grid>
        </Grid>
      </Box>
      {/* Rest of the code for deleteQuestion and editQuestion windows */}
      {/* ... */}
    </Window>
  );
};

export default FineTuneTextQuestionEditor;