import React, { FC } from 'react'
import List from '@mui/material/List'
import OrgSidebarMainLink from './OrgSidebarMainLink'
import ArrowBack from '@mui/icons-material/ArrowBack'
import SettingsIcon from '@mui/icons-material/Settings'
import Person from '@mui/icons-material/Person'
import GroupsIcon from '@mui/icons-material/Groups'
import PaymentIcon from '@mui/icons-material/Payment'
import SlideMenuContainer, { triggerMenuChange } from '../system/SlideMenuContainer'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'

// Menu identifier constant
const MENU_TYPE = 'orgs'

const OrgsSidebarMenu: FC<{
  
}> = ({
  
}) => {
  const router = useRouter()
  const account = useAccount()
  
  // Handler for menu exit animation
  const handleExitClick = () => {
    // Determine which resource type to navigate to (chat is default)
    const navigateToType = localStorage.getItem('last_resource_type') || 'chat'
    console.log(`[EXIT CLICK] Clicked Exit button. Current route: ${router.name}, navigating to resource type: ${navigateToType}`);
    
    // For settings -> org home transition:
    // Settings slides RIGHT, org home comes in FROM LEFT
    setTimeout(() => {
      if (window._activeMenus) {
        if (router.name === 'org_settings' || router.name === 'org_people' || router.name === 'org_teams') {
          console.log(`[EXIT CLICK] In a settings view (${router.name}). Triggering with direction RIGHT. This means:`);
          console.log(`[EXIT CLICK] - Current settings view will slide OUT to the RIGHT (100%)`);
          console.log(`[EXIT CLICK] - Home view will start at LEFT (-100%) and slide IN to CENTER`);
          
          // When exiting org settings panels to main org page
          // RIGHT direction means current slides right, new comes from left
          triggerMenuChange('orgs', 'orgs', 'right', false);
        } else {
          console.log(`[EXIT CLICK] Exiting org completely with direction RIGHT. This means org view moves RIGHT and personal home comes from LEFT`);
          
          // When exiting org completely to personal home
          if (window._activeMenus['orgs']) {
            triggerMenuChange('orgs', navigateToType, 'right', true);
          }
        }
      }
    }, 50);
    
    // Navigate after animation begins
    setTimeout(() => {
      console.log(`[EXIT CLICK] Navigating now`);
      
      // If we're in an organization, navigate to org_home, otherwise to personal home
      if (account.organizationTools.organization && account.organizationTools.organization.name) {
        console.log(`[EXIT CLICK] Navigating to org_home for ${account.organizationTools.organization.name}`);
        router.navigate('org_home', { 
          org_id: account.organizationTools.organization.name,
          resource_type: navigateToType
        })
      } else {
        console.log(`[EXIT CLICK] Navigating to personal home`);
        router.navigate('home', { resource_type: navigateToType })
      }
    }, 100);
  }
  
  // Store the current resource type for later use when exiting
  React.useEffect(() => {
    const currentResourceType = router.params.resource_type || 'chat'
    localStorage.setItem('last_resource_type', currentResourceType)
  }, [router.params.resource_type])

  return (
    <SlideMenuContainer menuType={MENU_TYPE}>
      <List disablePadding>
        <OrgSidebarMainLink
          id="exit-link"
          routeName="orgs"
          title="Exit"
          icon={<ArrowBack/>}
          includeOrgId={false}
          onBeforeNavigate={handleExitClick}
        />
        <OrgSidebarMainLink
          id="people-link"
          routeName="org_people"
          title="People"
          icon={<Person/>}
        />
        <OrgSidebarMainLink
          id="teams-link"
          routeName="org_teams"
          title="Teams"
          icon={<GroupsIcon/>}
        />
        <OrgSidebarMainLink
          id="settings-link"
          routeName="org_settings"
          title="Settings"
          icon={<SettingsIcon/>}
        />
        {/* <OrgSidebarMainLink
          id="billing-link"
          routeName="home"
          title="Billing"
          icon={<PaymentIcon/>}
        /> */}
      </List>
    </SlideMenuContainer>
  )
}

export default OrgsSidebarMenu 