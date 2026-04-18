import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import IconButton from '@mui/material/IconButton'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import LinearProgress from '@mui/material/LinearProgress'
import Divider from '@mui/material/Divider'
import Stack from '@mui/material/Stack'
import {
  Plus,
  Trash2,
  Play,
  ChevronDown,
  ChevronRight,
  Check,
  X,
  Loader2,
} from 'lucide-react'

import useSnackbar from '../../hooks/useSnackbar'
import {
  useListEvaluationSuites,
  useCreateEvaluationSuite,
  useUpdateEvaluationSuite,
  useDeleteEvaluationSuite,
  useStartEvaluationRun,
  useListEvaluationRuns,
  useDeleteEvaluationRun,
  evaluationRunsQueryKey,
  evaluationRunQueryKey,
} from '../../services/evaluationService'
import {
  TypesEvaluationSuite,
  TypesEvaluationRun,
  TypesEvaluationQuestionResult,
  TypesEvaluationRunStatus,
  TypesEvaluationRunSummary,
} from '../../api/api'

// Sent via SSE during evaluation runs — not part of the generated API client
interface TypesEvaluationRunProgress {
  run_id: string
  status: TypesEvaluationRunStatus
  current_question: number
  total_questions: number
  latest_result?: TypesEvaluationQuestionResult
  summary?: TypesEvaluationRunSummary
  error?: string
}
import { useQueryClient } from '@tanstack/react-query'

interface EvaluationTabProps {
  appId: string
}

const ASSERTION_TYPES = [
  { value: 'contains', label: 'Contains' },
  { value: 'not_contains', label: 'Not Contains' },
  { value: 'regex', label: 'Regex Match' },
  { value: 'llm_judge', label: 'LLM Judge' },
  { value: 'skill_used', label: 'Skill Used' },
]

// Track which suite has an active run and its accumulated results
interface ActiveRun {
  runId: string
  suiteId: string
  progress: TypesEvaluationRunProgress | null
  results: TypesEvaluationQuestionResult[]
}

