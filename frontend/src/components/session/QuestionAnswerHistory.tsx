import Box from "@mui/material/Box";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";
import { CheckCircle2, CircleSlash2 } from "lucide-react";

import type { TypesResolvedQuestion } from "../../api/api";

export default function QuestionAnswerHistory({
  history,
}: {
  history?: TypesResolvedQuestion[];
}) {
  if (!history?.length) return null;

  return (
    <Stack spacing={1} sx={{ width: "100%", mt: 1 }}>
      {history.map((resolved) => (
        <Box
          key={resolved.request_id}
          sx={{
            border: "1px solid",
            borderColor: "divider",
            borderRadius: 1,
            px: 1.5,
            py: 1,
            backgroundColor: "action.hover",
          }}
        >
          <Stack
            direction="row"
            spacing={0.75}
            alignItems="center"
            sx={{ mb: 0.5 }}
          >
            {resolved.outcome === "answered" ? (
              <CheckCircle2 size={16} />
            ) : (
              <CircleSlash2 size={16} />
            )}
            <Typography variant="caption" color="text.secondary">
              {resolved.outcome === "answered"
                ? "Answered"
                : "Question cancelled"}
            </Typography>
          </Stack>
          {resolved.questions?.map((question) => (
            <Box key={question.id} sx={{ "& + &": { mt: 0.75 } }}>
              <Typography variant="body2">{question.question}</Typography>
              {resolved.outcome === "answered" && question.id && (
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ whiteSpace: "pre-line" }}
                >
                  {resolved.answers?.[question.id]}
                </Typography>
              )}
            </Box>
          ))}
        </Box>
      ))}
    </Stack>
  );
}
