import { FC, ReactNode } from 'react'

import { IAppFlatState } from '../../types'
import AppSettings from './AppSettings'
import OrgAgentSettings from './OrgAgentSettings'
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
  accessManagement,
}) => {
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
          ? 'Set the worker name, coding harness, model, and reasoning effort.'
          : 'Choose the name used to identify this coding agent across the organization.'}
      >
        <AgentSettingsRow>
          {kind === 'org' ? (
            <OrgAgentSettings
              agentID={agentID}
              section="basics"
              readOnly={readOnly}
              onCanonicalUpdate={onCanonicalUpdate}
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
            <OrgAgentSettings agentID={agentID} section="runtime" readOnly={readOnly} embedded />
          </AgentSettingsRow>
        )}
      </AgentSettingsSection>

      {kind === 'coding' && (
        <AgentSettingsSection
          title="Provider and model"
          description="Choose the coding harness, credentials, model, and reasoning effort."
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
                embedded
              />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Available tools"
            description="Choose the organization capabilities this worker can call."
          >
            <AgentSettingsRow>
              <OrgAgentSettings agentID={agentID} section="tools" readOnly={readOnly} embedded />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Subscriptions"
            description="Choose the organization topics that trigger this worker."
          >
            <AgentSettingsRow>
              <OrgAgentSettings agentID={agentID} section="subscriptions" readOnly={readOnly} embedded />
            </AgentSettingsRow>
          </AgentSettingsSection>

          <AgentSettingsSection
            title="Permissions"
            description="Control the projects this worker can use and who can access its backing agent."
          >
            <AgentSettingsRow>
              <OrgAgentSettings agentID={agentID} section="access" readOnly={readOnly} embedded />
            </AgentSettingsRow>
            {accessManagement && <AgentSettingsRow>{accessManagement}</AgentSettingsRow>}
          </AgentSettingsSection>
        </>
      )}
    </AgentSettingsPage>
  )
}

export default FocusedAgentDetails
