import React, { FC } from "react";
import Chip from "@mui/material/Chip";
import Tooltip from "@mui/material/Tooltip";
import CallSplitIcon from "@mui/icons-material/CallSplit";
import useRouter from "../../hooks/useRouter";
import { TypesSession } from "../../api/api";

interface ForkBadgeProps {
  session: TypesSession;
}

/**
 * Small "Forked from <parent>" pill rendered in the session header for
 * sessions that were created via fork-and-pause. Clicking it navigates
 * to the parent (which is, by definition, paused) so the user can see
 * the prior conversation.
 *
 * No-op when the session has no parent (i.e. it wasn't forked).
 *
 * See design/tasks/002081_kickoff-mid-session/.
 */
const ForkBadge: FC<ForkBadgeProps> = ({ session }) => {
  const router = useRouter();
  const parentID = session.config?.parent_session_id;
  if (!parentID) return null;

  const orgId = (router.params?.org_id as string | undefined) || undefined;
  const goToParent = () => {
    if (orgId) {
      router.navigate("org_session", { org_id: orgId, session_id: parentID });
    } else {
      router.navigate("session", { session_id: parentID });
    }
  };

  // Display a truncated id to keep the badge compact; the full id is in
  // the tooltip for users who want to copy it out.
  const shortID = parentID.length > 12 ? parentID.slice(0, 12) + "…" : parentID;

  return (
    <Tooltip title={`Forked from ${parentID} — click to view the parent (paused)`}>
      <Chip
        icon={<CallSplitIcon sx={{ fontSize: 14 }} />}
        label={`Forked from ${shortID}`}
        size="small"
        variant="outlined"
        onClick={goToParent}
        sx={{
          height: 22,
          fontSize: "0.7rem",
          cursor: "pointer",
          "& .MuiChip-icon": { ml: 0.5 },
        }}
      />
    </Tooltip>
  );
};

export default ForkBadge;
