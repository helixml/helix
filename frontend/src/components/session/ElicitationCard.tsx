import React, { FC, useMemo, useState } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Checkbox from "@mui/material/Checkbox";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { CircleHelp, Check, MessageSquareReply } from "lucide-react";

import {
  ElicitationDraft,
  ElicitationField,
  buildElicitationContent,
  canSubmitElicitation,
  parseElicitationSchema,
  summariseAnswers,
} from "./elicitationSchema";

/** Question payload as it arrives on a response entry. */
export interface ElicitationPayload {
  id: string;
  tool_call_id?: string;
  message: string;
  mode?: string;
  schema?: unknown;
  status: string;
  content?: Record<string, unknown>;
  resolution_reason?: string;
}

interface ElicitationCardProps {
  elicitation: ElicitationPayload;
  /** Submits the answer. Rejects if the question is no longer answerable. */
  onRespond?: (
    elicitationId: string,
    action: "accept" | "decline",
    content: Record<string, unknown>,
  ) => Promise<void>;
  /** True while the user's own follow-up message is superseding this question. */
  supersededByFollowUp?: boolean;
}

const isLive = (status: string) => status === "pending" || status === "submitting";

/**
 * Explains a resolved question. "Cancelled" alone is unhelpful — the user wants to know
 * whether they replied instead, whether the agent moved on, or whether it expired.
 */
const resolutionText = (status: string, reason?: string): string => {
  if (reason === "follow_up") return "You replied with a message instead.";
  if (reason === "agent_no_longer_holds")
    return "This question expired — the agent restarted before it was answered.";
  if (reason === "interrupted") return "The turn ended before this was answered.";
  switch (status) {
    case "declined":
      return "You skipped this question — the agent continued without an answer.";
    case "accepted":
    case "completed":
      return "Answered.";
    case "cancelled":
      return "This question is no longer awaiting an answer.";
    default:
      return "";
  }
};

const OptionButton: FC<{
  selected: boolean;
  disabled: boolean;
  label: string;
  description?: string;
  multi: boolean;
  onClick: () => void;
}> = ({ selected, disabled, label, description, multi, onClick }) => (
  <Box
    component="button"
    type="button"
    disabled={disabled}
    onClick={onClick}
    sx={{
      display: "flex",
      alignItems: "flex-start",
      gap: 1,
      width: "100%",
      textAlign: "left",
      p: 1.25,
      borderRadius: 1,
      cursor: disabled ? "default" : "pointer",
      background: selected ? "rgba(255,255,255,0.06)" : "transparent",
      border: "1px solid",
      borderColor: selected ? "primary.main" : "rgba(255,255,255,0.12)",
      color: "inherit",
      font: "inherit",
      "&:hover": disabled ? {} : { borderColor: "primary.light" },
    }}
  >
    {multi ? (
      <Checkbox checked={selected} disabled={disabled} size="small" sx={{ p: 0, mt: 0.25 }} />
    ) : (
      <Box
        sx={{
          width: 16,
          height: 16,
          mt: 0.25,
          flexShrink: 0,
          borderRadius: "50%",
          border: "1px solid",
          borderColor: selected ? "primary.main" : "rgba(255,255,255,0.4)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {selected && (
          <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "primary.main" }} />
        )}
      </Box>
    )}
    <Box>
      <Typography variant="body2" sx={{ fontWeight: 500 }}>
        {label}
      </Typography>
      {description && (
        <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
          {description}
        </Typography>
      )}
    </Box>
  </Box>
);

const FieldControl: FC<{
  field: ElicitationField;
  draft: ElicitationDraft;
  disabled: boolean;
  onChange: (next: ElicitationDraft) => void;
}> = ({ field, draft, disabled, onChange }) => {
  const value = draft.values[field.name];

  const setValue = (next: string | string[] | boolean) =>
    onChange({ ...draft, values: { ...draft.values, [field.name]: next } });

  const toggleMulti = (optionValue: string) => {
    const current = Array.isArray(value) ? value : [];
    setValue(
      current.includes(optionValue)
        ? current.filter((v) => v !== optionValue)
        : [...current, optionValue],
    );
  };

  return (
    <Box sx={{ mb: 2 }}>
      {field.title && (
        <Chip
          label={field.title}
          size="small"
          sx={{ mb: 0.75, height: 20, fontSize: "0.7rem" }}
        />
      )}
      {field.description && (
        <Typography variant="body2" sx={{ mb: 1 }}>
          {field.description}
        </Typography>
      )}

      {field.options.length > 0 ? (
        <Stack spacing={0.75}>
          {field.options.map((option) => (
            <OptionButton
              key={option.value}
              multi={field.kind === "multiselect"}
              selected={
                field.kind === "multiselect"
                  ? Array.isArray(value) && value.includes(option.value)
                  : value === option.value
              }
              disabled={disabled}
              label={option.label}
              description={option.description || option.preview}
              onClick={() =>
                field.kind === "multiselect" ? toggleMulti(option.value) : setValue(option.value)
              }
            />
          ))}
        </Stack>
      ) : field.kind === "boolean" ? (
        <Checkbox
          checked={value === true}
          disabled={disabled}
          onChange={(e) => setValue(e.target.checked)}
        />
      ) : (
        <TextField
          fullWidth
          size="small"
          disabled={disabled}
          value={typeof value === "string" ? value : ""}
          onChange={(e) => setValue(e.target.value)}
        />
      )}

      {/* The "Other" box. A non-empty answer here replaces the selection entirely —
          that is the adapter's rule, and it is what makes "none of the above" work. */}
      {field.customAnswerField && (
        <TextField
          fullWidth
          size="small"
          sx={{ mt: 1 }}
          disabled={disabled}
          label={field.customAnswerField.title || "Other"}
          placeholder={field.customAnswerField.description}
          value={draft.customValues[field.name] ?? ""}
          onChange={(e) =>
            onChange({
              ...draft,
              customValues: { ...draft.customValues, [field.name]: e.target.value },
            })
          }
        />
      )}
    </Box>
  );
};

