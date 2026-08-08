import React, { FC, useCallback, useEffect, useMemo, useState } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import Typography from '@mui/material/Typography'
import { Plus } from 'lucide-react'

import Page from '../components/system/Page'
import PageSectionHeader from '../components/system/PageSectionHeader'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'
import NewAgentDialog from '../components/agent/NewAgentDialog'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useSubscriptionGate from '../hooks/useSubscriptionGate'
import Paywall from '../components/subscription/Paywall'
import HelixOrgTopNav from '../components/helix-org/HelixOrgTopNav'

import {
  AGENT_KIND_CODING,
  AGENT_KIND_HELIX,
  AGENT_KIND_ORG,
  IApp,
} from '../types'

const agentTabs = [
  {
    value: AGENT_KIND_CODING,
    label: 'Coding Agents',
    description: 'External coding harnesses for projects and spec tasks',
  },
  {
    value: AGENT_KIND_HELIX,
    label: 'Helix Agents',
    description: 'Native Helix agents for chat, tools, and automations',
  },
  {
    value: AGENT_KIND_ORG,
    label: 'Helix Org Agents',
    description: 'Workers managed through the Helix organization chart',
  },
]

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const { paywallActive, navigateToBilling } = useSubscriptionGate()

  const {
    params,
    mergeParams,
  } = useRouter()

  const selectedKind = agentTabs.some((tab) => tab.value === params.kind)
    ? params.kind
    : AGENT_KIND_CODING
  const visibleApps = useMemo(
    () => apps.apps.filter((app) => app.agent_kind === selectedKind),
    [apps.apps, selectedKind],
  )
  const [ deletingApp, setDeletingApp ] = useState<IApp>()
  const [newAgentOpen, setNewAgentOpen] = useState(false)

  const onEditApp = (app: IApp) => {
    account.orgNavigate('agent', {
      app_id: app.id,
    })
  }

  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const onNewAgent = () => {
    if(!checkLoginStatus()) return
    setNewAgentOpen(true)
  }

  const onAgentCreated = (kind: string, id: string) => {
    setNewAgentOpen(false)
    snackbar.success('Agent created')
    if (kind === AGENT_KIND_HELIX) {
      account.orgNavigate('agent', { app_id: id })
      return
    }
    mergeParams({ kind })
  }

  const onDeleteApp = useCallback(async () => {
    if(!deletingApp) return
    const result = await apps.deleteApp(deletingApp.id)
    if(!result) return
    setDeletingApp(undefined)
    apps.loadApps()
    snackbar.success('Agent deleted')
  }, [
    deletingApp,
    apps.deleteApp,
  ])  

  useEffect(() => {
    if(!params.snackbar_message) return
    snackbar.success(params.snackbar_message)
  }, [
    params.snackbar_message,
  ])

  useEffect(() => {
    if(account.user) {
      apps.loadApps()
    }
  }, [
    account, apps.loadApps,
  ])
  
  return (
    <Page
      breadcrumbTitle="Agents"
      orgBreadcrumbs={ true }
      globalSearch={true}
      notifications={true}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <>
          <HelixOrgTopNav />
        </>
      )}
    >
      <Container
        maxWidth="lg"
        sx={{
          mb: 4,
          mt: 4,
        }}
      >
        <PageSectionHeader
          title="Agents"
          description="Agents in this org. Click an agent to edit instructions, tools and subscriptions."
          action={
            <Button
              id="new-app-button"
              variant="contained"
              color="secondary"
              size="small"
              startIcon={<Plus size={18} />}
              onClick={onNewAgent}
            >
              New Agent
            </Button>
          }
        />
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
          <Tabs
            value={selectedKind}
            onChange={(_, value: string) => mergeParams({ kind: value })}
            aria-label="Agent kinds"
            variant="fullWidth"
            sx={{
              minHeight: 64,
              '& .MuiTabs-indicator': {
                height: 2,
                borderRadius: '2px 2px 0 0',
              },
            }}
          >
            {agentTabs.map((tab) => (
              <Tab
                key={tab.value}
                value={tab.value}
                disableRipple
                label={(
                  <Box sx={{ textAlign: 'left', width: '100%' }}>
                    <Typography
                      component="span"
                      variant="body2"
                      color="text.primary"
                      sx={{ display: 'block', fontWeight: 600, lineHeight: 1.3 }}
                    >
                      {tab.label}
                    </Typography>
                    <Typography
                      component="span"
                      variant="caption"
                      color="text.secondary"
                      sx={{ display: 'block', mt: 0.25, lineHeight: 1.3, textTransform: 'none' }}
                    >
                      {tab.description}
                    </Typography>
                  </Box>
                )}
                sx={{
                  minHeight: 64,
                  alignItems: 'flex-start',
                  px: 2,
                  py: 1.25,
                  textTransform: 'none',
                }}
              />
            ))}
          </Tabs>
        </Box>
        <Paywall active={paywallActive} onBillingClick={navigateToBilling}>
          <AppsTable
            authenticated={ !!account.user }
            data={ visibleApps }
            onEdit={ onEditApp }
            onDelete={ setDeletingApp }
            orgId={ account.organizationTools.organization?.id || '' }
            agentKind={selectedKind}
          />
        </Paywall>
      </Container>
      {
        deletingApp && (
          <DeleteConfirmWindow
            title="this agent"
            onCancel={ () => setDeletingApp(undefined) }
            onSubmit={ onDeleteApp }
          />
        )
      }
      <NewAgentDialog
        open={newAgentOpen}
        initialKind={selectedKind}
        onClose={() => setNewAgentOpen(false)}
        onCreated={onAgentCreated}
      />
    </Page>
  )
}

export default Apps
