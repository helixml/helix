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

const OrgSidebarMainLink: FC<{
  id?: string;
  routeName: string,
  title: string,
  icon: React.ReactNode,
  includeOrgId?: boolean,
}> = ({
  id,
  routeName,
  title,
  icon,
  includeOrgId = true,
}) => {
  const account = useAccount()
  const router = useRouter()
  const isActive = router.name == routeName

  return (
    <ListItem
      disablePadding
      dense
    >
      <ListItemButton
        id={id}
        selected={ isActive }
        sx={{
          pl: 3,
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
          router.navigate(routeName, includeOrgId ? { org_id: account.organizationTools.organization?.name } : {})
          account.setMobileMenuOpen(false)
        }}
      >
        <ListItemIcon sx={{color: COLORS.GREEN_BUTTON}}>
          { icon }
        </ListItemIcon>
        <ListItemText
          sx={{
            ml: 2,
            p: 1,
          }}
          primary={ title }
        />
      </ListItemButton>
    </ListItem>
  )
}

export default OrgSidebarMainLink
