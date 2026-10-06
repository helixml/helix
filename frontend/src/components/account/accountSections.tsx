import { ReactNode } from 'react'

import { GitCommitHorizontal, KeyRound, MessageSquare, Settings } from 'lucide-react'

export interface AccountSection {
  id: string
  label: string
  icon: ReactNode
}

// The desktop sidebar and the mobile section list are two presentations of the
// same navigation, so the sections live here rather than in either component.
export const ACCOUNT_SECTIONS: AccountSection[] = [
  { id: 'general', label: 'General Settings', icon: <Settings /> },
  { id: 'git_config', label: 'Git Config', icon: <GitCommitHorizontal /> },
  { id: 'chat', label: 'Chat', icon: <MessageSquare /> },
  { id: 'api_keys', label: 'API Keys', icon: <KeyRound /> },
]

export const DEFAULT_ACCOUNT_TAB = 'general'

export const isAccountTab = (tab?: string): boolean =>
  !!tab && ACCOUNT_SECTIONS.some((section) => section.id === tab)

export const accountSectionLabel = (tab: string): string =>
  ACCOUNT_SECTIONS.find((section) => section.id === tab)?.label || 'Account'
