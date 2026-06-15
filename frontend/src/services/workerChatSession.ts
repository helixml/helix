// workerChatSession resolves a Worker's per-Worker project "Human Desktop"
// session — the single long-lived exploratory session that IS the chat
// surface (Zed ⇄ Claude Code). Two entry points, deliberately split by
// their side effects:
//
//   - fetchExistingWorkerSession: GET-only. Never spins up infra. Safe to
//     call on page load to auto-show the inline transcript when a session
//     already exists.
//   - ensureRunningWorkerSession: GET → create-if-missing → resume-if-paused.
//     Spins up the desktop container. Used by the "Open / Provision Human
//     Desktop" actions.
//
// Both take their API calls injected so the branching is unit-testable
// without an axios client (see workerChatSession.test.ts).

export interface ExploratorySession {
  id?: string
  config?: { external_agent_status?: string }
}

export interface WorkerChatReader {
  // Returns the project's existing exploratory session, or null when none
  // exists. Implementations map the API's 204 No Content to null.
  getExploratorySession: (projectID: string) => Promise<ExploratorySession | null>
}

export interface WorkerChatApi extends WorkerChatReader {
  createExploratorySession: (projectID: string) => Promise<ExploratorySession>
  resumeSession: (sessionID: string) => Promise<unknown>
}

// fetchExistingWorkerSession returns the id of the project's long-lived
// exploratory session if one already exists, else null. No side effects.
export async function fetchExistingWorkerSession(
  projectID: string,
  api: WorkerChatReader,
): Promise<string | null> {
  const session = await api.getExploratorySession(projectID)
  return session?.id ?? null
}

// ensureRunningWorkerSession returns a running exploratory session id,
// creating one if the project has none and resuming it if it was paused
// (external_agent_status === 'stopped') so the caller never lands on a
// dead desktop.
export async function ensureRunningWorkerSession(
  projectID: string,
  api: WorkerChatApi,
): Promise<string> {
  let session = await api.getExploratorySession(projectID)
  if (!session?.id) {
    session = await api.createExploratorySession(projectID)
  } else if (session.config?.external_agent_status === 'stopped') {
    await api.resumeSession(session.id)
  }
  if (!session?.id) {
    throw new Error('failed to open Human Desktop session')
  }
  return session.id
}