/**
 * An agent's question, rendered inline in the conversation.
 *
 * The whole form is built generically from the elicitation's JSON Schema, so shapes other
 * than Claude Code's AskUserQuestion (MCP forwarding, the refusal-fallback consent
 * dialog, other ACP agents) render as best they can instead of crashing the view.
 */
const ElicitationCard: FC<ElicitationCardProps> = ({
  elicitation,
  onRespond,
  supersededByFollowUp,
}) => {
  const form = useMemo(
    () => parseElicitationSchema(elicitation.schema),
    [elicitation.schema],
  );
  const [draft, setDraft] = useState<ElicitationDraft>({ values: {}, customValues: {} });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | undefined>();

  const live = isLive(elicitation.status) && !supersededByFollowUp;
  const disabled = !live || submitting || !onRespond;

  const submit = async (action: "accept" | "decline") => {
    if (!onRespond) return;
    setSubmitting(true);
    setError(undefined);
    try {
      await onRespond(
        elicitation.id,
        action,
        action === "accept" ? buildElicitationContent(form, draft) : {},
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to send your answer");
    } finally {
      setSubmitting(false);
    }
  };

  const answered = useMemo(() => {
    if (live) return [];
    const content = (elicitation.content as Record<string, never>) || {};
    return summariseAnswers(form, content);
  }, [form, elicitation.content, live]);

  return (
    <Box
      sx={{
        my: 1.5,
        p: 2,
        borderRadius: 1,
        border: "1px solid",
        borderColor: live ? "primary.main" : "rgba(255,255,255,0.12)",
        background: "rgba(255,255,255,0.02)",
      }}
    >
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1.5 }}>
        <CircleHelp size={18} />
        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
          {live ? "The agent is asking you a question" : "The agent asked a question"}
        </Typography>
        {submitting && <CircularProgress size={14} />}
      </Stack>

      {elicitation.message && (
        <Typography variant="body2" sx={{ mb: 2 }}>
          {elicitation.message}
        </Typography>
      )}

      {live ? (
        <>
          {form.fields.map((field) => (
            <FieldControl
              key={field.name}
              field={field}
              draft={draft}
              disabled={disabled}
              onChange={setDraft}
            />
          ))}

          {form.fields.length === 0 && (
            <Alert severity="info" sx={{ mb: 2 }}>
              This question arrived in a format Helix does not recognise. Skipping lets the
              agent continue.
            </Alert>
          )}

          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}

          <Stack direction="row" spacing={1}>
            <Button
              variant="contained"
              size="small"
              startIcon={<Check size={16} />}
              disabled={disabled || !canSubmitElicitation(form, draft)}
              onClick={() => submit("accept")}
            >
              Submit
            </Button>
            <Button
              variant="outlined"
              size="small"
              disabled={disabled}
              onClick={() => submit("decline")}
            >
              Skip
            </Button>
            <Typography variant="caption" color="text.secondary" sx={{ alignSelf: "center" }}>
              Skipping lets the agent continue without your answer.
            </Typography>
          </Stack>
        </>
      ) : (
        <Box>
          {answered.length > 0 && (
            <Stack spacing={0.5} sx={{ mb: 1 }}>
              {answered.map((entry) => (
                <Stack key={entry.question} direction="row" spacing={1} alignItems="baseline">
                  <Typography variant="caption" color="text.secondary">
                    {entry.question}:
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    {entry.answer}
                  </Typography>
                </Stack>
              ))}
            </Stack>
          )}
          <Stack direction="row" spacing={1} alignItems="center">
            {supersededByFollowUp ? <MessageSquareReply size={14} /> : <Check size={14} />}
            <Typography variant="caption" color="text.secondary">
              {supersededByFollowUp
                ? "Replying instead…"
                : resolutionText(elicitation.status, elicitation.resolution_reason)}
            </Typography>
          </Stack>
        </Box>
      )}
    </Box>
  );
};

export default ElicitationCard;
