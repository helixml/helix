export const SESSION_SCROLL_CONTAINER_ATTRIBUTE = 'data-session-scroll-container'

export function preserveDisclosureExpansion(header: HTMLElement) {
  const container = header.closest<HTMLElement>(`[${SESSION_SCROLL_CONTAINER_ATTRIBUTE}]`)
  if (container) container.dataset.preserveDisclosureExpansion = 'true'
}