const EvaluationTab: FC<EvaluationTabProps> = ({ appId }) => {
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()

  const { data: suitesResponse, isLoading: suitesLoading } = useListEvaluationSuites(appId)
  const suites = suitesResponse?.data || []

  const createSuite = useCreateEvaluationSuite(appId)
  const updateSuite = useUpdateEvaluationSuite(appId)
  const deleteSuite = useDeleteEvaluationSuite(appId)
  const startRun = useStartEvaluationRun(appId)

  const [editingSuiteId, setEditingSuiteId] = useState<string | null>(null)
  const [editingSuite, setEditingSuite] = useState<TypesEvaluationSuite | null>(null)
  const [activeRun, setActiveRun] = useState<ActiveRun | null>(null)

  const eventSourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!activeRun?.runId) {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      return
    }

    const runId = activeRun.runId
    const suiteId = activeRun.suiteId

    const es = new EventSource(`/api/v1/apps/${appId}/evaluation-runs/${runId}/stream`)
    eventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const progress: TypesEvaluationRunProgress = JSON.parse(event.data)

        setActiveRun(prev => {
          if (!prev || prev.runId !== runId) return prev
          const results = [...prev.results]
          // Append latest result if it's new
          if (progress.latest_result && (results.length === 0 || results[results.length - 1].question_id !== progress.latest_result.question_id)) {
            results.push(progress.latest_result)
          }
          return { ...prev, progress, results }
        })

        if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'cancelled') {
          es.close()
          eventSourceRef.current = null
          queryClient.invalidateQueries({ queryKey: evaluationRunsQueryKey(appId, suiteId) })
          queryClient.invalidateQueries({ queryKey: evaluationRunQueryKey(appId, runId) })
        }
      } catch (e) {
        console.error('Failed to parse SSE data', e)
      }
    }

    es.onerror = () => {
      es.close()
      eventSourceRef.current = null
    }

    return () => {
      es.close()
    }
  }, [activeRun?.runId, activeRun?.suiteId, appId, queryClient])

  const handleCreateSuite = useCallback(async () => {
    try {
      const result = await createSuite.mutateAsync({
        name: 'New Evaluation Suite',
        description: '',
        questions: [],
      })
      if (result.data) {
        setEditingSuiteId(result.data.id || null)
        setEditingSuite(result.data)
      }
    } catch (e: any) {
      snackbar.error('Failed to create suite')
    }
  }, [createSuite, snackbar])

  const handleSaveSuite = useCallback(async () => {
    if (!editingSuite) return
    try {
      const result = await updateSuite.mutateAsync(editingSuite)
      if (result.data) {
        setEditingSuite(result.data)
      }
      snackbar.success('Suite saved')
    } catch (e: any) {
      snackbar.error('Failed to save suite')
    }
  }, [editingSuite, updateSuite, snackbar])

  const handleDeleteSuite = useCallback(async (suiteId: string) => {
    try {
      await deleteSuite.mutateAsync(suiteId)
      if (editingSuiteId === suiteId) {
        setEditingSuiteId(null)
        setEditingSuite(null)
      }
      if (activeRun?.suiteId === suiteId) {
        setActiveRun(null)
      }
      snackbar.success('Suite deleted')
    } catch (e: any) {
      snackbar.error('Failed to delete suite')
    }
  }, [deleteSuite, editingSuiteId, activeRun, snackbar])

  const handleStartRun = useCallback(async (suiteId: string) => {
    try {
      const result = await startRun.mutateAsync(suiteId)
      if (result.data?.id) {
        setActiveRun({
          runId: result.data.id,
          suiteId,
          progress: null,
          results: [],
        })
      }
    } catch (e: any) {
      snackbar.error('Failed to start evaluation')
    }
  }, [startRun, snackbar])

  const handleEditSuite = useCallback((suite: TypesEvaluationSuite) => {
    setEditingSuiteId(suite.id || null)
    setEditingSuite(JSON.parse(JSON.stringify(suite)))
  }, [])

  const addQuestion = () => {
    if (!editingSuite) return
    const questions = [...(editingSuite.questions || [])]
    questions.push({ id: crypto.randomUUID(), question: '', assertions: [] })
    setEditingSuite({ ...editingSuite, questions })
  }

  const removeQuestion = (idx: number) => {
    if (!editingSuite) return
    const questions = [...(editingSuite.questions || [])]
    questions.splice(idx, 1)
    setEditingSuite({ ...editingSuite, questions })
  }

  const updateQuestion = (idx: number, field: string, value: string) => {
    if (!editingSuite) return
    const questions = [...(editingSuite.questions || [])]
    questions[idx] = { ...questions[idx], [field]: value }
    setEditingSuite({ ...editingSuite, questions })
  }

  const addAssertion = (qIdx: number) => {
    if (!editingSuite) return
    const questions = [...(editingSuite.questions || [])]
    const assertions = [...(questions[qIdx].assertions || [])]
    assertions.push({ type: 'contains' as any, value: '' })
    questions[qIdx] = { ...questions[qIdx], assertions }
    setEditingSuite({ ...editingSuite, questions })
  }

  const removeAssertion = (qIdx: number, aIdx: number) => {
    if (!editingSuite) return
    const questions = [...(editingSuite.questions || [])]
    const assertions = [...(questions[qIdx].assertions || [])]
    assertions.splice(aIdx, 1)
    questions[qIdx] = { ...questions[qIdx], assertions }
    setEditingSuite({ ...editingSuite, questions })
  }

  const updateAssertion = (qIdx: number, aIdx: number, field: string, value: string) => {
    if (!editingSuite) return
    const questions = [...(editingSuite.questions || [])]
    const assertions = [...(questions[qIdx].assertions || [])]
    assertions[aIdx] = { ...assertions[aIdx], [field]: value }
    questions[qIdx] = { ...questions[qIdx], assertions }
    setEditingSuite({ ...editingSuite, questions })
  }

  return (
    <Box sx={{ mt: 2, mr: 2 }}>
      <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
        Evaluation
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Define test suites with questions and assertions to evaluate your agent's responses, knowledge, and skill usage.
      </Typography>

      {suitesLoading && <LinearProgress sx={{ mb: 2 }} />}

      {suites.map((suite) => (
        <Box
          key={suite.id}
          sx={{ mb: 3, p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}
        >
          {editingSuiteId === suite.id && editingSuite ? (
            <SuiteEditor
              suite={editingSuite}
              onNameChange={(name) => setEditingSuite({ ...editingSuite, name })}
              onDescriptionChange={(desc) => setEditingSuite({ ...editingSuite, description: desc })}
              onAddQuestion={addQuestion}
              onRemoveQuestion={removeQuestion}
              onUpdateQuestion={updateQuestion}
              onAddAssertion={addAssertion}
              onRemoveAssertion={removeAssertion}
              onUpdateAssertion={updateAssertion}
              onSave={handleSaveSuite}
              onCancel={() => { setEditingSuiteId(null); setEditingSuite(null) }}
              saving={updateSuite.isPending}
            />
          ) : (
            <SuiteListItem
              suite={suite}
              appId={appId}
              onEdit={() => handleEditSuite(suite)}
              onDelete={() => suite.id && handleDeleteSuite(suite.id)}
              onRun={() => suite.id && handleStartRun(suite.id)}
              isRunning={startRun.isPending}
              activeRun={activeRun?.suiteId === suite.id ? activeRun : null}
              onDismissRun={() => setActiveRun(null)}
            />
          )}
        </Box>
      ))}

      <Button
        variant="outlined"
        color="secondary"
        startIcon={<Plus size={16} />}
        onClick={handleCreateSuite}
        disabled={createSuite.isPending}
        sx={{ mt: 1 }}
      >
        New Suite
      </Button>
    </Box>
  )
}

