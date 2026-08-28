import { FC } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import useRouter from '../hooks/useRouter'

const HelixOrgTopicsRetired: FC = () => {
  const router = useRouter()
  const orgID = router.params.org_id as string
  return <HelixOrgShell showChat={false} breadcrumbs={useHelixOrgBreadcrumbs()} breadcrumbTitle="Topics retired"><Box sx={{ height: '100%', overflow: 'auto' }}><Container maxWidth="md" sx={{ py: 6 }}><Stack spacing={2}><Typography variant="h5">Topics are now Triggers</Typography><Alert severity="info">This is an old Topics link. Open Triggers to choose which events start your agents.</Alert><Box><Button variant="contained" onClick={() => router.navigate('helix_org_triggers', { org_id: orgID })}>Open Triggers</Button></Box></Stack></Container></Box></HelixOrgShell>
}

export default HelixOrgTopicsRetired
