import { Api, TypesElicitationRespondResponse } from "../api/api";

// `useApi().getApiClient()` hands back the generated client's `api` namespace
// object, not the `Api` instance itself, so that is what these take.
type ApiNamespace = Api<unknown>["api"];

/**
 * Answers a question the agent asked.
 *
 * Uses the generated client (never a hand-rolled fetch). The client is passed in rather
 * than imported: `getApiClient()` comes from the `useApi()` hook, so it can only be
 * obtained inside a component, while this module must stay callable from a plain mutation
 * function.
 *
 * The endpoint returns `submitting` — the authoritative terminal status arrives from the
 * agent over the session WebSocket once it has actually applied the answer.
 *
 * A 409 here is normal and meaningful: the question was already answered, skipped, or
 * cancelled (for example by the turn ending, or by another browser tab). The card surfaces
 * the message rather than retrying.
 */
export async function respondToElicitation(
  client: ApiNamespace,
  sessionId: string,
  elicitationId: string,
  action: "accept" | "decline",
  content: Record<string, unknown>,
): Promise<TypesElicitationRespondResponse> {
  const response = await client.v1SessionsElicitationsRespondCreate(
    sessionId,
    elicitationId,
    { action, content },
  );
  return response.data;
}

/** Questions on this session that can still be answered. */
export async function listSessionElicitations(
  client: ApiNamespace,
  sessionId: string,
) {
  const response = await client.v1SessionsElicitationsDetail(sessionId);
  return response.data;
}