// --- Sub Components ---

interface QuestionResultCardProps {
  result: TypesEvaluationQuestionResult
}

const QuestionResultCard: FC<QuestionResultCardProps> = ({ result }) => {
  const passed = result.passed && !result.error

  return (
    <Box sx={{ p: 1.5, borderRadius: 1, border: '1px solid', borderColor: passed ? 'success.main' : 'error.main' }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
        <Typography variant="body2" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {passed
            ? <Check size={14} color="#4caf50" />
            : <X size={14} color="#f44336" />}
          {result.question}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0, ml: 1 }}>
          {((result.duration_ms || 0) / 1000).toFixed(1)}s
        </Typography>
      </Box>
      {result.response && (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, whiteSpace: 'pre-wrap', maxHeight: 120, overflow: 'auto', fontSize: '0.8rem' }}>
          {result.response.substring(0, 500)}{result.response.length > 500 ? '...' : ''}
        </Typography>
      )}
      {result.skills_used && result.skills_used.length > 0 && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
          Skills: {result.skills_used.join(', ')}
        </Typography>
      )}
      {result.assertion_results && result.assertion_results.length > 0 && (
        <Box sx={{ mt: 1 }}>
          {result.assertion_results.map((ar, i) => (
            <Typography key={i} variant="caption" sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.25, color: ar.passed ? '#4caf50' : '#f44336' }}>
              {ar.passed ? <Check size={12} /> : <X size={12} />}
              {ar.assertion_type}: {ar.assertion_value}
              {ar.details ? ` — ${ar.details}` : ''}
            </Typography>
          ))}
        </Box>
      )}
      {result.error && (
        <Typography variant="caption" sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5, color: '#f44336' }}>
          <X size={12} /> {result.error}
        </Typography>
      )}
    </Box>
  )
}

// Pending question placeholder shown while a question is being processed
const PendingQuestionCard: FC<{ question: string; index: number }> = ({ question, index }) => {
  return (
    <Box sx={{ p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'divider', opacity: 0.5 }}>
      <Typography variant="body2" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <Loader2 size={14} />
        {question}
      </Typography>
    </Box>
  )
}

interface SuiteListItemProps {
  suite: TypesEvaluationSuite
  appId: string
  onEdit: () => void
  onDelete: () => void
  onRun: () => void
  isRunning: boolean
  activeRun: ActiveRun | null
  onDismissRun: () => void
}

