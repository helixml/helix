import React, { FC, useState, useCallback } from 'react'
import { v4 as uuidv4 } from 'uuid'
import useApi from '../hooks/useApi'
import useSnackbar from './useSnackbar'

import {
  IQuestionAnswer,
  IConversations,
} from '../types'

export const useInteractionQuestions = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  
  const [ questions, setQuestions ] = useState<IQuestionAnswer[]>([])

  const loadQuestions = useCallback(async (sessionID: string, interactionID: string) => {
    setQuestions([])
    const data = await api.get<IConversations[]>(`/api/v1/sessions/${sessionID}/finetune/text/conversations/${interactionID}`)
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
  }, [])

  const saveQuestions = useCallback(async (sessionID: string, interactionID: string, qs: IQuestionAnswer[]): Promise<boolean | undefined> => {
    setQuestions(qs)
    let data: IConversations[] = []
    qs.forEach(q => {
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
    await api.put(`/api/v1/sessions/${sessionID}/finetune/text/conversations/${interactionID}`, data, {}, {
      loading: true,
    })
    snackbar.success('Questions saved')
    return true
  }, [
    questions,
  ])

  return {
    questions,
    setQuestions,
    loadQuestions,
    saveQuestions,
  }
}

export default useInteractionQuestions