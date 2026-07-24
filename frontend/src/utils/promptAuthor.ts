import { TypesPromptAuthor } from '../api/api'

/**
 * promptAuthorLabel resolves a short, human-readable author label for a prompt in
 * the org-global queue. The queue shows prompts from every org member and from
 * service accounts, so each entry carries a server-resolved `author`. Service
 * accounts (or unresolvable owners) render as "System"; humans render as their
 * name, falling back to email. Returns '' when there is nothing useful to show.
 *
 * Accepts anything carrying an optional `author` so it works for both the
 * generated API entry and the local queue-entry types.
 */
export const promptAuthorLabel = (entry: { author?: TypesPromptAuthor }): string => {
  const author = entry.author
  if (!author) return ''
  if (author.is_system) return 'System'
  return author.name || author.email || ''
}
