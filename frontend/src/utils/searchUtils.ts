export function matchesAllTokens(
  query: string,
  ...fields: (string | undefined | null)[]
): boolean {
  const trimmed = query.trim();
  if (!trimmed) return true;
  const tokens = trimmed.toLowerCase().split(/\s+/);
  const searchableText = fields.filter(Boolean).join(" ").toLowerCase();
  return tokens.every((token) => searchableText.includes(token));
}
