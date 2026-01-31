import React, { useState, useEffect } from 'react';
import { v4 as uuidv4 } from 'uuid';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  TextField,
  IconButton,
  Alert,
  CircularProgress,
  Typography,
  Divider,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import DarkDialog from '../dialog/DarkDialog';
import AgentSelector from '../tasks/AgentSelector';
import ExecutionsHistory from './ExecutionsHistory';
import { TypesQuestionSet, TypesQuestion } from '../../api/api';
import { useCreateQuestionSet, useUpdateQuestionSet, useQuestionSet, useExecuteQuestionSet } from '../../services/questionSetsService';
import useAccount from '../../hooks/useAccount';
import useSnackbar from '../../hooks/useSnackbar';
import { IApp } from '../../types';

interface QuestionSetDialogProps {
  open: boolean;
  onClose: () => void;
  questionSetId?: string;
  apps: IApp[];
}

interface QuestionState {
  id: string;
  question: string;
}

const QuestionSetDialog: React.FC<QuestionSetDialogProps> = ({ open, onClose, questionSetId, apps }) => {
  const account = useAccount();
  const snackbar = useSnackbar();

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [questions, setQuestions] = useState<QuestionState[]>([{ id: uuidv4(), question: '' }]);
  const [selectedAgent, setSelectedAgent] = useState<IApp | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isExecuting, setIsExecuting] = useState(false);
  const [createdQuestionSetId, setCreatedQuestionSetId] = useState<string | undefined>(questionSetId);

  const { data: existingQuestionSet, isLoading: isLoadingQuestionSet } = useQuestionSet(
    questionSetId || '',
    { enabled: !!questionSetId && open }
  );

  const createMutation = useCreateQuestionSet();
  const updateMutation = useUpdateQuestionSet();
  const executeMutation = useExecuteQuestionSet();

  useEffect(() => {
    if (apps.length > 0 && !selectedAgent) {
      setSelectedAgent(apps[0]);
    }
  }, [apps]);

  useEffect(() => {
    setCreatedQuestionSetId(questionSetId);
  }, [questionSetId]);

  useEffect(() => {
    if (open && questionSetId && existingQuestionSet) {
      setName(existingQuestionSet.name || '');
      setDescription(existingQuestionSet.description || '');
      setQuestions(
        existingQuestionSet.questions && existingQuestionSet.questions.length > 0
          ? existingQuestionSet.questions.map((q) => ({
              id: q.id || uuidv4(),
              question: q.question || '',
            }))
          : [{ id: uuidv4(), question: '' }]
      );
      setError(null);
      setCreatedQuestionSetId(questionSetId);
    } else if (open && !questionSetId) {
      setName('');
      setDescription('');
      setQuestions([{ id: uuidv4(), question: '' }]);
      setError(null);
      setCreatedQuestionSetId(undefined);
    }
  }, [open, questionSetId, existingQuestionSet]);

  const handleAddQuestion = () => {
    setQuestions([...questions, { id: uuidv4(), question: '' }]);
  };

  const handleRemoveQuestion = (id: string) => {
    if (questions.length > 1) {
      setQuestions(questions.filter((q) => q.id !== id));
    }
  };

  const handleQuestionChange = (id: string, value: string) => {
    setQuestions(questions.map((q) => (q.id === id ? { ...q, question: value } : q)));
  };

  const handleSave = async () => {
    if (!name.trim()) {
      setError('Name is required');
      return;
    }

    if (!description.trim()) {
      setError('Description is required');
      return;
    }

    const validQuestions = questions.filter((q) => q.question.trim());
    if (validQuestions.length === 0) {
      setError('At least one question is required');
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const orgId = account.organizationTools.organization?.id || '';
      const questionsData: TypesQuestion[] = validQuestions.map((q) => ({
        id: q.id,
        question: q.question.trim(),
      }));

      const questionSetData: TypesQuestionSet = {
        name: name.trim(),
        description: description.trim(),
        questions: questionsData,
        user_id: account.user?.id || '',
        organization_id: orgId || undefined,
      };

      if (questionSetId && existingQuestionSet) {
        await updateMutation.mutateAsync({
          id: questionSetId,
          questionSet: {
            ...questionSetData,
            id: questionSetId,
          },
        });
        snackbar.success('Question set updated successfully');
      } else {
        const newQuestionSet = await createMutation.mutateAsync({
          questionSet: questionSetData,
          orgId: orgId || undefined,
        });
        if (newQuestionSet.id) {
          setCreatedQuestionSetId(newQuestionSet.id);
        }
        snackbar.success('Question set created successfully');
      }
    } catch (err) {
      console.error('Error saving question set:', err);
      setError(err instanceof Error ? err.message : 'Failed to save question set');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    if (!isSubmitting && !isExecuting) {
      setError(null);
      onClose();
    }
  };

  const handleRun = async () => {
    const currentQuestionSetId = createdQuestionSetId || questionSetId;
    if (!currentQuestionSetId) {
      setError('Question set must be saved before running');
      return;
    }

    if (!selectedAgent) {
      setError('Please select an agent');
      return;
    }

    setIsExecuting(true);
    setError(null);

    try {
      await executeMutation.mutateAsync({
        id: currentQuestionSetId,
        request: {
          app_id: selectedAgent.id,
          question_set_id: currentQuestionSetId,
        },
      });
      snackbar.success('Question set executed successfully');
    } catch (err) {
      console.error('Error executing question set:', err);
      setError(err instanceof Error ? err.message : 'Failed to execute question set');
    } finally {
      setIsExecuting(false);
    }
  };

  if (isLoadingQuestionSet && questionSetId) {
    return (
      <DarkDialog open={open} onClose={handleClose} maxWidth="xl" fullWidth>
        <DialogContent sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 200 }}>
          <CircularProgress />
        </DialogContent>
      </DarkDialog>
    );
  }

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="xl"
      fullWidth
      PaperProps={{
        sx: {
          maxHeight: '90vh',
        },
      }}
    >
      <DialogTitle
        sx={{
          m: 0,
          p: 2,
          ml: 1,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}
      >
        <Typography variant="h6" component="div">
          {questionSetId ? 'Edit Question Set' : 'Create Question Set'}
        </Typography>
        <IconButton
          aria-label="close"
          onClick={handleClose}
          disabled={isSubmitting || isExecuting}
          sx={{ color: '#A0AEC0' }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3 }}>
        <Box sx={{ display: 'flex', gap: 3 }}>
          <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 3 }}>
            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}

            <TextField
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            disabled={isSubmitting || isExecuting}
            fullWidth
            sx={{
              '& .MuiInputBase-root': {
                color: '#F1F1F1',
              },
              '& .MuiInputLabel-root': {
                color: '#A0AEC0',
              },
              '& .MuiOutlinedInput-notchedOutline': {
                borderColor: '#2D3748',
              },
              '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': {
                borderColor: '#4A5568',
              },
              '& .MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline': {
                borderColor: '#4A5568',
              },
            }}
          />

          <TextField
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            required
            disabled={isSubmitting || isExecuting}
            fullWidth
            multiline
            rows={3}
            sx={{
              '& .MuiInputBase-root': {
                color: '#F1F1F1',
              },
              '& .MuiInputLabel-root': {
                color: '#A0AEC0',
              },
              '& .MuiOutlinedInput-notchedOutline': {
                borderColor: '#2D3748',
              },
              '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': {
                borderColor: '#4A5568',
              },
              '& .MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline': {
                borderColor: '#4A5568',
              },
            }}
          />

          <Divider sx={{ borderColor: '#2D3748' }} />

          <Box>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Typography variant="h6" sx={{ color: '#F1F1F1' }}>
                Questions
              </Typography>
              <Button
                startIcon={<AddIcon />}
                onClick={handleAddQuestion}
                disabled={isSubmitting || isExecuting}
                sx={{
                  color: '#A0AEC0',
                  '&:hover': {
                    backgroundColor: 'rgba(255, 255, 255, 0.05)',
                  },
                }}
              >
                Add Question
              </Button>
            </Box>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              {questions.map((q, index) => (
                <Box
                  key={q.id}
                  sx={{
                    display: 'flex',
                    gap: 1,
                    alignItems: 'flex-start',
                    p: 2,
                    backgroundColor: '#1A202C',
                    borderRadius: 1,
                  }}
                >
                  <Box sx={{ flex: 1 }}>
                    <TextField
                      label={`Question ${index + 1}`}
                      value={q.question}
                      onChange={(e) => handleQuestionChange(q.id, e.target.value)}
                      disabled={isSubmitting || isExecuting}
                      fullWidth
                      multiline
                      rows={2}
                      sx={{
                        '& .MuiInputBase-root': {
                          color: '#F1F1F1',
                        },
                        '& .MuiInputLabel-root': {
                          color: '#A0AEC0',
                        },
                        '& .MuiOutlinedInput-notchedOutline': {
                          borderColor: '#2D3748',
                        },
                        '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': {
                          borderColor: '#4A5568',
                        },
                        '& .MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline': {
                          borderColor: '#4A5568',
                        },
                      }}
                    />
                  </Box>
                  <IconButton
                    onClick={() => handleRemoveQuestion(q.id)}
                    disabled={isSubmitting || isExecuting || questions.length === 1}
                    sx={{
                      color: questions.length === 1 ? '#4A5568' : '#EF4444',
                      '&:hover': {
                        backgroundColor: questions.length === 1 ? 'transparent' : 'rgba(239, 68, 68, 0.1)',
                      },
                    }}
                  >
                    <DeleteIcon />
                  </IconButton>
                </Box>
              ))}
            </Box>
          </Box>
          </Box>

          {(createdQuestionSetId || questionSetId) && (
            <ExecutionsHistory 
              questionSetId={createdQuestionSetId || questionSetId}
              questionSetName={name || existingQuestionSet?.name || 'Untitled'}
            />
          )}
        </Box>
      </DialogContent>

      <DialogActions
        sx={{
          p: 3,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 1,
        }}
      >
        <Box sx={{ display: 'flex', gap: 1 }}>
          <AgentSelector
            apps={apps}
            selectedAgent={selectedAgent}
            onAgentSelect={setSelectedAgent}
          />
        </Box>
        <Box sx={{ display: 'flex', gap: 1 }}>
          {(createdQuestionSetId || questionSetId) && (
            <Button
              variant="outlined"
              onClick={handleRun}
              color="primary"
              disabled={!selectedAgent || isExecuting || isSubmitting}
              startIcon={isExecuting ? <CircularProgress size={16} /> : undefined}
            >
              {isExecuting ? 'Running...' : 'Run'}
            </Button>
          )}
          <Button
            variant="outlined"
            onClick={handleSave}
            color="secondary"
            disabled={
              isSubmitting ||
              isExecuting ||
              !name.trim() ||
              !description.trim() ||
              questions.filter((q) => q.question.trim()).length === 0
            }
            startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
          >
            {isSubmitting ? 'Saving...' : questionSetId ? 'Save' : 'Create'}
          </Button>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default QuestionSetDialog;

