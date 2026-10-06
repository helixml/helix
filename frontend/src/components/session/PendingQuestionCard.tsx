import { useEffect, useRef, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import IconButton from "@mui/material/IconButton";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { alpha, useTheme } from "@mui/material/styles";
import { Check, ChevronDown, ChevronRight, X } from "lucide-react";

import type { TypesPendingQuestion } from "../../api/api";
import useSnackbar from "../../hooks/useSnackbar";
import {
  useCancelAgentQuestion,
  useRespondToAgentQuestion,
} from "../../services/agentQuestionService";
import { getChatColors } from "./chatStyles";

export default function PendingQuestionCard({
  interactionId,
  pendingQuestion,
  attachedAbove = false,
}: {
  interactionId: string;
  pendingQuestion: TypesPendingQuestion;
  attachedAbove?: boolean;
}) {
  const theme = useTheme();
  const colors = getChatColors(theme);
  const snackbar = useSnackbar();
  const respond = useRespondToAgentQuestion();
  const cancel = useCancelAgentQuestion();
  const [expanded, setExpanded] = useState(true);
  const [questionIndex, setQuestionIndex] = useState(0);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [customAnswer, setCustomAnswer] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [optimisticSingleSelect, setOptimisticSingleSelect] = useState("");
  const autoAdvanceTimerRef = useRef<number | null>(null);
  const numberSelectionRef = useRef<(optionIndex: number) => void>(() => {});

  useEffect(() => {
    if (autoAdvanceTimerRef.current !== null) {
      window.clearTimeout(autoAdvanceTimerRef.current);
      autoAdvanceTimerRef.current = null;
    }
    setExpanded(true);
    setQuestionIndex(0);
    setAnswers({});
    setCustomAnswer("");
    setSubmitted(false);
    setOptimisticSingleSelect("");
  }, [pendingQuestion.request_id]);

  useEffect(
    () => () => {
      if (autoAdvanceTimerRef.current !== null) {
        window.clearTimeout(autoAdvanceTimerRef.current);
      }
    },
    [],
  );

  const questions = pendingQuestion.questions ?? [];
  const question = questions[questionIndex];
  const questionId = question?.id ?? "";
  const optionCount = question?.options?.length ?? 0;
  const requestId = pendingQuestion.request_id ?? "";
  const isPending = respond.isPending || cancel.isPending || submitted;

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
    if (!questionId || !answer.trim()) return;
    const nextAnswers = { ...answers, [questionId]: answer };
    setAnswers(nextAnswers);
    setCustomAnswer("");
    setOptimisticSingleSelect("");
    if (questionIndex < questions.length - 1) {
      setQuestionIndex(questionIndex + 1);
      setExpanded(true);
      return;
    }
    submitAnswers(nextAnswers);
  };

  const selectedValues = new Set(
    (answers[questionId] ?? "").split("\n").filter(Boolean),
  );
  const toggleOption = (label: string) => {
    if (!questionId) return;
    const next = new Set(selectedValues);
    if (next.has(label)) next.delete(label);
    else next.add(label);
    setAnswers({
      ...answers,
      [questionId]: Array.from(next).join("\n"),
    });
    setCustomAnswer("");
  };

  const selectOption = (label: string) => {
    if (!question || !label) return;
    if (question.multi_select) {
      toggleOption(label);
      return;
    }
    setOptimisticSingleSelect(label);
    if (autoAdvanceTimerRef.current !== null) {
      window.clearTimeout(autoAdvanceTimerRef.current);
    }
    autoAdvanceTimerRef.current = window.setTimeout(() => {
      autoAdvanceTimerRef.current = null;
      completeQuestion(label);
    }, 200);
  };

  const cancelQuestion = () => {
    if (autoAdvanceTimerRef.current !== null) {
      window.clearTimeout(autoAdvanceTimerRef.current);
      autoAdvanceTimerRef.current = null;
    }
    cancel.mutate(
      { interactionId, requestId },
      {
        onSuccess: () => setSubmitted(true),
        onError: () => snackbar.error("Failed to cancel the question"),
      },
    );
  };

  numberSelectionRef.current = (optionIndex: number) => {
    const label = question?.options?.[optionIndex]?.label ?? "";
    selectOption(label);
  };

  useEffect(() => {
    if (!expanded || isPending) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target;
      if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) return;
      if (
        target instanceof HTMLElement &&
        target.closest('[contenteditable]:not([contenteditable="false"])')
      ) return;
      const optionNumber = Number.parseInt(event.key, 10);
      if (Number.isNaN(optionNumber) || optionNumber < 1 || optionNumber > 9) return;
      event.preventDefault();
      numberSelectionRef.current(optionNumber - 1);
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [expanded, isPending, requestId, questionId, optionCount]);

  if (!question || !interactionId || !requestId) return null;

  return (
    <Box
      data-composer-pending-question="true"
      sx={{
        position: "relative",
        zIndex: 0,
        overflow: "hidden",
        border: "1px solid",
        borderBottom: 0,
        borderColor: colors.border,
        borderRadius: attachedAbove ? 0 : "20px 20px 0 0",
        backgroundColor: colors.composerSurface,
        color: colors.foreground,
      }}
    >
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "minmax(0, 1fr) auto",
          alignItems: "center",
          minHeight: 32,
          px: 0.5,
          py: 0.5,
        }}
      >
        <Box
          component="button"
          type="button"
          aria-expanded={expanded}
          title={expanded ? "Hide the question and its options" : "Show the question and its options"}
          onClick={() => setExpanded((value) => !value)}
          sx={{
            minWidth: 0,
            alignSelf: "stretch",
            display: "grid",
            gridTemplateColumns: "24px minmax(0, 1fr) auto",
            alignItems: "center",
            gap: 0.5,
            p: 0,
            border: 0,
            borderRadius: "8px",
            background: "transparent",
            color: "inherit",
            font: "inherit",
            textAlign: "left",
            cursor: "pointer",
            "&:hover": {
              backgroundColor: alpha(theme.palette.text.primary, 0.025),
            },
            "&:focus-visible": {
              outline: `2px solid ${theme.palette.primary.main}`,
              outlineOffset: -2,
            },
          }}
        >
          <Box component="span" aria-hidden sx={{ width: 24 }} />
          <Box
            component="span"
            sx={{
              minWidth: 0,
              display: "flex",
              alignItems: "center",
              gap: 0.75,
            }}
          >
            <Typography
              component="span"
              variant="caption"
              sx={{ flexShrink: 0, color: colors.subtle, fontWeight: 500 }}
            >
              {question.header || "User input needed"}
            </Typography>
            {!expanded && (
              <Typography
                component="span"
                variant="caption"
                sx={{
                  minWidth: 0,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                  color: colors.subtle,
                  opacity: 0.72,
                }}
              >
                {question.question}
              </Typography>
            )}
          </Box>
          <Stack direction="row" alignItems="center" spacing={0.5} sx={{ pr: 0.25 }}>
            {questions.length > 1 && (
              <Typography
                component="span"
                variant="caption"
                sx={{
                  color: colors.subtle,
                  fontWeight: 500,
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                {questionIndex + 1}/{questions.length}
              </Typography>
            )}
            {expanded ? (
              <ChevronDown aria-hidden size={14} color={colors.subtle} />
            ) : (
              <ChevronRight aria-hidden size={14} color={colors.subtle} />
            )}
          </Stack>
        </Box>
        <Tooltip title="Dismiss question without answering">
          <span>
            <IconButton
              aria-label="Dismiss question without answering"
              disabled={isPending}
              onClick={cancelQuestion}
              sx={{ width: 30, height: 30, color: colors.subtle }}
            >
              <X size={18} />
            </IconButton>
          </span>
        </Tooltip>
      </Box>

      <Collapse in={expanded}>
        <Box sx={{ pl: 4, pr: 1.5, pb: 1.25 }}>
          <Typography variant="body2" sx={{ color: alpha(colors.foreground, 0.85) }}>
            {question.question}
          </Typography>
          {question.multi_select && (
            <Typography
              variant="caption"
              sx={{ display: "block", mt: 0.25, color: colors.subtle }}
            >
              Select one or more options.
            </Typography>
          )}

          <Stack spacing={0.25} sx={{ mt: 1 }}>
            {question.options?.map((option, optionIndex) => {
              const label = option.label ?? "";
              const selected =
                optimisticSingleSelect === label ||
                (selectedValues.has(label) && !customAnswer.trim());
              return (
                <Box
                  component="button"
                  type="button"
                  key={`${questionId}:${optionIndex}:${label}`}
                  aria-pressed={selected}
                  disabled={isPending || !label}
                  onClick={() => selectOption(label)}
                  sx={{
                    width: "100%",
                    minWidth: 0,
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    px: 1.25,
                    py: 1,
                    border: 0,
                    borderRadius: "6px",
                    backgroundColor: selected
                      ? alpha(theme.palette.text.primary, 0.07)
                      : "transparent",
                    color: alpha(colors.foreground, 0.85),
                    font: "inherit",
                    textAlign: "left",
                    cursor: isPending ? "not-allowed" : "pointer",
                    transition: "background-color 150ms ease, opacity 150ms ease",
                    opacity: isPending ? 0.5 : 1,
                    "&:hover": {
                      backgroundColor: alpha(theme.palette.text.primary, selected ? 0.09 : 0.04),
                    },
                    "&:focus-visible": {
                      outline: `1px solid ${alpha(theme.palette.primary.main, 0.4)}`,
                      outlineOffset: -1,
                    },
                  }}
                >
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      {label}
                    </Typography>
                    {option.description && option.description !== label && (
                      <Typography
                        variant="caption"
                        sx={{ display: "block", color: colors.subtle }}
                      >
                        {option.description}
                      </Typography>
                    )}
                  </Box>
                  {selected ? (
                    <Check aria-hidden size={14} color={theme.palette.primary.main} />
                  ) : optionIndex < 9 ? (
                    <Typography
                      component="kbd"
                      variant="caption"
                      sx={{
                        width: 20,
                        height: 20,
                        flexShrink: 0,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        color: colors.subtle,
                        fontFamily: "inherit",
                        fontWeight: 500,
                        fontVariantNumeric: "tabular-nums",
                      }}
                    >
                      {optionIndex + 1}
                    </Typography>
                  ) : null}
                </Box>
              );
            })}
          </Stack>

          {question.allow_custom_answer && (
            <TextField
              fullWidth
              size="small"
              placeholder="Write custom answer"
              value={customAnswer}
              disabled={isPending}
              onChange={(event) => {
                if (autoAdvanceTimerRef.current !== null) {
                  window.clearTimeout(autoAdvanceTimerRef.current);
                  autoAdvanceTimerRef.current = null;
                }
                setCustomAnswer(event.target.value);
                setOptimisticSingleSelect("");
                if (event.target.value.trim()) {
                  setAnswers({ ...answers, [questionId]: "" });
                }
              }}
              sx={{
                mt: 1,
                "& .MuiOutlinedInput-root": {
                  borderRadius: "8px",
                  backgroundColor: alpha(theme.palette.background.default, 0.45),
                },
              }}
            />
          )}

          {(question.multi_select || question.allow_custom_answer) && (
            <Stack direction="row" justifyContent="flex-end" sx={{ mt: 0.75 }}>
              <Button
                size="small"
                variant="contained"
                disabled={
                  isPending || (!customAnswer.trim() && selectedValues.size === 0)
                }
                onClick={() =>
                  completeQuestion(
                    customAnswer.trim() || Array.from(selectedValues).join("\n"),
                  )
                }
                sx={{ minHeight: 30, borderRadius: 999, px: 1.5, textTransform: "none" }}
              >
                {questionIndex < questions.length - 1 ? "Next" : "Submit"}
              </Button>
            </Stack>
          )}

          {submitted && (
            <Typography
              variant="caption"
              sx={{ display: "block", mt: 0.75, color: colors.subtle }}
            >
              Response sent. Waiting for the agent…
            </Typography>
          )}
        </Box>
      </Collapse>
    </Box>
  );
}
