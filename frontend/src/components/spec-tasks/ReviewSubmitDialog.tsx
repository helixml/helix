import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  TextField,
} from "@mui/material";

interface ReviewSubmitDialogProps {
  open: boolean;
  onClose: () => void;
  overallComment: string;
  onCommentChange: (value: string) => void;
  onSubmit: () => void;
  isSubmitting: boolean;
}

export default function ReviewSubmitDialog({
  open,
  onClose,
  overallComment,
  onCommentChange,
  onSubmit,
  isSubmitting,
}: ReviewSubmitDialogProps) {
  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      sx={{ zIndex: 200000 }}
    >
      <DialogTitle>Approve Design</DialogTitle>
      <DialogContent>
        <TextField
          fullWidth
          multiline
          rows={4}
          label="Overall Comment (optional)"
          value={overallComment}
          onChange={(event) => onCommentChange(event.target.value)}
          sx={{ mt: 2 }}
        />
      </DialogContent>
      <Box p={2} display="flex" gap={2} justifyContent="flex-end">
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          color="success"
          onClick={onSubmit}
          disabled={isSubmitting}
          startIcon={
            isSubmitting ? (
              <CircularProgress size={16} color="inherit" />
            ) : undefined
          }
        >
          {isSubmitting ? "Approving…" : "Approve"}
        </Button>
      </Box>
    </Dialog>
  );
}
