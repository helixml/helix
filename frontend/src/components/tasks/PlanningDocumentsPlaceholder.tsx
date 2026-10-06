import { Box, Typography } from "@mui/material";
import { FileText, TriangleAlert } from "lucide-react";
import { TypesPushError } from "../../api/api";

interface PlanningDocumentsPlaceholderProps {
  pushError?: TypesPushError;
}

const PlanningDocumentsPlaceholder = ({ pushError }: PlanningDocumentsPlaceholderProps) => {
  const failed = Boolean(pushError);

  return (
    <Box
      sx={{
        flex: 1,
        minHeight: 0,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        p: 4,
      }}
    >
      <Box
        sx={{
          width: "100%",
          maxWidth: 420,
          px: 3,
          py: 3.5,
          textAlign: "center",
          border: "1px solid",
          borderColor: failed ? "error.main" : "divider",
          borderRadius: 1,
          bgcolor: "action.hover",
        }}
      >
        <Box
          sx={{
            display: "flex",
            justifyContent: "center",
            mb: 1.5,
            color: failed ? "error.main" : "text.secondary",
          }}
        >
          {failed ? <TriangleAlert size={22} /> : <FileText size={22} />}
        </Box>
        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.5 }}>
          {failed ? "Plan documents were not published" : "Planning in progress"}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {failed
            ? pushError?.cause || "The planning agent could not publish the plan documents."
            : "requirements.md, design.md, and tasks.md will appear here when the planning agent publishes them."}
        </Typography>
        {pushError?.next_step && (
          <Typography variant="body2" sx={{ mt: 1.5, color: "text.primary" }}>
            {pushError.next_step}
          </Typography>
        )}
      </Box>
    </Box>
  );
};

export default PlanningDocumentsPlaceholder;
