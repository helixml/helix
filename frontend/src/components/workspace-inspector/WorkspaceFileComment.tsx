import { memo, useCallback, useState, type ReactNode } from "react";
import {
  Box,
  Button,
  IconButton,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { MessageSquare, Trash2 } from "lucide-react";

export interface WorkspaceFileCommentEntry {
  id: string;
  kind: "draft" | "comment";
  startLine: number;
  endLine: number;
  text: string;
}

interface WorkspaceFileCommentProps {
  entry: WorkspaceFileCommentEntry;
  onCancel: (entryId: string) => void;
  onSubmit: (entry: WorkspaceFileCommentEntry, text: string) => void;
}

const WorkspaceFileComment = memo(function WorkspaceFileComment({
  entry,
  onCancel,
  onSubmit,
}: WorkspaceFileCommentProps) {
  const [draftText, setDraftText] = useState(entry.text);
  const submit = useCallback(() => {
    const text = draftText.trim();
    if (text) onSubmit(entry, text);
  }, [draftText, entry, onSubmit]);
  const lineLabel = entry.startLine === entry.endLine
    ? `L${entry.startLine}`
    : `L${entry.startLine}–${entry.endLine}`;

  const cardSx = {
    mx: 1.5,
    my: 0.5,
    maxWidth: 620,
    border: "1px solid",
    borderColor: "divider",
    borderRadius: 1,
    bgcolor: "background.paper",
    boxShadow: "0 6px 20px rgba(0, 0, 0, 0.14)",
    overflow: "hidden",
  } as const;

  const header = (title: string, action?: ReactNode) => (
    <Box
      sx={{
        minHeight: 36,
        px: 1.25,
        display: "flex",
        alignItems: "center",
        gap: 0.75,
        borderBottom: "1px solid",
        borderColor: "divider",
        bgcolor: "action.hover",
      }}
    >
      <Box
        sx={{
          width: 22,
          height: 22,
          display: "grid",
          placeItems: "center",
          borderRadius: "50%",
          color: "text.secondary",
          bgcolor: "action.selected",
          flexShrink: 0,
        }}
      >
        <MessageSquare size={13} />
      </Box>
      <Typography variant="caption" sx={{ fontWeight: 600, color: "text.primary" }}>
        {title}
      </Typography>
      <Typography
        variant="caption"
        sx={{
          px: 0.625,
          py: 0.125,
          borderRadius: 0.5,
          color: "text.secondary",
          bgcolor: "action.selected",
          fontFamily: "monospace",
          fontSize: "0.68rem",
          lineHeight: 1.4,
        }}
      >
        {lineLabel}
      </Typography>
      <Box sx={{ flex: 1 }} />
      {action}
    </Box>
  );

  if (entry.kind === "comment") {
    return (
      <Box
        data-file-comment
        sx={cardSx}
      >
        {header("File comment", (
          <Tooltip title="Delete comment">
            <IconButton
              size="small"
              aria-label="Delete comment"
              onClick={() => onCancel(entry.id)}
              sx={{ width: 28, height: 28, color: "text.secondary" }}
            >
              <Trash2 size={14} />
            </IconButton>
          </Tooltip>
        ))}
        <Typography
          variant="body2"
          sx={{ px: 1.5, py: 1.25, whiteSpace: "pre-wrap", lineHeight: 1.55 }}
        >
          {entry.text}
        </Typography>
      </Box>
    );
  }

  return (
    <Box
      data-file-comment-draft
      sx={cardSx}
    >
      {header("Add a comment")}
      <Box sx={{ p: 1.25 }}>
        <TextField
          autoFocus
          fullWidth
          multiline
          minRows={2}
          size="small"
          value={draftText}
          placeholder="Leave feedback for the agent…"
          inputProps={{
            "aria-label": `Comment on ${entry.startLine === entry.endLine
              ? `line ${entry.startLine}`
              : `lines ${entry.startLine} to ${entry.endLine}`}`,
          }}
          onChange={(event) => setDraftText(event.target.value)}
          onKeyDown={(event) => {
            event.stopPropagation();
            if (event.key === "Escape") onCancel(entry.id);
            if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
              event.preventDefault();
              submit();
            }
          }}
          sx={{
            "& .MuiOutlinedInput-root": {
              alignItems: "flex-start",
              bgcolor: "background.default",
              fontSize: "0.875rem",
              lineHeight: 1.5,
            },
          }}
        />
        <Box
          sx={{
            mt: 1,
            display: "flex",
            alignItems: "center",
            justifyContent: "flex-end",
            gap: 0.75,
          }}
        >
          <Typography variant="caption" color="text.secondary" sx={{ mr: "auto" }}>
            {navigator.platform.includes("Mac") ? "⌘ Enter" : "Ctrl Enter"} to add
          </Typography>
          <Button size="small" color="inherit" onClick={() => onCancel(entry.id)}>
            Cancel
          </Button>
          <Button
            size="small"
            variant="contained"
            disabled={!draftText.trim()}
            onClick={submit}
            sx={{ textTransform: "none", px: 1.25 }}
          >
            Add comment
          </Button>
        </Box>
      </Box>
    </Box>
  );
});

export default WorkspaceFileComment;
