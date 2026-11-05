import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import Typography from "@mui/material/Typography";

import useSnackbar from "../../hooks/useSnackbar";
import { useDecideCompletion } from "../../services/specTaskCompletionService";

/**
 * Confirms marking a task done by hand: finishing it, or abandoning it when no
 * pull request turned out to be needed. Done stops the task's agent desktop.
 */
export default function MarkDoneDialog({
  open,
  onClose,
  taskId,
  openPullRequests,
}: {
  open: boolean;
  onClose: () => void;
  taskId: string;
  openPullRequests: number;
}) {
  const snackbar = useSnackbar();
  const decide = useDecideCompletion(taskId);

  const confirm = () =>
    decide.mutate(
      { decision: "approve" },
      {
        onSuccess: () => {
          snackbar.success("Task marked done");
          onClose();
        },
        onError: (err: any) => {
          const data = err?.response?.data;
          snackbar.error(
            (typeof data === "string" ? data : data?.message) ||
              "Failed to mark the task done",
          );
        },
      },
    );

  return (
    <Dialog
      open={open}
      onClose={decide.isPending ? undefined : onClose}
      onClick={(e) => e.stopPropagation()}
      maxWidth="xs"
      fullWidth
    >
      <DialogTitle>Mark task done?</DialogTitle>
      <DialogContent>
        <Typography variant="body2">
          The task moves to Done and its agent desktop stops. Use this when the
          work is finished, or to close a task that turned out not to need any
          changes.
        </Typography>
        {openPullRequests > 0 && (
          <Typography variant="body2" sx={{ mt: 1.5, color: "warning.main" }}>
            {openPullRequests === 1
              ? "1 pull request is still open. It stays open on the provider."
              : `${openPullRequests} pull requests are still open. They stay open on the provider.`}
          </Typography>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={decide.isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          color="success"
          onClick={confirm}
          disabled={decide.isPending}
          startIcon={decide.isPending ? <CircularProgress size={14} color="inherit" /> : undefined}
        >
          Mark done
        </Button>
      </DialogActions>
    </Dialog>
  );
}
