import React, { FC, FormEvent, useEffect, useRef, useState } from 'react'
import {
  Alert,
  Box,
  CircularProgress,
  Menu,
  MenuItem,
  IconButton,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe2,
  Plus,
  RotateCw,
  X,
} from 'lucide-react'

import type { TypesVHostRoute } from '../../api/api'
import { useGetConfig } from '../../services/userService'
import {
  useCreateSessionPreviewToken,
  useSessionPreviewTokens,
} from '../../services/sessionPreviewService'
import {
  parseSandboxBrowserTarget,
  sandboxPreviewUrl,
  sandboxPreviewURLWithScheme,
} from './sandboxBrowserUrl'
import {
  closeSandboxBrowserTabs,
  type SandboxBrowserTabCloseAction,
} from './sandboxBrowserTabs'

interface SandboxBrowserProps {
  sessionId: string
}

interface BrowserHistoryEntry {
  displayUrl: string
  previewUrl: string
}

interface BrowserTab {
  address: string
  error: string
  history: BrowserHistoryEntry[]
  historyIndex: number
  id: string
  reloadNonce: number
}

interface TabContextMenu {
  left: number
  tabId: string
  top: number
}

const defaultAddress = 'http://localhost:8080'

function createBrowserTab(id: string, address = defaultAddress): BrowserTab {
  return {
    address,
    error: '',
    history: [],
    historyIndex: -1,
    id,
    reloadNonce: 0,
  }
}

function browserTabLabel(tab: BrowserTab): string {
  const current = tab.historyIndex >= 0 ? tab.history[tab.historyIndex] : undefined
  try {
    return new URL(current?.displayUrl || tab.address).host || 'New tab'
  } catch {
    return 'New tab'
  }
}

const browserButtonSx = {
  width: 30,
  height: 30,
  minWidth: 30,
  minHeight: 30,
  p: 0.75,
  color: 'text.secondary',
  '& svg': { width: 18, height: 18 },
  '&:hover': { color: 'text.primary' },
} as const

function errorMessage(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const response = (error as { response?: { data?: unknown } }).response
    if (typeof response?.data === 'string') return response.data
    if (typeof response?.data === 'object' && response.data !== null) {
      const data = response.data as { message?: unknown; error?: unknown }
      if (typeof data.message === 'string') return data.message
      if (typeof data.error === 'string') return data.error
    }
  }
  return error instanceof Error ? error.message : 'Preview could not be opened'
}

