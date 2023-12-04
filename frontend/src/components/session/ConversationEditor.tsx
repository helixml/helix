import React, { FC, useState, useMemo, useEffect, useCallback } from 'react'
import { v4 as uuidv4 } from 'uuid'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import DataGrid2, { IDataGrid2_Column } from '../datagrid/DataGrid'
import useApi from '../../hooks/useApi'
import ClickLink from '../widgets/ClickLink'
import Window from '../widgets/Window'
import SimpleDeleteConfirmWindow from '../widgets/SimpleDeleteConfirmWindow'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import useSnackbar from '../../hooks/useSnackbar'

import {
  IQuestionAnswer,
  IConversations,
} from '../../types'

export const ConversationEditor: FC<{
  session_id: string,
}> = ({
  session_id,
}) => {
  const snackbar = useSnackbar()
  const api = useApi()

  const [ editMode, setEditMode ] = useState(false)
  const [ editQuestion, setEditQuestion ] = useState<IQuestionAnswer>()
  const [ deleteQuestion, setDeleteQuestion ] = useState<IQuestionAnswer>()
  const [ questions, setQuestions ] = useState<IQuestionAnswer[]>([])

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
  }, [])

  const submitData = useCallback(async () => {
    let data: IConversations[] = []
    questions.forEach(q => {
      const c: IConversations = {
        conversations: [
          {
            from: 'human',
            value: q.question,
          },
          {
            from: 'gpt',
            value: q.answer,
          }
        ]
      }
      data.push(c)
    })
    await api.post(`/api/v1/sessions/${session_id}/finetune/text/conversations`, data, {}, {
      loading: true,
    })
    snackbar.success('Questions saved')
  }, [
    questions,
  ])

  useEffect(() => {
    if(!session_id) return
    const doAsync = async () => {
      const data = await api.get<IConversations[]>(`/api/v1/sessions/${session_id}/finetune/text/conversations`)
      if(!data) return
      let qas: IQuestionAnswer[] = []
      data.forEach(c => {
        const qa: IQuestionAnswer = {
          id: uuidv4(),
          question: '',
          answer: '',
        }
        c.conversations.forEach(c => {
          if(c.from == 'human') {
            qa.question = c.value
          } else if(c.from == 'gpt') {
            qa.answer = c.value
          }
        })
        qas.push(qa)
      })
      setQuestions(qas)
    }
    doAsync()
  }, [
    session_id,
  ])

  return (
    <Box>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
        }}
      >
        <Box
          sx={{
            flexGrow: 1,
          }}
        >
          <Typography gutterBottom>
            Your documents have been turned into question answer pairs ready for fine tuning.
          </Typography>
          {
            editMode ? (
              <Typography gutterBottom>
                Please edit the questions and answers below and click the <strong>Save</strong> button to continue.
              </Typography>
            ) : (
              <Typography gutterBottom>
                You can start training now or edit the questions and answers.
              </Typography>
            )
          }
        </Box>
        <Box
          sx={{
            flexGrow: 0,
          }}
        >
          {
            editMode ? (
              <Button
                variant="contained"
                color="secondary"
                endIcon={<NavigateNextIcon />}
                onClick={ submitData }
              >
                Start Training
              </Button>
            ) : (
              <>
                <Button
                  variant="contained"
                  color="primary"
                  sx={{
                    mr: 2,
                  }}
                  endIcon={<EditIcon />}
                  onClick={ () => setEditMode(true) }
                >
                  Edit Questions
                </Button>
                <Button
                  variant="contained"
                  color="secondary"
                  endIcon={<NavigateNextIcon />}
                  onClick={ submitData }
                >
                  Start Training
                </Button>
              </>
            )
          }
        </Box>

      </Box>
           
      <Box sx={{ height: editMode ? 600 : 0, width: '100%' }}>
        {
          editMode && (
            <DataGrid2
              autoSort
              userSelect
              rows={ questions }
              columns={ columns }
              loading={ false }
            />
          )
        }
        {
          deleteQuestion && (
            <SimpleDeleteConfirmWindow
              title="this question"
              onCancel={ () => setDeleteQuestion(undefined) }
              onSubmit={ () => {
                setQuestions(questions => questions.filter(q => q.id != deleteQuestion.id))
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
                setQuestions(questions => questions.map(q => {
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
      </Box>
    </Box>
  )  
}

export default ConversationEditor