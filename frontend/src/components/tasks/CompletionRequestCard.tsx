import { useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { alpha, useTheme } from "@mui/material/styles";
import { Flag } from "lucide-react";

import useSnackbar from "../../hooks/useSnackbar";
import { useDecideCompletion } from "../../services/specTaskCompletionService";
import { getChatColors } from "../session/chatStyles";

/**
 * The agent's request to mark its task done (mark_task_complete), shown in the
 * chat thread. Marking done stops the agent's desktop; sending back passes the
 * note to the agent as feedback.
 */
export default function CompletionRequestCard({
  specTaskId,
  summary,
  openPullRequests,
}: {
  specTaskId: string;
  summary: string;
  openPullRequests: number;
}) {
  const theme = useTheme();
  const colors = getChatColors(theme);
  const snackbar = useSnackbar();
  const decide = useDecideCompletion(specTaskId);
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");

  const submit = (decision: "approve" | "reject") => {
    setError("");
    decide.mutate(
      { decision, comment: decision === "reject" ? comment.trim() || undefined : undefined },
      {
        onSuccess: () => {
          setComment("");
          snackbar.success(
            decision === "approve"
              ? "Task marked done"
              : "Sent back — the agent has your feedback",
          );
        },
        onError: (err: any) => {
          const data = err?.response?.data;
          setError(typeof data === "string" ? data : data?.message || err?.message || "Request failed");
        },
      },
    );
  };

  const busy = decide.isPending;
  const fieldSx = {
    "& .MuiOutlinedInput-root": {
      borderRadius: "8px",
      backgroundColor: alpha(theme.palette.background.default, 0.45),
    },
    "& .MuiInputBase-input": { fontSize: "0.875rem" },
  };

  return (
    <Box
      data-composer-completion-request={specTaskId}
      sx={{
        flexShrink: 0,
        border: "1px solid",
        borderColor: colors.border,
        borderRadius: "16px",
        backgroundColor: colors.composerSurface,
        color: colors.foreground,
        px: 1.5,
        py: 1.25,
      }}
    >
      <Stack direction="row" alignItems="center" spacing={0.75}>
        <Flag aria-hidden size={14} color={theme.palette.success.main} />
        <Typography variant="caption" sx={{ color: colors.subtle, fontWeight: 500 }}>
          Agent says the task is finished
        </Typography>
      </Stack>
      <Typography
        variant="body2"
        sx={{ mt: 0.75, pl: 2.75, color: alpha(colors.foreground, 0.85), whiteSpace: "pre-wrap" }}
      >
        {summary}
      </Typography>
      <Stack spacing={1} sx={{ mt: 1, pl: 2.75 }}>
        {openPullRequests > 0 && (
          <Typography variant="caption" sx={{ color: theme.palette.warning.main }}>
            {openPullRequests === 1
              ? "1 pull request is still open. Marking the task done leaves it open on the provider."
              : `${openPullRequests} pull requests are still open. Marking the task done leaves them open on the provider.`}
          </Typography>
        )}
        <TextField
          size="small"
          label="What's missing? (sent to the agent if you send it back)"
          value={comment}
          disabled={busy}
          multiline
          maxRows={6}
          onChange={(e) => setComment(e.target.value)}
          sx={fieldSx}
        />
        {error && (
          <Typography variant="caption" sx={{ color: theme.palette.error.main }}>
            {error}
          </Typography>
        )}
        <Stack direction="row" justifyContent="flex-end" alignItems="center" spacing={0.75}>
          <Typography variant="caption" sx={{ color: colors.subtle, mr: "auto" }}>
            Marking done stops the agent's desktop.
          </Typography>
          <Button
            size="small"
            disabled={busy}
            onClick={() => submit("reject")}
            sx={{ minHeight: 30, borderRadius: 999, px: 1.5, textTransform: "none", color: colors.subtle, whiteSpace: "nowrap", flexShrink: 0 }}
          >
            Send back
          </Button>
          <Button
            size="small"
            variant="contained"
            color="success"
            disabled={busy}
            onClick={() => submit("approve")}
            sx={{ minHeight: 30, borderRadius: 999, px: 1.5, textTransform: "none", whiteSpace: "nowrap", flexShrink: 0 }}
          >
            {busy ? "Working…" : "Mark done"}
          </Button>
        </Stack>
      </Stack>
    </Box>
  );
}
