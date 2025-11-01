import React, { useState, useEffect } from 'react';
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
import { TypesQuestionSet, TypesQuestion } from '../../api/api';
import { useCreateQuestionSet, useUpdateQuestionSet, useQuestionSet } from '../../services/questionSetsService';
import useAccount from '../../hooks/useAccount';
import useSnackbar from '../../hooks/useSnackbar';

interface QuestionSetDialogProps {
  open: boolean;
  onClose: () => void;
  questionSetId?: string;
}

interface QuestionState {
  id: string;
  question: string;
}

const QuestionSetDialog: React.FC<QuestionSetDialogProps> = ({ open, onClose, questionSetId }) => {
  const account = useAccount();
  const snackbar = useSnackbar();

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [questions, setQuestions] = useState<QuestionState[]>([{ id: crypto.randomUUID(), question: '' }]);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const { data: existingQuestionSet, isLoading: isLoadingQuestionSet } = useQuestionSet(
    questionSetId || '',
    { enabled: !!questionSetId && open }
  );

  const createMutation = useCreateQuestionSet();
  const updateMutation = useUpdateQuestionSet();

  useEffect(() => {
    if (open && questionSetId && existingQuestionSet) {
      setName(existingQuestionSet.name || '');
      setDescription(existingQuestionSet.description || '');
      setQuestions(
        existingQuestionSet.questions && existingQuestionSet.questions.length > 0
          ? existingQuestionSet.questions.map((q) => ({
              id: q.id || crypto.randomUUID(),
              question: q.question || '',
            }))
          : [{ id: crypto.randomUUID(), question: '' }]
      );
      setError(null);
    } else if (open && !questionSetId) {
      setName('');
      setDescription('');
      setQuestions([{ id: crypto.randomUUID(), question: '' }]);
      setError(null);
    }
  }, [open, questionSetId, existingQuestionSet]);

  const handleAddQuestion = () => {
    setQuestions([...questions, { id: crypto.randomUUID(), question: '' }]);
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
        await createMutation.mutateAsync({
          questionSet: questionSetData,
          orgId: orgId || undefined,
        });
        snackbar.success('Question set created successfully');
      }

      onClose();
    } catch (err) {
      console.error('Error saving question set:', err);
      setError(err instanceof Error ? err.message : 'Failed to save question set');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    if (!isSubmitting) {
      setError(null);
      onClose();
    }
  };

  if (isLoadingQuestionSet && questionSetId) {
    return (
      <DarkDialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
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
      maxWidth="md"
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
          disabled={isSubmitting}
          sx={{ color: '#A0AEC0' }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3 }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
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
            disabled={isSubmitting}
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
            disabled={isSubmitting}
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
                disabled={isSubmitting}
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
                      disabled={isSubmitting}
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
                    disabled={isSubmitting || questions.length === 1}
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
      </DialogContent>

      <DialogActions
        sx={{
          p: 3,
          display: 'flex',
          justifyContent: 'flex-end',
          alignItems: 'center',
          gap: 1,
        }}
      >
        <Button
          variant="outlined"
          onClick={handleClose}
          disabled={isSubmitting}
          sx={{
            color: '#A0AEC0',
            borderColor: '#2D3748',
            '&:hover': {
              borderColor: '#4A5568',
              backgroundColor: 'rgba(255, 255, 255, 0.05)',
            },
          }}
        >
          Cancel
        </Button>
        <Button
          variant="outlined"
          onClick={handleSave}
          color="secondary"
          disabled={
            isSubmitting ||
            !name.trim() ||
            !description.trim() ||
            questions.filter((q) => q.question.trim()).length === 0
          }
          startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
          sx={{
            borderColor: '#2D3748',
            color: '#A0AEC0',
            '&:hover': {
              borderColor: '#4A5568',
              backgroundColor: 'rgba(255, 255, 255, 0.05)',
            },
            '&.Mui-disabled': {
              borderColor: '#2D3748',
              color: '#4A5568',
            },
          }}
        >
          {isSubmitting ? 'Saving...' : questionSetId ? 'Save' : 'Create'}
        </Button>
      </DialogActions>
    </DarkDialog>
  );
};

export default QuestionSetDialog;

