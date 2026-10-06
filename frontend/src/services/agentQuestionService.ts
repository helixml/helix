import { useMutation } from "@tanstack/react-query";

import useApi from "../hooks/useApi";

export function useRespondToAgentQuestion() {
  const api = useApi();
  return useMutation({
    mutationFn: ({
      interactionId,
      requestId,
      answers,
    }: {
      interactionId: string;
      requestId: string;
      answers: Record<string, string>;
    }) =>
      api
        .getApiClient()
        .v1InteractionsQuestionsRespondCreate(interactionId, requestId, {
          answers,
        }),
  });
}

export function useCancelAgentQuestion() {
  const api = useApi();
  return useMutation({
    mutationFn: ({
      interactionId,
      requestId,
    }: {
      interactionId: string;
      requestId: string;
    }) =>
      api
        .getApiClient()
        .v1InteractionsQuestionsCancelCreate(interactionId, requestId),
  });
}
