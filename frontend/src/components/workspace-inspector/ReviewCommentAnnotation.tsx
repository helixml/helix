import { useCallback, useState, type ChangeEvent, type KeyboardEvent } from "react";
import {
  alpha,
  Box,
  Button,
  CircularProgress,
  IconButton,
  Tooltip,
  Typography,
} from "@mui/material";
import { MessageCircle, Trash2 } from "lucide-react";

import { APP_FONT_FAMILY, TYPOGRAPHY } from "../../styles/typography";

interface ReviewCommentAnnotationProps {
  kind: "draft" | "comment";
  text: string;
  rangeLabel: string;
  onCancel: () => void;
  onSubmit: (text: string) => void;
  onTextChange?: (text: string) => void;
  submitLabel?: string;
  isSubmitting?: boolean;
  onSend?: (text: string) => void;
  isSending?: boolean;
}

export default function ReviewCommentAnnotation({
  kind,
  text,
  rangeLabel,
  onCancel,
  onSubmit,
  onTextChange,
  submitLabel = "Comment",
  isSubmitting = false,
  onSend,
  isSending = false,
}: ReviewCommentAnnotationProps) {
  const [localText, setLocalText] = useState(text);
  const displayedText = kind === "draft" && !onTextChange ? localText : text;
  const updateText = useCallback((value: string) => {
    if (onTextChange) onTextChange(value);
    else setLocalText(value);
  }, [onTextChange]);
  const submit = useCallback(() => {
    const trimmed = displayedText.trim();
    if (trimmed && !isSubmitting && !isSending) onSubmit(trimmed);
  }, [displayedText, isSending, isSubmitting, onSubmit]);
  const send = useCallback(() => {
    const trimmed = displayedText.trim();
    if (trimmed && onSend && !isSubmitting && !isSending) onSend(trimmed);
  }, [displayedText, isSending, isSubmitting, onSend]);

  if (kind === "comment") {
    return (
      <Box
        data-review-comment-annotation
        contentEditable={false}
        onPointerDown={(event) => event.stopPropagation()}
        sx={{
          display: "flex",
          minWidth: 0,
          alignItems: "flex-start",
          gap: 1.25,
          borderLeft: "2px solid",
          borderColor: (theme) => alpha(theme.palette.primary.main, 0.55),
          bgcolor: (theme) => alpha(theme.palette.primary.main, 0.045),
          px: 1.5,
          py: 1.25,
          fontFamily: APP_FONT_FAMILY,
          "&:hover .review-comment-delete, &:focus-within .review-comment-delete": {
            opacity: 1,
          },
        }}
      >
        <MessageCircle size={14} style={{ marginTop: 3, flexShrink: 0 }} aria-hidden="true" />
        <Typography
          variant="body2"
          sx={{ minWidth: 0, flex: 1, whiteSpace: "pre-wrap", lineHeight: 1.5 }}
        >
          {displayedText}
        </Typography>
        <Tooltip title="Delete comment">
          <IconButton
            className="review-comment-delete"
            size="small"
            aria-label="Delete comment"
            onClick={onCancel}
            sx={{
              width: 24,
              height: 24,
              mt: -0.5,
              mr: -0.5,
              flexShrink: 0,
              color: "text.secondary",
              opacity: { xs: 1, sm: 0 },
              transition: "opacity 120ms ease",
            }}
          >
            <Trash2 size={13} />
          </IconButton>
        </Tooltip>
      </Box>
    );
  }

  return (
    <Box
      data-review-comment-draft
      contentEditable={false}
      onPointerDown={(event) => event.stopPropagation()}
      sx={{ px: 1.5, py: 1, fontFamily: APP_FONT_FAMILY }}
    >
      <Box
        component="textarea"
        autoFocus
        rows={2}
        aria-label={`Comment on ${rangeLabel}`}
        placeholder="Add a comment…"
        value={displayedText}
        onChange={(event: ChangeEvent<HTMLTextAreaElement>) => updateText(event.target.value)}
        onKeyDown={(event: KeyboardEvent<HTMLTextAreaElement>) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            onCancel();
          }
          if (event.key === "Enter" && (event.metaKey || event.ctrlKey) && displayedText.trim()) {
            event.preventDefault();
            if (onSend) send();
            else submit();
          }
        }}
        sx={{
          display: "block",
          boxSizing: "border-box",
          width: "100%",
          minHeight: 48,
          resize: "vertical",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
          outline: 0,
          bgcolor: "transparent",
          color: "text.primary",
          px: 1.25,
          py: 0.75,
          fontFamily: APP_FONT_FAMILY,
          fontSize: TYPOGRAPHY.chatFontSize,
          lineHeight: TYPOGRAPHY.chatLineHeight,
          "&:focus": { borderColor: "text.secondary" },
          "&::placeholder": { color: "text.secondary", opacity: 0.8 },
        }}
      />
      <Box sx={{ mt: 0.75, display: "flex", alignItems: "center", gap: 0.5 }}>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ mr: "auto", opacity: 0.7, fontSize: TYPOGRAPHY.codeChromeFontSize }}
        >
          ⌘/Ctrl Enter to {onSend ? "send" : "add"}
        </Typography>
        <Button
          size="small"
          color="inherit"
          onClick={onCancel}
          sx={{
            minWidth: 0,
            minHeight: 24,
            px: 1,
            py: 0,
            fontSize: TYPOGRAPHY.codeChromeFontSize,
            textTransform: "none",
          }}
        >
          Cancel
        </Button>
        <Button
          size="small"
          variant={onSend ? "text" : "contained"}
          color={onSend ? "inherit" : "primary"}
          disabled={!displayedText.trim() || isSubmitting || isSending}
          onClick={submit}
          sx={{
            minWidth: 0,
            minHeight: 24,
            px: 1,
            py: 0,
            fontSize: TYPOGRAPHY.codeChromeFontSize,
            textTransform: "none",
          }}
        >
          {isSubmitting ? <CircularProgress size={12} color="inherit" /> : submitLabel}
        </Button>
        {onSend && (
          <Button
            size="small"
            variant="contained"
            disabled={!displayedText.trim() || isSubmitting || isSending}
            onClick={send}
            sx={{
              minWidth: 0,
              minHeight: 24,
              px: 1,
              py: 0,
              fontSize: TYPOGRAPHY.codeChromeFontSize,
              textTransform: "none",
            }}
          >
            {isSending ? <CircularProgress size={12} color="inherit" /> : "Send"}
          </Button>
        )}
      </Box>
    </Box>
  );
}
