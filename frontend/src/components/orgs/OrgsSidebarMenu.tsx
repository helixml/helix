import React, { FC } from 'react'
import List from '@mui/material/List'
import OrgSidebarMainLink from './OrgSidebarMainLink'
import ArrowBack from '@mui/icons-material/ArrowBack'
import SettingsIcon from '@mui/icons-material/Settings'
import Person from '@mui/icons-material/Person'
import GroupsIcon from '@mui/icons-material/Groups'
import PaymentIcon from '@mui/icons-material/Payment'

const OrgsSidebarMenu: FC<{
  
}> = ({
  
}) => {
  return (
    <List disablePadding>
      <OrgSidebarMainLink
        id="exit-link"
        routeName="home"
        title="Exit"
        icon={ <ArrowBack/> }
      />
      <OrgSidebarMainLink
        id="settings-link"
        routeName="org_settings"
        title="Settings"
        icon={ <SettingsIcon/> }
      />
      <OrgSidebarMainLink
        id="teams-link"
        routeName="org_teams"
        title="Teams"
        icon={ <GroupsIcon/> }
      />
      <OrgSidebarMainLink
        id="people-link"
        routeName="org_people"
        title="People"
        icon={ <Person/> }
      />
      {/* <OrgSidebarMainLink
        id="billing-link"
        routeName="home"
        title="Billing"
        icon={ <PaymentIcon/> }
      /> */}
    </List>
  )
}

export default OrgsSidebarMenu 