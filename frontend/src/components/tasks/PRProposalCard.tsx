import { useEffect, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import IconButton from "@mui/material/IconButton";
import Stack from "@mui/material/Stack";
import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { alpha, useTheme } from "@mui/material/styles";
import {
  ArrowRight,
  ChevronDown,
  ChevronRight,
  GitPullRequestArrow,
  X,
} from "lucide-react";

import {
  TypesSpecTaskPRProposal,
  TypesSpecTaskPRProposalStatus,
} from "../../api/api";
import useSnackbar from "../../hooks/useSnackbar";
import { useDecidePRProposal } from "../../services/specTaskPRProposalService";
import { APP_MONO_FONT_FAMILY } from "../../styles/typography";
import { getChatColors } from "../session/chatStyles";

const { PRProposalStatusPending, PRProposalStatusApproved, PRProposalStatusFailed } =
  TypesSpecTaskPRProposalStatus;

/**
 * The agent's request to push a branch and open a pull request from it,
 * attached above the chat composer like an agent question. Nothing reaches the
 * git provider until the user approves here.
 */
export default function PRProposalCard({
  specTaskId,
  proposal,
}: {
  specTaskId: string;
  proposal: TypesSpecTaskPRProposal;
}) {
  const theme = useTheme();
  const colors = getChatColors(theme);
  const snackbar = useSnackbar();
  const decide = useDecidePRProposal(specTaskId);
  const status = proposal.status;
  const isPending = status === PRProposalStatusPending;

  const [expanded, setExpanded] = useState(isPending);
  const [headBranch, setHeadBranch] = useState(proposal.head_branch ?? "");
  const [baseBranch, setBaseBranch] = useState(proposal.base_branch ?? "");
  const [title, setTitle] = useState(proposal.title ?? "");
  const [body, setBody] = useState(proposal.body ?? "");
  const [comment, setComment] = useState("");
  const [autoApproveFuture, setAutoApproveFuture] = useState(false);
  const [error, setError] = useState("");

  // The agent may refine a pending proposal; reset local edits when it does.
  useEffect(() => {
    setHeadBranch(proposal.head_branch ?? "");
    setBaseBranch(proposal.base_branch ?? "");
    setTitle(proposal.title ?? "");
    setBody(proposal.body ?? "");
    setError("");
  }, [proposal.id, proposal.updated_at]);

  const submit = (decision: "approve" | "reject") => {
    setError("");
    // Only send edits that differ, so the agent is told about real changes only.
    const changed = (value: string, original?: string) =>
      isPending && value.trim() !== (original ?? "") ? value.trim() : undefined;
    decide.mutate(
      {
        proposalId: proposal.id!,
        request: {
          decision,
          comment: comment.trim() || undefined,
          auto_approve_future: decision === "approve" && autoApproveFuture ? true : undefined,
          head_branch: decision === "approve" ? changed(headBranch, proposal.head_branch) : undefined,
          base_branch: decision === "approve" ? changed(baseBranch, proposal.base_branch) : undefined,
          title: decision === "approve" ? changed(title, proposal.title) : undefined,
          body: decision === "approve" && isPending && body !== (proposal.body ?? "") ? body : undefined,
        },
      },
      {
        onSuccess: (result) => {
          setComment("");
          if (decision === "reject") {
            snackbar.info("Proposal rejected — the agent has been told why");
          } else if (result.status === TypesSpecTaskPRProposalStatus.PRProposalStatusOpened) {
            snackbar.success(`Pull request #${result.pr_number} opened`);
          } else if (result.status === PRProposalStatusFailed) {
            snackbar.error("Approved, but the pull request could not be opened");
          } else {
            snackbar.success(`Approved — the agent can now push to ${result.head_branch}`);
          }
        },
        onError: (err: any) => {
          const data = err?.response?.data;
          if (data?.error === "oauth_required") {
            setError(
              `${data.message} Connect it under Account → Connected Services, then approve again.`,
            );
            return;
          }
          setError(typeof data === "string" ? data : data?.message || err?.message || "Request failed");
        },
      },
    );
  };

  const busy = decide.isPending;
  const headerLabel =
    status === PRProposalStatusApproved
      ? proposal.auto_approved
        ? "Auto-approved — waiting for the agent to push"
        : "Approved — waiting for the agent to push"
      : status === PRProposalStatusFailed
        ? "Pull request could not be opened"
        : "Agent wants to open a pull request";
  const accent =
    status === PRProposalStatusFailed
      ? theme.palette.error.main
      : status === PRProposalStatusApproved
        ? theme.palette.text.secondary
        : theme.palette.warning.main;
  const branchText = { fontFamily: APP_MONO_FONT_FAMILY, fontSize: "0.8rem" };
  const fieldSx = {
    "& .MuiOutlinedInput-root": {
      borderRadius: "8px",
      backgroundColor: alpha(theme.palette.background.default, 0.45),
    },
    "& .MuiInputBase-input": { fontSize: "0.875rem" },
  };

  return (
    <Box
      data-composer-pr-proposal={proposal.id}
      sx={{
        position: "relative",
        zIndex: 0,
        overflow: "hidden",
        flexShrink: 0,
        border: "1px solid",
        borderColor: colors.border,
        borderRadius: "16px",
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
            "&:hover": { backgroundColor: alpha(theme.palette.text.primary, 0.025) },
            "&:focus-visible": {
              outline: `2px solid ${theme.palette.primary.main}`,
              outlineOffset: -2,
            },
          }}
        >
          <Box component="span" sx={{ display: "flex", justifyContent: "center", color: accent }}>
            <GitPullRequestArrow aria-hidden size={14} />
          </Box>
          <Box component="span" sx={{ minWidth: 0, display: "flex", alignItems: "center", gap: 0.75 }}>
            <Typography component="span" variant="caption" sx={{ flexShrink: 0, color: colors.subtle, fontWeight: 500 }}>
              {headerLabel}
            </Typography>
            {!expanded && (
              <Typography
                component="span"
                variant="caption"
                sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", color: colors.subtle, opacity: 0.72 }}
              >
                {proposal.head_branch} → {proposal.base_branch} · {proposal.title}
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
        {!isPending && (
          <Tooltip title="Withdraw approval — the agent loses push rights to this branch">
            <span>
              <IconButton
                aria-label="Withdraw approval"
                disabled={busy}
                onClick={() => submit("reject")}
                sx={{ width: 30, height: 30, color: colors.subtle }}
              >
                <X size={18} />
              </IconButton>
            </span>
          </Tooltip>
        )}
      </Box>

      <Collapse in={expanded}>
        <Box sx={{ pl: 4, pr: 1.5, pb: 1.25 }}>
          {proposal.reason && (
            <Typography variant="body2" sx={{ color: alpha(colors.foreground, 0.85), whiteSpace: "pre-wrap" }}>
              {proposal.reason}
            </Typography>
          )}

          {isPending ? (
            <Stack spacing={1} sx={{ mt: 1 }}>
              <Stack direction="row" spacing={0.75} alignItems="center">
                <TextField
                  size="small"
                  label="Push to branch"
                  value={headBranch}
                  disabled={busy}
                  onChange={(e) => setHeadBranch(e.target.value)}
                  inputProps={{ style: branchText, "aria-label": "Head branch" }}
                  sx={{ ...fieldSx, flex: 3 }}
                />
                <ArrowRight aria-hidden size={14} color={colors.subtle} />
                <TextField
                  size="small"
                  label="Into"
                  value={baseBranch}
                  disabled={busy}
                  onChange={(e) => setBaseBranch(e.target.value)}
                  inputProps={{ style: branchText, "aria-label": "Base branch" }}
                  sx={{ ...fieldSx, flex: 2 }}
                />
              </Stack>
              <TextField
                size="small"
                label="Title"
                value={title}
                disabled={busy}
                onChange={(e) => setTitle(e.target.value)}
                sx={fieldSx}
              />
              <TextField
                size="small"
                label="Description"
                value={body}
                disabled={busy}
                multiline
                minRows={2}
                maxRows={8}
                onChange={(e) => setBody(e.target.value)}
                sx={fieldSx}
              />
              <TextField
                size="small"
                label="Note to the agent (optional)"
                value={comment}
                disabled={busy}
                onChange={(e) => setComment(e.target.value)}
                sx={fieldSx}
              />
              <Typography variant="caption" sx={{ color: colors.subtle }}>
                Approving lets the agent push to{" "}
                <Box component="span" sx={branchText}>{headBranch || "…"}</Box> in{" "}
                {proposal.repository_name} and opens a pull request into{" "}
                <Box component="span" sx={branchText}>{baseBranch || "…"}</Box>.
              </Typography>
            </Stack>
          ) : (
            <Typography variant="caption" component="div" sx={{ mt: 0.5, color: colors.subtle }}>
              <Box component="span" sx={branchText}>{proposal.head_branch}</Box> →{" "}
              <Box component="span" sx={branchText}>{proposal.base_branch}</Box> in {proposal.repository_name} · {proposal.title}
              {status === PRProposalStatusApproved &&
                ". The pull request opens as soon as the branch has commits that are not on the base."}
            </Typography>
          )}

          {status === PRProposalStatusFailed && proposal.error && (
            <Typography
              variant="caption"
              component="pre"
              sx={{ mt: 0.75, mb: 0, whiteSpace: "pre-wrap", wordBreak: "break-word", color: theme.palette.error.main, fontFamily: APP_MONO_FONT_FAMILY }}
            >
              {proposal.error}
            </Typography>
          )}
          {error && (
            <Typography variant="caption" component="div" sx={{ mt: 0.75, color: theme.palette.error.main }}>
              {error}
            </Typography>
          )}

          {(isPending || status === PRProposalStatusFailed) && (
            <Stack direction="row" alignItems="center" spacing={0.75} sx={{ mt: 1 }}>
              <Tooltip describeChild title="Later proposals from this task's agent are approved without asking, using your credentials. Turn it off in the task's details.">
                <FormControlLabel
                  disabled={busy}
                  control={
                    <Checkbox
                      size="small"
                      checked={autoApproveFuture}
                      onChange={(e) => setAutoApproveFuture(e.target.checked)}
                      sx={{ p: 0.5 }}
                    />
                  }
                  label={
                    <Typography variant="caption" sx={{ color: colors.subtle }}>
                      Auto-approve future pull requests for this task
                    </Typography>
                  }
                  sx={{ mr: "auto", ml: 0 }}
                />
              </Tooltip>
              <Button
                size="small"
                disabled={busy}
                onClick={() => submit("reject")}
                sx={{ minHeight: 30, borderRadius: 999, px: 1.5, textTransform: "none", color: colors.subtle, whiteSpace: "nowrap", flexShrink: 0 }}
              >
                {isPending ? "Reject" : "Cancel"}
              </Button>
              <Button
                size="small"
                variant="contained"
                color="success"
                disabled={busy || (isPending && (!headBranch.trim() || !baseBranch.trim()))}
                onClick={() => submit("approve")}
                sx={{ minHeight: 30, borderRadius: 999, px: 1.5, textTransform: "none", whiteSpace: "nowrap", flexShrink: 0 }}
              >
                {busy ? "Working…" : isPending ? "Approve & open PR" : "Retry"}
              </Button>
            </Stack>
          )}
        </Box>
      </Collapse>
    </Box>
  );
}
