import { memo, useCallback, useState } from "react";
import {
  Box,
  Button,
  IconButton,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { MessageCircle, Trash2 } from "lucide-react";

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

  if (entry.kind === "comment") {
    return (
      <Box
        data-file-comment
        sx={{
          display: "flex",
          alignItems: "flex-start",
          gap: 1,
          px: 1.5,
          py: 1,
          borderLeft: "2px solid",
          borderColor: "primary.main",
          bgcolor: "action.hover",
        }}
      >
        <MessageCircle size={15} style={{ marginTop: 2, flexShrink: 0 }} />
        <Typography variant="body2" sx={{ flex: 1, whiteSpace: "pre-wrap" }}>
          {entry.text}
        </Typography>
        <Tooltip title="Delete comment">
          <IconButton
            size="small"
            aria-label="Delete comment"
            onClick={() => onCancel(entry.id)}
          >
            <Trash2 size={14} />
          </IconButton>
        </Tooltip>
      </Box>
    );
  }

  return (
    <Box
      data-file-comment-draft
      sx={{
        px: 1.5,
        py: 1,
        borderLeft: "2px solid",
        borderColor: "primary.main",
        bgcolor: "action.hover",
      }}
    >
      <TextField
        autoFocus
        fullWidth
        multiline
        minRows={2}
        size="small"
        value={draftText}
        placeholder="Add a comment…"
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
      />
      <Box sx={{ mt: 0.75, display: "flex", justifyContent: "flex-end", gap: 0.75 }}>
        <Button size="small" onClick={() => onCancel(entry.id)}>
          Cancel
        </Button>
        <Button size="small" variant="contained" disabled={!draftText.trim()} onClick={submit}>
          Comment
        </Button>
      </Box>
    </Box>
  );
});

export default WorkspaceFileComment;
