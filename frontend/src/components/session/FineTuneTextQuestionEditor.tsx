import React, { FC, useState, useMemo } from 'react';
import { v4 as uuidv4 } from 'uuid';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';

import DataGrid2, { IDataGrid2_Column } from '../datagrid/DataGrid';
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

  const columns = useMemo<IDataGrid2_Column<IQuestionAnswer>[]>(() => [
    {
      name: 'question',
      header: 'Questions',
      defaultFlex: 1,
      render: ({ data }) => (
        <Box sx={{ width: '100%', height: '100%' }}>
          <Typography variant="caption" sx={{ whiteSpace: 'normal', wordBreak: 'break-word' }}>
            {data.question}
          </Typography>
        </Box>
      ),
    },
    {
      name: 'answer',
      header: 'Answers',
      defaultFlex: 1,
      render: ({ data }) => (
        <Box sx={{}}>
          <Typography variant="caption" sx={{ whiteSpace: 'normal', wordBreak: 'break-word' }}>
            {data.answer}
          </Typography>
        </Box>
      ),
    },
    // Other columns can be added here if needed
  ], [readOnly]);

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
      <DataGrid2
        autoSort
        userSelect
        rows={questions}
        columns={columns}
        loading={false}
      />
      {deleteQuestion && (
        <SimpleDeleteConfirmWindow
          title="Confirm Deletion"
          onCancel={() => setDeleteQuestion(null)}
          onSubmit={() => {
            setQuestions(questions.filter(q => q.id !== deleteQuestion.id));
            setDeleteQuestion(null);
            snackbar.info('Question deleted');
          }}
        />
      )}
      {editQuestion && (
        <Window
          title="Edit Question"
          open
          withCancel
          onCancel={() => setEditQuestion(null)}
          onSubmit={() => {
            let updatedQuestions;
            if (editQuestion.id === 'new') {
              updatedQuestions = [...questions, { ...editQuestion, id: uuidv4() }];
            } else {
              updatedQuestions = questions.map(q => (q.id === editQuestion.id ? editQuestion : q));
            }
            setQuestions(updatedQuestions);
            setEditQuestion(null);
            snackbar.info('Question updated');
          }}
        >
          <Box sx={{ p: 2 }}>
            <TextField
              label="Question"
              fullWidth
              multiline
              rows={5}
              value={editQuestion?.question || ''}
              onChange={(e) => setEditQuestion({ ...editQuestion, question: e.target.value })}
            />
          </Box>
          <Box sx={{ p: 2 }}>
            <TextField
              label="Answer"
              fullWidth
              multiline
              rows={5}
              value={editQuestion?.answer || ''}
              onChange={(e) => setEditQuestion({ ...editQuestion, answer: e.target.value })}
            />
          </Box>
        </Window>
      )}
    </Window>
  );
};

export default FineTuneTextQuestionEditor;