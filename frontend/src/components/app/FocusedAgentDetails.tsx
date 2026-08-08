import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import { IAppFlatState } from '../../types'
import AppSettings from './AppSettings'
import OrgAgentSettings from './OrgAgentSettings'

interface FocusedAgentDetailsProps {
  agentID: string
  app: IAppFlatState
  kind: 'coding' | 'org'
  onUpdate: (updates: Partial<IAppFlatState>) => Promise<void>
  onCanonicalUpdate: () => Promise<unknown>
  readOnly: boolean
  showErrors: boolean
  isAdmin: boolean
  accessManagement?: ReactNode
}

const SectionDivider = () => <Divider sx={{ my: 4 }} />

const FocusedAgentDetails: FC<FocusedAgentDetailsProps> = ({
  agentID,
  app,
  kind,
  onUpdate,
  onCanonicalUpdate,
  readOnly,
  showErrors,
  isAdmin,
  accessManagement,
}) => (
  <Box sx={{ maxWidth: 880, pb: 8 }}>
    <Stack spacing={0}>
      <Box sx={{ mb: 4 }}>
        <Typography variant="h4" sx={{ mb: 0.75 }}>
          {kind === 'org' ? 'Helix org agent' : 'Coding agent'}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {kind === 'org'
            ? 'Configure the worker runtime, desktop, organization tools, and permissions.'
            : 'Configure the coding harness, model, and desktop environment.'}
        </Typography>
      </Box>

      {kind === 'org' ? (
        <>
          <OrgAgentSettings
            agentID={agentID}
            section="basics"
            readOnly={readOnly}
            onCanonicalUpdate={onCanonicalUpdate}
          />
          <OrgAgentSettings agentID={agentID} section="runtime" readOnly={readOnly} />
        </>
      ) : (
        <AppSettings
          id={agentID}
          app={app}
          onUpdate={onUpdate}
          readOnly={readOnly}
          showErrors={showErrors}
          isAdmin={isAdmin}
          section="general"
          focusedExternal
        />
      )}

      <SectionDivider />

      <AppSettings
        id={agentID}
        app={app}
        onUpdate={onUpdate}
        readOnly={readOnly}
        showErrors={showErrors}
        isAdmin={isAdmin}
        section="runtime"
        hideAgentType
        focusedExternal
        externalRuntimeView={kind === 'org' ? 'desktop' : 'all'}
      />

      {kind === 'org' && (
        <>
          <SectionDivider />
          <Box>
            <Typography variant="h5" sx={{ mb: 0.5 }}>Available tools</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Choose the organization capabilities this worker can call.
            </Typography>
            <OrgAgentSettings agentID={agentID} section="tools" readOnly={readOnly} />
          </Box>

          <SectionDivider />
          <Box>
            <Typography variant="h5" sx={{ mb: 0.5 }}>Permissions</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Control the projects this worker can use and who can access its backing agent.
            </Typography>
            <OrgAgentSettings agentID={agentID} section="access" readOnly={readOnly} />
            {accessManagement}
          </Box>
        </>
      )}
    </Stack>
  </Box>
)

export default FocusedAgentDetails
