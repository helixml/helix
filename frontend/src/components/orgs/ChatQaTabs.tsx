import { FC } from 'react'
import Box from '@mui/material/Box'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import { useAccount } from '../../contexts/account'

// Segmented toggle that folds the low-traffic Q&A page under Chat. Rendered at
// the top of both the Chat (Home) and Q&A (QuestionSets) pages; switching just
// navigates between the existing org_chat / org_qa routes.
const ChatQaTabs: FC<{ value: 'chat' | 'qa' }> = ({ value }) => {
  const account = useAccount()
  return (
    <Box sx={{ borderBottom: 1, borderColor: 'divider', px: { xs: 2, sm: 3 } }}>
      <Tabs value={value} onChange={(_, v) => account.orgNavigate(v)}>
        <Tab value="chat" label="Chat" />
        <Tab value="qa" label="Q&A" />
      </Tabs>
    </Box>
  )
}

export default ChatQaTabs
