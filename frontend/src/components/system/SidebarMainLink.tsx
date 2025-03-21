import React, { FC } from 'react'

import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'

import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'

import {
  COLORS,
} from '../../config'

const SidebarMainLink: FC<{
  id?: string;
  routeName: string,
  title: string,
  icon: React.ReactNode,
  handler?: () => void,
}> = ({
  id,
  routeName,
  title,
  icon,
  handler,
}) => {
  const account = useAccount()
  const router = useRouter()
  const isBigScreen = useIsBigScreen()
  const isActive = router.name == routeName
  return (
    <ListItem
      disablePadding
      dense={ isBigScreen ? false : true }
    >
      <ListItemButton
        id={id}
        selected={ isActive }
        sx={{
          height: '64px',
          '&:hover': {
            '.MuiListItemText-root .MuiTypography-root': { color: COLORS.GREEN_BUTTON_HOVER },
            '.MuiListItemIcon-root': { color: COLORS.GREEN_BUTTON_HOVER },
          },
          '.MuiListItemText-root .MuiTypography-root': {
            fontWeight: 'bold',
            color: isActive ? COLORS.GREEN_BUTTON_HOVER : COLORS.GREEN_BUTTON,
          },
          '.MuiListItemIcon-root': {
            color: isActive ? COLORS.GREEN_BUTTON_HOVER : COLORS.GREEN_BUTTON,
          },
        }}
        onClick={ () => {
          account.setMobileMenuOpen(false)
          if(handler) {
            handler()
          } else {
            account.orgNavigate(routeName)
          }
        }}
      >
        <ListItemText
          sx={{
            ml: 2,
            p: 1,
          }}
          primary={ title }
        />
        <ListItemIcon sx={{color: COLORS.GREEN_BUTTON}}>
          { icon }
        </ListItemIcon>
      </ListItemButton>
    </ListItem>
  )
}

export default SidebarMainLink
