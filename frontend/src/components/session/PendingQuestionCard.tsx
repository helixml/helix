import { useEffect, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import IconButton from "@mui/material/IconButton";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import {
  CheckSquare2,
  ChevronDown,
  ChevronUp,
  MessageCircleQuestion,
  Square,
} from "lucide-react";

import type { TypesPendingQuestion } from "../../api/api";
import useSnackbar from "../../hooks/useSnackbar";
import {
  useCancelAgentQuestion,
  useRespondToAgentQuestion,
} from "../../services/agentQuestionService";

export default function PendingQuestionCard({
  interactionId,
  pendingQuestion,
}: {
  interactionId: string;
  pendingQuestion: TypesPendingQuestion;
}) {
  const snackbar = useSnackbar();
  const respond = useRespondToAgentQuestion();
  const cancel = useCancelAgentQuestion();
  const [expanded, setExpanded] = useState(true);
  const [questionIndex, setQuestionIndex] = useState(0);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [customAnswer, setCustomAnswer] = useState("");
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    setExpanded(true);
    setQuestionIndex(0);
    setAnswers({});
    setCustomAnswer("");
    setSubmitted(false);
  }, [pendingQuestion.request_id]);

  const questions = pendingQuestion.questions ?? [];
  const question = questions[questionIndex];
  const requestId = pendingQuestion.request_id ?? "";
  const isPending = respond.isPending || cancel.isPending || submitted;

  if (!question || !interactionId || !requestId) return null;

  const submitAnswers = (nextAnswers: Record<string, string>) => {
    respond.mutate(
      { interactionId, requestId, answers: nextAnswers },
      {
        onSuccess: () => setSubmitted(true),
        onError: () => snackbar.error("Failed to send the answer to the agent"),
      },
    );
  };

  const completeQuestion = (answer: string) => {
    const id = question.id ?? "";
    if (!id || !answer.trim()) return;
    const nextAnswers = { ...answers, [id]: answer };
    setAnswers(nextAnswers);
    setCustomAnswer("");
    if (questionIndex < questions.length - 1) {
      setQuestionIndex(questionIndex + 1);
      return;
    }
    submitAnswers(nextAnswers);
  };

  const selectedValues = new Set(
    (answers[question.id ?? ""] ?? "").split("\n").filter(Boolean),
  );
  const toggleOption = (label: string) => {
    const next = new Set(selectedValues);
    if (next.has(label)) next.delete(label);
    else next.add(label);
    setAnswers({
      ...answers,
      [question.id ?? ""]: Array.from(next).join("\n"),
    });
  };

  return (
    <Box
      sx={{
        width: "100%",
        border: "1px solid",
        borderColor: "divider",
        borderRadius: 1,
        p: 1.5,
        mt: 1,
        backgroundColor: "background.paper",
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1}>
        <MessageCircleQuestion size={18} />
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="subtitle2">Agent needs your input</Typography>
          {questions.length > 1 && (
            <Typography variant="caption" color="text.secondary">
              Question {questionIndex + 1} of {questions.length}
            </Typography>
          )}
        </Box>
        <Tooltip title={expanded ? "Collapse question" : "Expand question"}>
          <IconButton
            aria-label={expanded ? "Collapse question" : "Expand question"}
            size="small"
            onClick={() => setExpanded(!expanded)}
            sx={{ width: 30, height: 30, color: "text.secondary" }}
          >
            {expanded ? <ChevronUp size={18} /> : <ChevronDown size={18} />}
          </IconButton>
        </Tooltip>
      </Stack>

      <Collapse in={expanded}>
        {submitted && (
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: "block", mt: 1 }}
          >
            Answer sent. Waiting for the agent to continue…
          </Typography>
        )}
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: "block", mt: 1 }}
        >
          {question.header}
        </Typography>
        <Typography variant="body2" sx={{ mt: 0.25, mb: 1 }}>
          {question.question}
        </Typography>

        <Stack spacing={0.75}>
          {question.options?.map((option) => {
            const label = option.label ?? "";
            const selected = selectedValues.has(label);
            return (
              <Button
                key={label}
                variant={selected ? "contained" : "outlined"}
                color="inherit"
                disabled={isPending || !label}
                onClick={() =>
                  question.multi_select
                    ? toggleOption(label)
                    : completeQuestion(label)
                }
                sx={{
                  justifyContent: "flex-start",
                  textAlign: "left",
                  py: 0.75,
                }}
                startIcon={
                  question.multi_select ? (
                    selected ? <CheckSquare2 size={18} /> : <Square size={18} />
                  ) : undefined
                }
              >
                <Box>
                  <Typography variant="body2">{label}</Typography>
                  {option.description && (
                    <Typography variant="caption" color="text.secondary">
                      {option.description}
                    </Typography>
                  )}
                </Box>
              </Button>
            );
          })}
        </Stack>

        {question.allow_custom_answer && (
          <TextField
            fullWidth
            size="small"
            label="Other"
            value={customAnswer}
            disabled={isPending}
            onChange={(event) => setCustomAnswer(event.target.value)}
            sx={{ mt: 1 }}
          />
        )}

        <Stack
          direction="row"
          spacing={1}
          justifyContent="flex-end"
          sx={{ mt: 1 }}
        >
          <Button
            color="inherit"
            disabled={isPending}
            onClick={() =>
              cancel.mutate(
                { interactionId, requestId },
                {
                  onSuccess: () => setSubmitted(true),
                  onError: () =>
                    snackbar.error("Failed to cancel the question"),
                },
              )
            }
          >
            Cancel
          </Button>
          {(question.multi_select || question.allow_custom_answer) && (
            <Button
              variant="contained"
              disabled={
                isPending || (!customAnswer.trim() && selectedValues.size === 0)
              }
              onClick={() =>
                completeQuestion(
                  customAnswer.trim() || Array.from(selectedValues).join("\n"),
                )
              }
            >
              {questionIndex < questions.length - 1 ? "Next" : "Submit"}
            </Button>
          )}
        </Stack>
      </Collapse>
    </Box>
  );
}