const SandboxBrowser: FC<SandboxBrowserProps> = ({ sessionId }) => {
  const storageKey = `helix.sandboxBrowser.url.${sessionId}`
  const nextTabId = useRef(1)
  const [tabs, setTabs] = useState<BrowserTab[]>(() => [
    createBrowserTab('browser-0', window.localStorage.getItem(storageKey) ?? defaultAddress),
  ])
  const [activeTabId, setActiveTabId] = useState<string | null>('browser-0')
  const [tabContextMenu, setTabContextMenu] = useState<TabContextMenu | null>(null)
  const configQuery = useGetConfig()
  const previewConfigured = !!configQuery.data?.dev_subdomain
  const previewURLHTTPS = configQuery.data?.preview_url_https ?? true
  const tokensQuery = useSessionPreviewTokens(sessionId, previewConfigured)
  const createToken = useCreateSessionPreviewToken(sessionId)

  useEffect(() => {
    const nextStorageKey = `helix.sandboxBrowser.url.${sessionId}`
    nextTabId.current = 1
    setTabs([
      createBrowserTab('browser-0', window.localStorage.getItem(nextStorageKey) ?? defaultAddress),
    ])
    setActiveTabId('browser-0')
    setTabContextMenu(null)
  }, [sessionId])

  const activeTab = tabs.find((tab) => tab.id === activeTabId)
  const current = activeTab && activeTab.historyIndex >= 0
    ? activeTab.history[activeTab.historyIndex]
    : undefined
  const externalPreviewUrl = current
    ? sandboxPreviewURLWithScheme(current.previewUrl, previewURLHTTPS)
    : undefined
  const isLoading = createToken.isPending

  if (configQuery.isLoading) {
    return (
      <Box sx={{ flex: 1, minHeight: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <CircularProgress size={24} />
      </Box>
    )
  }

  if (!previewConfigured) {
    return (
      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          p: 3,
          overflow: 'hidden',
          backgroundColor: 'background.default',
        }}
      >
        <Box sx={{ width: '100%', maxWidth: 560 }}>
          <Alert severity="warning" sx={{ alignItems: 'flex-start' }}>
            <Typography variant="subtitle2">Sandbox browser previews are not configured</Typography>
            <Typography variant="body2" sx={{ mt: 0.5 }}>
              Set a wildcard preview domain on the API, then restart it. Choose HTTPS only when
              that domain has TLS termination.
            </Typography>
            <Box
              component="pre"
              sx={{
                m: 0,
                mt: 1.5,
                p: 1.5,
                overflowX: 'auto',
                borderRadius: 1,
                backgroundColor: 'rgba(0, 0, 0, 0.28)',
                color: 'text.primary',
                fontFamily: 'monospace',
                fontSize: '0.78rem',
                lineHeight: 1.6,
              }}
            >
              {`DEV_SUBDOMAIN=preview.example.com\nPREVIEW_URL_HTTPS=${previewURLHTTPS}`}
            </Box>
          </Alert>
        </Box>
      </Box>
    )
  }

  const findOrCreateRoute = async (port: number): Promise<TypesVHostRoute> => {
    const cached = tokensQuery.data?.find((route) => route.port === port)
    if (cached) return cached

    const refreshed = await tokensQuery.refetch()
    const existing = refreshed.data?.find((route) => route.port === port)
    if (existing) return existing

    const created = await createToken.mutateAsync(port)
    if (!created?.hostname) throw new Error('Preview route was created without a hostname')
    return created
  }

  const updateTab = (tabId: string, update: (tab: BrowserTab) => BrowserTab) => {
    setTabs((currentTabs) => currentTabs.map((tab) => tab.id === tabId ? update(tab) : tab))
  }

  const navigate = async (tabId: string, input: string) => {
    updateTab(tabId, (tab) => ({ ...tab, error: '' }))
    try {
      const target = parseSandboxBrowserTarget(input)
      const route = await findOrCreateRoute(target.port)
      if (!route.url) throw new Error('Preview route has no public URL')

      const entry = {
        displayUrl: target.displayUrl,
        previewUrl: sandboxPreviewUrl(route.url, target.path, previewURLHTTPS),
      }
      updateTab(tabId, (tab) => {
        const nextHistory = [...tab.history.slice(0, tab.historyIndex + 1), entry]
        return {
          ...tab,
          address: target.displayUrl,
          error: '',
          history: nextHistory,
          historyIndex: nextHistory.length - 1,
        }
      })
      window.localStorage.setItem(storageKey, target.displayUrl)
    } catch (navigateError) {
      updateTab(tabId, (tab) => ({ ...tab, error: errorMessage(navigateError) }))
    }
  }

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (activeTab) void navigate(activeTab.id, activeTab.address)
  }

  const moveHistory = (tabId: string, nextIndex: number) => {
    const tab = tabs.find((candidate) => candidate.id === tabId)
    const next = tab?.history[nextIndex]
    if (!next) return
    updateTab(tabId, (currentTab) => ({
      ...currentTab,
      address: next.displayUrl,
      error: '',
      historyIndex: nextIndex,
    }))
  }

  const addTab = () => {
    const tabId = `browser-${nextTabId.current}`
    nextTabId.current += 1
    setTabs((currentTabs) => [...currentTabs, createBrowserTab(tabId)])
    setActiveTabId(tabId)
    setTabContextMenu(null)
  }

  const closeTabs = (targetTabId: string, action: SandboxBrowserTabCloseAction) => {
    const result = closeSandboxBrowserTabs(
      tabs.map((tab) => tab.id),
      activeTabId,
      targetTabId,
      action,
    )
    const remainingTabIds = new Set(result.openTabIds)
    setTabs((currentTabs) => currentTabs.filter((tab) => remainingTabIds.has(tab.id)))
    setActiveTabId(result.activeTabId)
    setTabContextMenu(null)
  }

  return (
    <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden', backgroundColor: 'background.default' }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          minHeight: 36,
          borderBottom: '1px solid',
          borderColor: 'divider',
          backgroundColor: 'background.paper',
          flexShrink: 0,
        }}
      >
        {tabs.length > 0 && (
          <Tabs
            value={activeTabId || false}
            onChange={(_, tabId) => setActiveTabId(tabId)}
            variant="scrollable"
            scrollButtons="auto"
            sx={{
              minHeight: 36,
              flex: 1,
              minWidth: 0,
              '& .MuiTab-root': {
                minHeight: 36,
                minWidth: 0,
                px: 1.25,
                py: 0,
                fontSize: 11,
                textTransform: 'none',
              },
            }}
          >
            {tabs.map((tab) => {
              const label = browserTabLabel(tab)
              return (
                <Tab
                  key={tab.id}
                  value={tab.id}
                  onContextMenu={(event) => {
                    event.preventDefault()
                    event.stopPropagation()
                    setTabContextMenu({ left: event.clientX, tabId: tab.id, top: event.clientY })
                  }}
                  icon={<Globe2 size={13} />}
                  iconPosition="start"
                  label={(
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                      <Tooltip title={tab.address}>
                        <Box component="span" sx={{ maxWidth: 150, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {label}
                        </Box>
                      </Tooltip>
                      <IconButton
                        component="span"
                        size="small"
                        aria-label={`Close ${label}`}
                        onClick={(event) => {
                          event.stopPropagation()
                          closeTabs(tab.id, 'close')
                        }}
                        sx={{ p: 0.15 }}
                      >
                        <X size={11} />
                      </IconButton>
                    </Box>
                  )}
                />
              )
            })}
          </Tabs>
        )}
        <Tooltip title="New browser tab">
          <IconButton aria-label="New browser tab" onClick={addTab} sx={{ ...browserButtonSx, mx: 0.5 }}>
            <Plus />
          </IconButton>
        </Tooltip>
      </Box>

      <Menu
        open={tabContextMenu !== null}
        onClose={() => setTabContextMenu(null)}
        anchorReference="anchorPosition"
        anchorPosition={tabContextMenu ? { left: tabContextMenu.left, top: tabContextMenu.top } : undefined}
        MenuListProps={{
          'aria-label': tabContextMenu ? 'Browser tab options' : undefined,
          dense: true,
        }}
      >
        <MenuItem onClick={() => tabContextMenu && closeTabs(tabContextMenu.tabId, 'close')}>Close</MenuItem>
        <MenuItem
          onClick={() => tabContextMenu && closeTabs(tabContextMenu.tabId, 'close_others')}
          disabled={tabs.length <= 1}
        >
          Close others
        </MenuItem>
        <MenuItem
          onClick={() => tabContextMenu && closeTabs(tabContextMenu.tabId, 'close_right')}
          disabled={!tabContextMenu || tabs.findIndex((tab) => tab.id === tabContextMenu.tabId) === tabs.length - 1}
        >
          Close to the right
        </MenuItem>
        <MenuItem onClick={() => tabContextMenu && closeTabs(tabContextMenu.tabId, 'close_all')}>Close all</MenuItem>
      </Menu>

      {activeTab && (
      <Box
        component="form"
        onSubmit={handleSubmit}
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
          px: 1,
          py: 0.75,
          borderBottom: '1px solid',
          borderColor: 'divider',
          backgroundColor: 'background.paper',
          flexShrink: 0,
        }}
      >
        <Tooltip title="Back">
          <span>
            <IconButton
              aria-label="Browser back"
              disabled={activeTab.historyIndex <= 0}
              onClick={() => moveHistory(activeTab.id, activeTab.historyIndex - 1)}
              sx={browserButtonSx}
            >
              <ArrowLeft />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Forward">
          <span>
            <IconButton
              aria-label="Browser forward"
              disabled={activeTab.historyIndex < 0 || activeTab.historyIndex >= activeTab.history.length - 1}
              onClick={() => moveHistory(activeTab.id, activeTab.historyIndex + 1)}
              sx={browserButtonSx}
            >
              <ArrowRight />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Reload">
          <span>
            <IconButton
              aria-label="Reload browser preview"
              disabled={!current || isLoading}
              onClick={() => updateTab(activeTab.id, (tab) => ({ ...tab, reloadNonce: tab.reloadNonce + 1 }))}
              sx={browserButtonSx}
            >
              {isLoading ? <CircularProgress size={16} /> : <RotateCw />}
            </IconButton>
          </span>
        </Tooltip>
        <TextField
          value={activeTab.address}
          onChange={(event) => updateTab(activeTab.id, (tab) => ({ ...tab, address: event.target.value }))}
          placeholder="http://localhost:8080"
          size="small"
          fullWidth
          inputProps={{ 'aria-label': 'Sandbox browser address' }}
          sx={{
            '& .MuiInputBase-root': { height: 34, borderRadius: 2 },
            '& .MuiInputBase-input': { py: 0.5, fontSize: '0.82rem', fontFamily: 'monospace' },
          }}
        />
        <Tooltip title="Open preview in a new tab">
          <span>
            <IconButton
              component="a"
              aria-label="Open browser preview in new tab"
              aria-disabled={!externalPreviewUrl}
              href={externalPreviewUrl}
              target="_blank"
              rel="noopener noreferrer"
              tabIndex={externalPreviewUrl ? 0 : -1}
              sx={{
                ...browserButtonSx,
                pointerEvents: externalPreviewUrl ? 'auto' : 'none',
                opacity: externalPreviewUrl ? 1 : 0.38,
              }}
            >
              <ExternalLink />
            </IconButton>
          </span>
        </Tooltip>
      </Box>
      )}

      {activeTab?.error && <Alert severity="error" sx={{ borderRadius: 0 }}>{activeTab.error}</Alert>}

      <Box sx={{ flex: 1, minHeight: 0, position: 'relative', overflow: 'hidden', backgroundColor: '#000' }}>
        {tabs.map((tab) => {
          const tabCurrent = tab.historyIndex >= 0 ? tab.history[tab.historyIndex] : undefined
          if (!tabCurrent) return null
          const resolvedPreviewUrl = sandboxPreviewURLWithScheme(
            tabCurrent.previewUrl,
            previewURLHTTPS,
          )
          return (
            <Box
              key={tab.id}
              sx={{
                position: 'absolute',
                inset: 0,
                display: tab.id === activeTabId ? 'block' : 'none',
              }}
            >
              <iframe
                key={`${resolvedPreviewUrl}:${tab.reloadNonce}`}
                src={resolvedPreviewUrl}
                title={`Sandbox browser: ${tabCurrent.displayUrl}`}
                allow="clipboard-read; clipboard-write; fullscreen"
                style={{ width: '100%', height: '100%', border: 0, display: 'block', colorScheme: 'dark' }}
              />
            </Box>
          )
        })}
        {activeTab && !current && (
          <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', p: 3 }}>
            <Box sx={{ maxWidth: 480, textAlign: 'center' }}>
              <Globe2 size={38} strokeWidth={1.4} />
              <Typography variant="h6" sx={{ mt: 1.5 }}>Open a sandbox web app</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.75 }}>
                Enter a localhost URL above, such as http://localhost:8080, then press Enter.
                Traffic goes through Helix to this task&apos;s sandbox.
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1.5 }}>
                Opening a port creates a shareable preview URL. Anyone with that unguessable URL can
                access the port until you revoke it in Details.
              </Typography>
            </Box>
          </Box>
        )}
        {!activeTab && (
          <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', p: 3 }}>
            <Box sx={{ textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">All browser tabs are closed.</Typography>
              <Typography variant="caption" color="text.secondary">Use the + button to open a new tab.</Typography>
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default SandboxBrowser
