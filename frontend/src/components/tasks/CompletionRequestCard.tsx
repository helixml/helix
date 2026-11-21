import { useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import Stack from "@mui/material/Stack";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import { alpha, useTheme } from "@mui/material/styles";
import { ChevronDown, ChevronRight, Flag } from "lucide-react";

import useSnackbar from "../../hooks/useSnackbar";
import { useDecideCompletion } from "../../services/specTaskCompletionService";
import { useCollapseOnUserMessage } from "../session/actionCardCollapse";
import { getChatColors } from "../session/chatStyles";

/**
 * The agent's request to mark its task done (mark_task_complete), shown in the
 * chat thread. Marking done stops the agent's desktop; sending back passes the
 * note to the agent as feedback. The header folds the card down to one line
 * (with a preview of the summary) and it folds itself when the user sends a
 * chat message, like PRProposalCard.
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
  const [expanded, setExpanded] = useState(true);
  useCollapseOnUserMessage(() => setExpanded(false));

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
        overflow: "hidden",
        flexShrink: 0,
        border: "1px solid",
        borderColor: colors.border,
        borderRadius: "16px",
        backgroundColor: colors.composerSurface,
        color: colors.foreground,
      }}
    >
      <Box sx={{ p: 0.5 }}>
        <Box
          component="button"
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
          sx={{
            width: "100%",
            minHeight: 32,
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
            "&:hover": { backgroundColor: alpha(theme.palette.text.primary, 0.025) },
            "&:focus-visible": {
              outline: `2px solid ${theme.palette.primary.main}`,
              outlineOffset: -2,
            },
          }}
        >
          <Box component="span" sx={{ display: "flex", justifyContent: "center" }}>
            <Flag aria-hidden size={14} color={theme.palette.success.main} />
          </Box>
          <Box component="span" sx={{ minWidth: 0, display: "flex", alignItems: "center", gap: 0.75 }}>
            <Typography component="span" variant="caption" sx={{ flexShrink: 0, color: colors.subtle, fontWeight: 500 }}>
              Agent says the task is finished
            </Typography>
            {!expanded && summary && (
              <Typography
                component="span"
                variant="caption"
                sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", color: colors.subtle, opacity: 0.72 }}
              >
                {summary}
              </Typography>
            )}
          </Box>
          <Box component="span" sx={{ pr: 0.25, display: "flex" }}>
            {expanded ? (
              <ChevronDown aria-hidden size={14} color={colors.subtle} />
            ) : (
              <ChevronRight aria-hidden size={14} color={colors.subtle} />
            )}
          </Box>
        </Box>
      </Box>

      <Collapse in={expanded}>
        <Box sx={{ pl: 4, pr: 1.5, pb: 1.25 }}>
          <Typography
            variant="body2"
            sx={{ color: alpha(colors.foreground, 0.85), whiteSpace: "pre-wrap" }}
          >
            {summary}
          </Typography>
          <Stack spacing={1} sx={{ mt: 1 }}>
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
      </Collapse>
    </Box>
  );
}
