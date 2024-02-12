import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Typography from '@mui/material/Typography'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'

import CollectionHeader from '../components/collection/CollectionHeader'
import useAccount from '../hooks/useAccount'

const Collection: FC = () => {
  const account = useAccount()
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  // Fixture data for demonstration purposes
  const fixtureData = {
    owner: '123', // Assuming '123' is the id of the fixture owner
    name: 'My Collection',
    items: [
      { id: '1', name: 'Item 1' },
      { id: '2', name: 'Item 2' },
      { id: '3', name: 'Item 3' },
    ],
  }

  const isOwner = account.user?.id === fixtureData.owner

  return (
    <>
      <Box
        sx={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Box
          sx={{
            width: '100%',
            flexGrow: 0,
            py: 1,
            px: 2,
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            justifyContent: 'center',
            borderBottom: theme.palette.mode === 'light' ? themeConfig.lightBorder: themeConfig.darkBorder,
          }}
        >
          <CollectionHeader
            collection={fixtureData}
            onOpenMobileMenu={() => {}}
          />
        </Box>
      </Box>
      <Box
        sx={{
          height: 'calc(100% - 100px)',
          mt: 12,
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Box
          sx={{
            width: '100%',
            flexGrow: 0,
            py: 1,
            px: 2,
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            justifyContent: 'center',
            borderBottom: `1px solid ${theme.palette.divider}`,
          }}
        >
          {(isOwner || account.admin) && (
            <Typography variant="h6">Collection: {fixtureData.name}</Typography>
          )}
        </Box>
        <Box
          id="collection-scroller"
          sx={{
            width: '100%',
            flexGrow: 1,
            overflowY: 'auto',
            p: 2,
            '&::-webkit-scrollbar': {
              width: '4px',
              borderRadius: '8px',
              my: 2,
            },
            '&::-webkit-scrollbar-track': {
              background: theme.palette.background.paper,
            },
            '&::-webkit-scrollbar-thumb': {
              background: theme.palette.action.active,
              borderRadius: '8px',
            },
            '&::-webkit-scrollbar-thumb:hover': {
              background: theme.palette.action.hover,
            },
          }}
        >
          <Container maxWidth="lg">
            {fixtureData.items.map(item => (
              <Typography key={item.id} variant="body1" sx={{ mt: 2 }}>
                {item.name}
              </Typography>
            ))}
          </Container>
        </Box>
      </Box>
    </>
  )
}

export default Collection
