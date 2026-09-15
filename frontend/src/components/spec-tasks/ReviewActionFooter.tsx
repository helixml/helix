import { Alert, Box, Button, Tooltip } from "@mui/material";
import { Code2 } from "lucide-react";

interface ReviewActionFooterProps {
  reviewStatus:
    | "pending"
    | "in_review"
    | "changes_requested"
    | "approved"
    | "superseded";
  unresolvedCount: number;
  startingImplementation: boolean;
  implementationStarted: boolean;
  isBlockedByDependencies?: boolean;
  blockedReason?: string;
  onApprove: () => void;
  onStartImplementation: () => void;
}

export default function ReviewActionFooter({
  reviewStatus,
  unresolvedCount,
  startingImplementation,
  implementationStarted,
  isBlockedByDependencies = false,
  blockedReason = "",
  onApprove,
  onStartImplementation,
}: ReviewActionFooterProps) {
  return (
    <Box
      sx={{
        minHeight: 56,
        px: 2,
        py: 1,
        display: "flex",
        flexShrink: 0,
        alignItems: "center",
        justifyContent: "flex-end",
        gap: 1,
        borderTop: 1,
        borderColor: "divider",
        bgcolor: "background.paper",
      }}
    >
      {reviewStatus === "approved" ? (
        <Box display="flex" alignItems="center" gap={1} flex={1}>
          <Alert severity="success" sx={{ flex: 1, py: 0 }}>
            {implementationStarted
              ? "Design approved! Implementation in progress."
              : "Design approved! Ready to start implementation."}
          </Alert>
          {!implementationStarted && (
            <Tooltip
              title={isBlockedByDependencies ? blockedReason : ""}
              placement="top"
            >
              <span>
                <Button
                  variant="contained"
                  color="primary"
                  startIcon={<Code2 size={18} />}
                  onClick={onStartImplementation}
                  disabled={startingImplementation}
                  sx={{ minHeight: 40 }}
                >
                  {startingImplementation
                    ? "Starting Implementation..."
                    : isBlockedByDependencies
                      ? "Queue Implementation"
                      : "Start Implementation"}
                </Button>
              </span>
            </Tooltip>
          )}
        </Box>
      ) : reviewStatus !== "superseded" ? (
        <>
          {unresolvedCount > 0 && (
            <Alert severity="warning" sx={{ flex: 1, py: 0 }}>
              {unresolvedCount} unresolved comment
              {unresolvedCount !== 1 ? "s" : ""}
            </Alert>
          )}
          <Tooltip
            title={
              unresolvedCount > 0
                ? `Resolve ${unresolvedCount} comment${unresolvedCount !== 1 ? "s" : ""} before approving`
                : ""
            }
            placement="top"
          >
            <span>
              <Button
                variant="contained"
                color="success"
                onClick={onApprove}
                disabled={unresolvedCount > 0}
                sx={{ minHeight: 40 }}
              >
                Approve
              </Button>
            </span>
          </Tooltip>
        </>
      ) : (
        <Alert severity="info" sx={{ flex: 1, py: 0 }}>
          This review has been superseded by a newer version
        </Alert>
      )}
    </Box>
  );
}
