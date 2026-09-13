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
    <Stack spacing={1.25} sx={{ width: "100%" }}>
      {history.map((resolved, index) => (
        <Stack
          key={resolved.request_id || resolved.tool_call_id || index}
          spacing={0.75}
        >
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
      ))}
    </Stack>
  );
}
