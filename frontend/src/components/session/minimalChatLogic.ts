/**
 * The two judgement calls behind the customer-facing chat embed.
 *
 * Both are one-liners at the call site and both are wrong in a way nobody sees
 * until it is in front of a customer — one hides a message that should be
 * visible, the other shows a system prompt that should not be. Pulled out here
 * so they can be tested without mounting a session.
 */

/**
 * Which rendered interaction is the session's opening briefing, or -1 for none.
 *
 * The first turn of an agent session is not something the customer said: it is
 * the prompt that told the agent who it is and what it can do, and it renders
 * as a user message like any other. On an embed it has to go.
 *
 * It is only safe to assume index 0 is that turn once pagination has reached
 * the oldest page. A thread that starts mid-conversation has a real customer
 * message at index 0, and blanking that would be worse than the problem.
 */
export function seedPromptIndex(minimal: boolean, hasOlderInteractions: boolean): number {
  if (!minimal) return -1
  if (hasOlderInteractions) return -1
  return 0
}

/**
 * Should the welcome screen stand in for the thread?
 *
 * One interaction means only the opening briefing exists — the agent was told
 * who it is talking to and replied with a greeting. Nobody has asked it
 * anything, so there is no conversation to show yet.
 *
 * `hasSent` covers the gap between the customer clicking send and the server
 * count catching up on the next poll. Without it the welcome screen flashes
 * back for a beat after they have already spoken.
 */
export function shouldShowWelcome(
  minimal: boolean,
  hasSent: boolean,
  totalInteractions: number,
): boolean {
  if (!minimal) return false
  if (hasSent) return false
  return totalInteractions <= 1
}
