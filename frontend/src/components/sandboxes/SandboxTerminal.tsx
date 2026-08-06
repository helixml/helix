import { FC, useCallback, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select, { SelectChangeEvent } from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import RestartAltIcon from '@mui/icons-material/RestartAlt'

import {
  sandboxTerminalUrl,
  useSandboxTerminalSessions,
} from '../../services/sandboxesService'
import PersistentTerminalPane from '../session/PersistentTerminalPane'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
  // height of the rendered terminal — defaults to a roomy value for the tab
  // view, callers (e.g. the card preview) can pass a smaller value.
  height?: number | string
  // showControls renders a small toolbar with the session selector and a
  // "New session" button. Disabled for compact previews.
  showControls?: boolean
  // readOnly suppresses keyboard input — used for the read-only card preview
  // so accidental clicks don't run shell commands.
  readOnly?: boolean
  // fillContainer makes the component stretch to its parent's height instead
  // of sizing to its `height` prop. Used when the parent already constrains
  // height (e.g. an aspect-ratio box on a card).
  fillContainer?: boolean
}

interface PersistentTerminalSession {
  name: string
  attached: boolean
  windows?: number
  created?: number
}

interface PersistentTerminalProps {
  storageKey: string
  targetId: string
  running: boolean
  terminalUrl: (sessionName: string) => string
  sessions?: PersistentTerminalSession[]
  unavailableMessage: string
  newSessionTooltip?: string
  height?: number | string
  showControls?: boolean
  readOnly?: boolean
  fillContainer?: boolean
}

const sandboxSessionStorageKey = (sandboxId: string) => `helix.sandbox.${sandboxId}.terminalSession`

const generateSessionName = (): string => {
  const bytes = crypto.getRandomValues(new Uint8Array(6))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

const readStoredSession = (storageKey: string): string | undefined => {
  try {
    const v = window.localStorage.getItem(storageKey)
    return v || undefined
  } catch {
    return undefined
  }
}

const writeStoredSession = (storageKey: string, name: string) => {
  try {
    window.localStorage.setItem(storageKey, name)
  } catch {
    // ignore — best-effort persistence
  }
}

// PersistentTerminal is transport-independent: callers provide the endpoint,
// persisted-selection key, and tmux session list for their container type.
const PersistentTerminal: FC<PersistentTerminalProps> = ({
  storageKey,
  targetId,
  running,
  terminalUrl,
  sessions = [],
  unavailableMessage,
  newSessionTooltip = 'Start a fresh tmux session',
  height = 480,
  showControls = true,
  readOnly = false,
  fillContainer = false,
}) => {
  const [sessionName, setSessionName] = useState<string>(() => {
    const stored = readStoredSession(storageKey)
    if (stored) return stored
    const fresh = generateSessionName()
    writeStoredSession(storageKey, fresh)
    return fresh
  })
  const websocketUrl = terminalUrl(sessionName)

  // Build the dropdown options: every helix-managed session reported by the
  // backend, plus the locally selected one if it hasn't been observed yet
  // (which is the normal case immediately after "New session" — tmux only
  // materialises the session when the websocket connects).
  const sessionOptions = useMemo<string[]>(() => {
    const set = new Set<string>()
    set.add(sessionName)
    for (const s of sessions) {
      if (s?.name) set.add(s.name)
    }
    return Array.from(set)
  }, [sessionName, sessions])

  const handleNewSession = useCallback(() => {
    const fresh = generateSessionName()
    writeStoredSession(storageKey, fresh)
    setSessionName(fresh)
  }, [storageKey])

  const handleSelectSession = useCallback(
    (e: SelectChangeEvent<string>) => {
      const next = e.target.value
      if (!next || next === sessionName) return
      writeStoredSession(storageKey, next)
      setSessionName(next)
    },
    [sessionName, storageKey],
  )

  if (!running) {
    return (
      <Box
        sx={{
          p: 4,
          textAlign: 'center',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 1,
        }}
      >
        <Typography variant="body2" color="text.secondary">
          {unavailableMessage}
        </Typography>
      </Box>
    )
  }

  return (
    <Stack
      spacing={1}
      sx={{
        width: '100%',
        ...(fillContainer ? { height: '100%' } : {}),
      }}
    >
      {showControls && (
        <Stack direction="row" alignItems="center" spacing={1.5} sx={{ minHeight: 40 }}>
          <FormControl size="small" sx={{ minWidth: 240 }}>
            <InputLabel id={`terminal-${targetId}-session-label`}>Session</InputLabel>
            <Select
              labelId={`terminal-${targetId}-session-label`}
              label="Session"
              value={sessionName}
              onChange={handleSelectSession}
              renderValue={(v) => `helix-${v}`}
            >
              {sessionOptions.map((name) => {
                const meta = sessions.find((s) => s.name === name)
                return (
                  <MenuItem key={name} value={name}>
                    <Stack direction="row" alignItems="center" spacing={1}>
                      <Box component="span" sx={{ fontFamily: 'monospace' }}>
                        helix-{name}
                      </Box>
                      {meta?.attached && (
                        <Typography variant="caption" color="text.secondary">
                          (attached)
                        </Typography>
                      )}
                      {!meta && (
                        <Typography variant="caption" color="text.secondary">
                          (new)
                        </Typography>
                      )}
                    </Stack>
                  </MenuItem>
                )
              })}
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary">
            Reconnects reattach to the selected tmux session.
          </Typography>
          <Box sx={{ flex: 1 }} />
          <Tooltip title={newSessionTooltip}>
            <Button
              size="small"
              variant="outlined"
              startIcon={<RestartAltIcon fontSize="small" />}
              onClick={handleNewSession}
            >
              New session
            </Button>
          </Tooltip>
        </Stack>
      )}
      <Box
        sx={{
          p: 1,
          bgcolor: '#000',
          ...(fillContainer
            ? { flex: 1, minHeight: 0 }
            : {
                height,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
              }),
        }}
      >
        <PersistentTerminalPane
          key={websocketUrl}
          websocketUrl={websocketUrl}
          readOnly={readOnly}
          active={!readOnly}
        />
      </Box>
    </Stack>
  )
}

const SandboxTerminal: FC<Props> = ({
  orgId,
  sandboxId,
  running,
  height = 480,
  showControls = true,
  readOnly = false,
  fillContainer = false,
}) => {
  const sessionsQuery = useSandboxTerminalSessions(
    showControls && running ? orgId : undefined,
    showControls && running ? sandboxId : undefined,
  )

  return (
    <PersistentTerminal
      key={sandboxId}
      storageKey={sandboxSessionStorageKey(sandboxId)}
      targetId={sandboxId}
      running={running}
      terminalUrl={(sessionName) => sandboxTerminalUrl(orgId, sandboxId, sessionName)}
      sessions={sessionsQuery.data?.sessions}
      unavailableMessage='Sandbox is not running yet — terminal will be available when status is "running".'
      newSessionTooltip="Start a fresh tmux session in this sandbox"
      height={height}
      showControls={showControls}
      readOnly={readOnly}
      fillContainer={fillContainer}
    />
  )
}

export default SandboxTerminal
