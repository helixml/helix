import React, { FC, useMemo } from "react";
import Alert from "@mui/material/Alert";
import AlertTitle from "@mui/material/AlertTitle";
import Box from "@mui/material/Box";
import Link from "@mui/material/Link";
import PauseCircleOutlineIcon from "@mui/icons-material/PauseCircleOutline";
import useRouter from "../../hooks/useRouter";
import { TypesSession } from "../../api/api";

interface PausedBannerProps {
  session: TypesSession;
}

/**
 * Banner rendered at the top of a paused session's view, explaining why
 * the session is frozen and (when applicable) linking to the child it
 * was forked into.
 *
 * The chat input on a paused session should also be disabled — that
 * happens in the parent that owns the input, by reading
 * `session.config.paused`.
 *
 * See design/tasks/002081_kickoff-mid-session/.
 */
const PausedBanner: FC<PausedBannerProps> = ({ session }) => {
  const router = useRouter();
  const metadata = session.config;
  const paused = !!metadata?.paused;
  const reason = metadata?.paused_reason || "";

  // PausedReason format from the backend: "forked_to:<child_session_id>".
  // We parse that here to render an actionable link rather than dumping
  // the raw string. v1 has no other producers; if/when manual pause
  // arrives, plumb the additional reasons through here.
  const forkedChildID = useMemo(() => {
    if (!reason.startsWith("forked_to:")) return "";
    return reason.slice("forked_to:".length);
  }, [reason]);

  if (!paused) return null;

  const orgId = (router.params?.org_id as string | undefined) || undefined;
  const navigateToChild = () => {
    if (!forkedChildID) return;
    if (orgId) {
      router.navigate("org_session", {
        org_id: orgId,
        session_id: forkedChildID,
      });
    } else {
      router.navigate("session", { session_id: forkedChildID });
    }
  };

  return (
    <Box sx={{ px: 2, pt: 2 }}>
      <Alert
        severity="info"
        icon={<PauseCircleOutlineIcon fontSize="small" />}
        sx={{ alignItems: "center" }}
      >
        <AlertTitle sx={{ mb: 0.5 }}>Paused</AlertTitle>
        {forkedChildID ? (
          <>
            This session was forked into a new conversation. Continue in the{" "}
            <Link
              component="button"
              onClick={navigateToChild}
              underline="hover"
              sx={{ fontWeight: 600 }}
            >
              child session
            </Link>
            . This view remains as a frozen checkpoint.
          </>
        ) : (
          <>
            This session is paused and not accepting new messages.
            {reason ? ` (${reason})` : ""}
          </>
        )}
      </Alert>
    </Box>
  );
};

export default PausedBanner;
