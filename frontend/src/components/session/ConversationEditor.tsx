import React, { FC, useState, useMemo } from 'react'

import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'

import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'

import DataGrid2, { IDataGrid2_Column } from '../datagrid/DataGrid'
import useApi from '../../hooks/useApi'
import ClickLink from '../widgets/ClickLink'
import Window from '../widgets/Window'
import SimpleDeleteConfirmWindow from '../widgets/SimpleDeleteConfirmWindow'

import useSnackbar from '../../hooks/useSnackbar'

import {
  IQuestionAnswer,
} from '../../types'

export const ConversationEditor: FC<{
  readOnly?: boolean,
  initialQuestions: IQuestionAnswer[],
  onSubmit: {
    (questions: IQuestionAnswer[]): void,
  },
  onCancel: {
    (): void,
  }
}> = ({
  readOnly = false,
  initialQuestions,
  onSubmit,
  onCancel,
}) => {
  const snackbar = useSnackbar()

  const [ questions, setQuestions ] = useState<IQuestionAnswer[]>(initialQuestions)
  const [ editQuestion, setEditQuestion ] = useState<IQuestionAnswer>()
  const [ deleteQuestion, setDeleteQuestion ] = useState<IQuestionAnswer>()
  
  const columns = useMemo<IDataGrid2_Column<IQuestionAnswer>[]>(() => {
    return [{
      name: 'question',
      header: 'Question',
      defaultFlex: 1,
      render: ({ data }) => {
        return (
          <Box
            sx={{
              width: '100%',
              height: '100%',
            }}
          >
            <Typography
              variant="caption"
              sx={{
                whiteSpace: 'normal',
                wordBreak: 'break-word',
              }}
            >
              { data.question }
            </Typography>
          </Box>
        )
      }
    },
    {
      name: 'answer',
      header: 'Answer',
      defaultFlex: 1,
      render: ({ data }) => {
        return (
          <Box
            sx={{
              
            }}
          >
            <Typography
              variant="caption"
              sx={{
                whiteSpace: 'normal',
                wordBreak: 'break-word',
              }}
            >
              { data.answer }
            </Typography>
          </Box>
        )
      }
    },
    {
      name: 'actions',
      header: 'Actions',
      minWidth: 120,
      defaultWidth: 120,
      render: ({ data }) => {
        if(readOnly) return null
        return (
          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'flex-end',
              justifyContent: 'space-between',
              pl: 2,
              pr: 2,
            }}
          >
            <ClickLink
              onClick={ () => {
                setDeleteQuestion(data)
              }}
            >
              <DeleteIcon />
            </ClickLink>
            <ClickLink
              onClick={ () => {
                setEditQuestion(data)
              }}
            >
              <EditIcon />
            </ClickLink>
          </Box>
        )
      }
    }
  ]
  }, [
    readOnly,
  ])

  return (
    <Window
      title="Edit Questions"
      size="lg"
      fullHeight
      open
      withCancel
      submitTitle="Save"
      onCancel={ onCancel }
      onSubmit={ () => {
        onSubmit(questions)
      }}
    >
      <DataGrid2
        autoSort
        userSelect
        rows={ questions }
        columns={ columns }
        loading={ false }
      />
      {
        deleteQuestion && (
          <SimpleDeleteConfirmWindow
            title="this question"
            onCancel={ () => setDeleteQuestion(undefined) }
            onSubmit={ () => {
              setQuestions(questions.filter(q => q.id != deleteQuestion.id))
              setDeleteQuestion(undefined)
              snackbar.info('Question deleted')
            } }
          />
        )
      }
      {
        editQuestion && (
          <Window
            title="Edit Question"
            open
            withCancel
            onCancel={ () => setEditQuestion(undefined) }
            onSubmit={ () => {
              setQuestions(questions.map(q => {
                if(q.id == editQuestion.id) {
                  return editQuestion
                }
                return q
              }))
              setEditQuestion(undefined)
              snackbar.info('Question updated')
            } }
          >
            <Box
              sx={{
                p: 2,
              }}
            >
              <TextField
                label="Question"
                fullWidth
                multiline
                helperText="Enter the question text here"
                rows={ 5 }
                value={ editQuestion.question }
                onChange={ (e) => {
                  setEditQuestion({
                    ...editQuestion,
                    question: e.target.value,
                  })
                }}
              />
            </Box>
            <Box
              sx={{
                p: 2,
              }}
            >
              <TextField
                label="Answer"
                fullWidth
                multiline
                helperText="Enter the answer text here"
                rows={ 5 }
                value={ editQuestion.answer }
                onChange={ (e) => {
                  setEditQuestion({
                    ...editQuestion,
                    answer: e.target.value,
                  })
                }}
              />
            </Box>
          </Window>
        )
      }
    </Window>
  )  
}

export default ConversationEditor