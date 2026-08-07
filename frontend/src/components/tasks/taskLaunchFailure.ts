export const SUBSCRIPTION_PROVIDER_LABELS: Record<string, string> = {
  claude: "Claude",
  codex: "ChatGPT",
};

export interface SubscriptionRequirement {
  provider: string;
  label: string;
}

/**
 * Reads the machine-readable reason the backend records when a desktop launch
 * is refused. A missing subscription cannot be retried away — the user has to
 * connect the provider first — so the browser needs to know which one rather
 * than pattern-matching the error prose.
 */
export function subscriptionRequirementFromTask(
  metadata: Record<string, unknown> | undefined,
): SubscriptionRequirement | undefined {
  if (!metadata || metadata.error_code !== "subscription_required") return undefined;
  const provider = typeof metadata.error_provider === "string" ? metadata.error_provider : "";
  if (!provider) return undefined;
  return { provider, label: SUBSCRIPTION_PROVIDER_LABELS[provider] || provider };
}