const SuiteListItem: FC<SuiteListItemProps> = ({ suite, appId, onEdit, onDelete, onRun, isRunning, activeRun, onDismissRun }) => {
  const [showRuns, setShowRuns] = useState(false)
  const { data: runsResponse } = useListEvaluationRuns(appId, suite.id || '')
  const deleteRun = useDeleteEvaluationRun(appId, suite.id || '')
  const runs = runsResponse?.data || []

  const progress = activeRun?.progress
  const isActive = !!activeRun
  const isComplete = progress?.status === 'completed' || progress?.status === 'failed' || progress?.status === 'cancelled'
  const questions = suite.questions || []

  const pct = progress?.total_questions
    ? Math.round(((progress.current_question || 0) / progress.total_questions) * 100)
    : 0

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Box>
          <Typography gutterBottom>{suite.name}</Typography>
          <Typography variant="body2" color="text.secondary">
            {suite.description || `${questions.length} question(s)`}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1}>
          <Button
            size="small"
            color="secondary"
            variant="outlined"
            startIcon={<Play size={14} />}
            onClick={onRun}
            disabled={isRunning || !questions.length || (isActive && !isComplete)}
          >
            Run
          </Button>
          <Button size="small" color="secondary" variant="outlined" onClick={onEdit}>
            Edit
          </Button>
          <IconButton size="small" onClick={onDelete}>
            <Trash2 size={16} />
          </IconButton>
        </Stack>
      </Box>

      {/* Inline run progress — shows when a run is active for this suite */}
      {isActive && (
        <Box sx={{ mt: 2 }}>
          <Divider sx={{ mb: 2 }} />

          {/* Progress bar and stats */}
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
            <Typography variant="body2" sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              {!isComplete && <Loader2 size={14} />}
              {progress?.status === 'completed' && <Check size={14} color="#4caf50" />}
              {(progress?.status === 'failed' || progress?.status === 'cancelled') && <X size={14} color="#f44336" />}
              {progress?.status || 'starting...'}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {progress?.current_question || 0} / {progress?.total_questions || questions.length}
            </Typography>
          </Box>
          <LinearProgress variant="determinate" value={pct} color="inherit" sx={{ mb: 1.5, height: 4, borderRadius: 2 }} />

          {progress?.summary && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
              {progress.summary.passed || 0} passed, {progress.summary.failed || 0} failed
              {' · '}{progress.summary.total_tokens || 0} tokens
              {' · '}${(progress.summary.total_cost || 0).toFixed(4)}
              {' · '}{((progress.summary.total_duration_ms || 0) / 1000).toFixed(1)}s
            </Typography>
          )}

          {/* Question-by-question results */}
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            {/* Completed results */}
            {activeRun.results.map((result, i) => (
              <QuestionResultCard key={result.question_id || i} result={result} />
            ))}

            {/* Currently running question */}
            {!isComplete && (progress?.current_question || 0) < questions.length && (
              <PendingQuestionCard
                question={questions[progress?.current_question || 0]?.question || ''}
                index={(progress?.current_question || 0) + 1}
              />
            )}

            {/* Remaining questions shown as dimmed */}
            {!isComplete && questions.slice((progress?.current_question || 0) + 1).map((q, i) => (
              <Box key={q.id || i} sx={{ p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'divider', opacity: 0.3 }}>
                <Typography variant="body2">{q.question}</Typography>
              </Box>
            ))}
          </Box>

          {/* Dismiss button when complete */}
          {isComplete && (
            <Box sx={{ mt: 2, display: 'flex', justifyContent: 'flex-end' }}>
              <Button size="small" color="secondary" variant="outlined" onClick={onDismissRun}>
                Dismiss
              </Button>
            </Box>
          )}
        </Box>
      )}

      {/* Past run history */}
      {!isActive && runs.length > 0 && (
        <Box sx={{ mt: 1.5 }}>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 0.5, userSelect: 'none' }}
            onClick={() => setShowRuns(!showRuns)}
          >
            {showRuns ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            Run history ({runs.length})
          </Typography>
          {showRuns && (
            <Box sx={{ mt: 1 }}>
              {runs.map((run) => (
                <RunSummaryRow key={run.id} run={run} onDelete={() => run.id && deleteRun.mutate(run.id)} />
              ))}
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}

interface RunSummaryRowProps {
  run: TypesEvaluationRun
  onDelete: () => void
}

const RunSummaryRow: FC<RunSummaryRowProps> = ({ run, onDelete }) => {
  const [expanded, setExpanded] = useState(false)
  const allPassed = run.status === 'completed' && (run.summary?.failed || 0) === 0
  const hasFailed = (run.summary?.failed || 0) > 0 || run.status === 'failed'

  return (
    <Box sx={{ mb: 1, p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
      <Box
        sx={{ display: 'flex', alignItems: 'center', gap: 2, cursor: 'pointer', userSelect: 'none' }}
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <Typography variant="body2" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {allPassed && <Check size={14} color="#4caf50" />}
          {hasFailed && <X size={14} color="#f44336" />}
          {run.status}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {(run.summary?.passed || 0)}/{run.summary?.total_questions || 0} passed
          {(run.summary?.failed || 0) > 0 && (
            <span style={{ color: '#f44336', marginLeft: 8 }}>{run.summary?.failed} failed</span>
          )}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {run.created ? new Date(run.created).toLocaleString() : ''}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ ml: 'auto', mr: 1 }}>
          {((run.summary?.total_duration_ms || 0) / 1000).toFixed(1)}s · {run.summary?.total_tokens || 0} tokens · ${(run.summary?.total_cost || 0).toFixed(4)}
        </Typography>
        <IconButton
          size="small"
          onClick={(e) => { e.stopPropagation(); onDelete() }}
        >
          <Trash2 size={14} />
        </IconButton>
      </Box>
      {expanded && (
        <Box sx={{ mt: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
          {run.results && run.results.map((result, i) => (
            <QuestionResultCard key={i} result={result} />
          ))}
          {run.error && (
            <Typography variant="caption" sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1, color: '#f44336' }}>
              <X size={12} /> {run.error}
            </Typography>
          )}
          {run.summary?.skills_used && run.summary.skills_used.length > 0 && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
              Skills used: {run.summary.skills_used.join(', ')}
            </Typography>
          )}
        </Box>
      )}
    </Box>
  )
}

interface SuiteEditorProps {
  suite: TypesEvaluationSuite
  onNameChange: (name: string) => void
  onDescriptionChange: (desc: string) => void
  onAddQuestion: () => void
  onRemoveQuestion: (idx: number) => void
  onUpdateQuestion: (idx: number, field: string, value: string) => void
  onAddAssertion: (qIdx: number) => void
  onRemoveAssertion: (qIdx: number, aIdx: number) => void
  onUpdateAssertion: (qIdx: number, aIdx: number, field: string, value: string) => void
  onSave: () => void
  onCancel: () => void
  saving: boolean
}

const SuiteEditor: FC<SuiteEditorProps> = ({
  suite, onNameChange, onDescriptionChange,
  onAddQuestion, onRemoveQuestion, onUpdateQuestion,
  onAddAssertion, onRemoveAssertion, onUpdateAssertion,
  onSave, onCancel, saving,
}) => {
  return (
    <Box>
      <TextField
        label="Suite Name"
        value={suite.name || ''}
        onChange={(e) => onNameChange(e.target.value)}
        fullWidth
        size="small"
        sx={{ mb: 2 }}
      />
      <TextField
        label="Description"
        value={suite.description || ''}
        onChange={(e) => onDescriptionChange(e.target.value)}
        fullWidth
        size="small"
        multiline
        rows={2}
        sx={{ mb: 2 }}
      />

      <Divider sx={{ mb: 2 }} />
      <Typography variant="body2" sx={{ mb: 1 }}>Questions</Typography>

      {(suite.questions || []).map((q, qIdx) => (
        <Box key={q.id || qIdx} sx={{ p: 2, mb: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
          <Box sx={{ display: 'flex', gap: 1, mb: 1 }}>
            <TextField
              label={`Question ${qIdx + 1}`}
              value={q.question || ''}
              onChange={(e) => onUpdateQuestion(qIdx, 'question', e.target.value)}
              fullWidth
              size="small"
              multiline
              rows={2}
            />
            <IconButton size="small" onClick={() => onRemoveQuestion(qIdx)}>
              <Trash2 size={16} />
            </IconButton>
          </Box>

          <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>
            Assertions
          </Typography>
          {(q.assertions || []).map((a, aIdx) => (
            <Box key={aIdx} sx={{ display: 'flex', gap: 1, mb: 1, alignItems: 'center' }}>
              <FormControl size="small" sx={{ minWidth: 140 }}>
                <InputLabel>Type</InputLabel>
                <Select
                  value={a.type || 'contains'}
                  label="Type"
                  onChange={(e) => onUpdateAssertion(qIdx, aIdx, 'type', e.target.value)}
                >
                  {ASSERTION_TYPES.map((t) => (
                    <MenuItem key={t.value} value={t.value}>{t.label}</MenuItem>
                  ))}
                </Select>
              </FormControl>
              <TextField
                label={a.type === 'llm_judge' ? 'Criteria / Expected behavior' : a.type === 'skill_used' ? 'Skill name' : 'Value'}
                value={a.value || ''}
                onChange={(e) => onUpdateAssertion(qIdx, aIdx, 'value', e.target.value)}
                fullWidth
                size="small"
              />
              {a.type === 'llm_judge' && (
                <TextField
                  label="Custom Judge Prompt (optional)"
                  value={a.llm_judge_prompt || ''}
                  onChange={(e) => onUpdateAssertion(qIdx, aIdx, 'llm_judge_prompt', e.target.value)}
                  fullWidth
                  size="small"
                />
              )}
              <IconButton size="small" onClick={() => onRemoveAssertion(qIdx, aIdx)}>
                <Trash2 size={14} />
              </IconButton>
            </Box>
          ))}
          <Button size="small" color="secondary" onClick={() => onAddAssertion(qIdx)} startIcon={<Plus size={14} />}>
            Add Assertion
          </Button>
        </Box>
      ))}

      <Button size="small" color="secondary" onClick={onAddQuestion} startIcon={<Plus size={14} />} sx={{ mb: 2 }}>
        Add Question
      </Button>

      <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
        <Button onClick={onCancel} color="secondary" variant="outlined" size="small">
          Close
        </Button>
        <Button onClick={onSave} color="secondary" variant="outlined" size="small" disabled={saving}>
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </Box>
    </Box>
  )
}

export default EvaluationTab
