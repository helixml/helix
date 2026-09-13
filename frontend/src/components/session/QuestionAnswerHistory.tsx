import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";

import type { TypesResolvedQuestion } from "../../api/api";

export default function QuestionAnswerHistory({
  history,
}: {
  history?: TypesResolvedQuestion[];
}) {
  if (!history?.length) return null;

  return (
    <Stack spacing={1.25} sx={{ width: "100%", mt: 1, pl: 3.5 }}>
      {history.map((resolved) =>
        resolved.outcome === "answered" ? (
          <Stack key={resolved.request_id} spacing={0.75}>
            {resolved.questions?.map((question) => (
              <Stack key={question.id} spacing={0.25}>
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ whiteSpace: "pre-line" }}
                >
                  {question.question}
                </Typography>
                {question.id && resolved.answers?.[question.id] && (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ pl: 1.5, whiteSpace: "pre-line" }}
                  >
                    {resolved.answers[question.id]}
                  </Typography>
                )}
              </Stack>
            ))}
          </Stack>
        ) : (
          <Typography
            key={resolved.request_id}
            variant="body2"
            color="text.secondary"
          >
            Question cancelled
          </Typography>
        ),
      )}
    </Stack>
  );
}
