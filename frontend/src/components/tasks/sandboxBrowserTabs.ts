export type SandboxBrowserTabCloseAction = 'close' | 'close_others' | 'close_right' | 'close_all'

interface SandboxBrowserTabCloseResult {
  activeTabId: string | null
  openTabIds: string[]
}

export function closeSandboxBrowserTabs(
  openTabIds: readonly string[],
  activeTabId: string | null,
  targetTabId: string,
  action: SandboxBrowserTabCloseAction,
): SandboxBrowserTabCloseResult {
  const targetIndex = openTabIds.indexOf(targetTabId)
  if (targetIndex === -1) return { activeTabId, openTabIds: [...openTabIds] }

  let remainingTabIds: string[]
  switch (action) {
    case 'close':
      remainingTabIds = openTabIds.filter((tabId) => tabId !== targetTabId)
      break
    case 'close_others':
      remainingTabIds = [targetTabId]
      break
    case 'close_right':
      remainingTabIds = openTabIds.slice(0, targetIndex + 1)
      break
    case 'close_all':
      remainingTabIds = []
      break
  }

  if (!activeTabId || remainingTabIds.includes(activeTabId)) {
    return { activeTabId, openTabIds: remainingTabIds }
  }

  return {
    activeTabId: remainingTabIds[Math.min(targetIndex, remainingTabIds.length - 1)] || null,
    openTabIds: remainingTabIds,
  }
}
