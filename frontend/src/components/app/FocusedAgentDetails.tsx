import { FC, ReactNode } from 'react'
import Alert from '@mui/material/Alert'
import CircularProgress from '@mui/material/CircularProgress'

import { IAppFlatState } from '../../types'
import AppSettings from './AppSettings'
import OrgAgentSettings from './OrgAgentSettings'
import WorkerSecretsPanel from '../helix-org/WorkerSecretsPanel'
import { BotDetailDTO } from '../../services/helixOrgService'
import {
  AgentSettingsPage,
  AgentSettingsRow,
  AgentSettingsSection,
} from './AgentSettingsLayout'

interface FocusedAgentDetailsProps {
  agentID: string
  app: IAppFlatState
  kind: 'coding' | 'org'
  onUpdate: (updates: Partial<IAppFlatState>) => Promise<void>
  onCanonicalUpdate: () => Promise<unknown>
  readOnly: boolean
  showErrors: boolean
  isAdmin: boolean
  orgAgentDetail?: BotDetailDTO
  orgAgentDetailLoading?: boolean
  accessManagement?: ReactNode
}

const FocusedAgentDetails: FC<FocusedAgentDetailsProps> = ({
  agentID,
  app,
  kind,
  onUpdate,
  onCanonicalUpdate,
  readOnly,
  showErrors,
  isAdmin,
  orgAgentDetail,
  orgAgentDetailLoading = false,
  accessManagement,
}) => {
  if (kind === 'org' && orgAgentDetailLoading) {
    return (
      <AgentSettingsPage>
        <AgentSettingsRow>
          <CircularProgress size={22} aria-label="Loading org agent settings" />
        </AgentSettingsRow>
      </AgentSettingsPage>
    )
  }

  if (kind === 'org' && !orgAgentDetail) {
    return (
      <AgentSettingsPage>
        <Alert severity="warning">
          This backing agent is no longer linked to an organization worker. Open the current worker from the Helix Org Agents list.
        </Alert>
      </AgentSettingsPage>
    )
  }

  const appSettings = (view: 'configuration' | 'desktop') => (
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
      externalRuntimeView={view}
      embedded
    />
  )

  return (
    <AgentSettingsPage>
      <AgentSettingsSection
        title="General"
        description={kind === 'org'
          ? 'Set the worker name, coding runtime, model, and reasoning effort.'
          : 'Choose the name used to identify this coding agent across the organization.'}
      >
        <AgentSettingsRow>
          {kind === 'org' ? (
            <OrgAgentSettings
              agentID={agentID}
              section="basics"
              readOnly={readOnly}
              onCanonicalUpdate={onCanonicalUpdate}
              detail={orgAgentDetail}
              embedded
            />
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
              embedded
            />
          )}
        </AgentSettingsRow>
        {kind === 'org' && (
          <AgentSettingsRow>
            <OrgAgentSettings
              agentID={agentID}
              section="runtime"
              readOnly={readOnly}
              detail={orgAgentDetail}
              embedded
            />
          </AgentSettingsRow>
        )}
      </AgentSettingsSection>

      {kind === 'coding' && (
        <AgentSettingsSection
          title="Provider and model"
          description="Choose the coding runtime, credentials, model, and reasoning effort."
        >
          <AgentSettingsRow>{appSettings('configuration')}</AgentSettingsRow>
        </AgentSettingsSection>
      )}

      <AgentSettingsSection
        title="Desktop"
        description="Configure the desktop environment available when this agent starts a session."
      >
        <AgentSettingsRow>{appSettings('desktop')}</AgentSettingsRow>
      </AgentSettingsSection>

      {kind === 'org' && (
        <>
          <AgentSettingsSection
            title="Instructions"
            description="Define this worker's role, operating rules, and expected behavior."
          >
            <AgentSettingsRow>
              <OrgAgentSettings
                agentID={agentID}
                section="instructions"
                readOnly={readOnly}
                onCanonicalUpdate={onCanonicalUpdate}
                detail={orgAgentDetail}
                embedded
              />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Available tools"
            description="Choose the organization capabilities this worker can call."
          >
            <AgentSettingsRow>
              <OrgAgentSettings
                agentID={agentID}
                section="tools"
                readOnly={readOnly}
                detail={orgAgentDetail}
                embedded
              />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Triggers"
            description="Choose what starts this agent. You can use a Trigger directly or the result of a Processor."
          >
            <AgentSettingsRow>
              <OrgAgentSettings
                agentID={agentID}
                section="subscriptions"
                readOnly={readOnly}
                detail={orgAgentDetail}
                embedded
              />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Secrets"
            description="Choose which credentials this agent may retrieve when it needs them."
          >
            <AgentSettingsRow>
              <WorkerSecretsPanel agentID={orgAgentDetail?.bot?.id} projectID={orgAgentDetail?.project_id} readOnly={readOnly} />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Permissions"
            description="Control the projects this worker can use and who can access its backing agent."
          >
            <AgentSettingsRow>
              <OrgAgentSettings
                agentID={agentID}
                section="access"
                readOnly={readOnly}
                detail={orgAgentDetail}
                embedded
              />
            </AgentSettingsRow>
            {accessManagement && <AgentSettingsRow>{accessManagement}</AgentSettingsRow>}
          </AgentSettingsSection>
        </>
      )}
    </AgentSettingsPage>
  )
}

export default FocusedAgentDetails
