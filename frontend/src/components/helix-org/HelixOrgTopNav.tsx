// Elegant top-bar nav for helix-org (Chart / Agents / Topics / Assets).
// Renders next to theme + notifications on the Page AppBar.

import { FC } from 'react'
import Box from '@mui/material/Box'
import ButtonBase from '@mui/material/ButtonBase'
import Tooltip from '@mui/material/Tooltip'
import DnsOutlinedIcon from '@mui/icons-material/DnsOutlined'
import { Bot, Network, RadioTower } from 'lucide-react'

import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'

type NavItem = {
  id: string
  label: string
  route: string
  icon: React.ReactNode
  isActive: (routeName: string) => boolean
}

const ITEMS: NavItem[] = [
  {
    id: 'chart',
    label: 'Chart',
    route: 'helix_org_chart',
    icon: <Network size={16} />,
    isActive: (n) => n === 'helix_org_chart' || n === 'helix_org_root',
  },
  {
    id: 'agents',
    label: 'Agents',
    route: 'org_agents',
    icon: <Bot size={16} />,
    isActive: (n) => n === 'org_agents' || n === 'org_agent',
  },
  {
    id: 'triggers',
    label: 'Triggers',
    route: 'helix_org_triggers',
    icon: <RadioTower size={16} />,
    isActive: (n) => n === 'helix_org_triggers' || n === 'helix_org_trigger_detail',
  },
  {
    id: 'assets',
    label: 'Assets',
    route: 'helix_org_assets',
    icon: <DnsOutlinedIcon sx={{ fontSize: 16 }} />,
    isActive: (n) => n === 'helix_org_assets',
  },
]

const HelixOrgTopNav: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const orgId = router.params.org_id as string | undefined
  const routeName = router.name

  const go = (route: string) => {
    if (!orgId) return
    router.navigate(route, { org_id: orgId })
    account.setMobileMenuOpen(false)
  }

  const isLight = lightTheme.isLight
  const trackBg = isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)'
  const activeBg = isLight ? '#fff' : 'rgba(255,255,255,0.12)'
  const activeShadow = isLight
    ? '0 1px 2px rgba(0,0,0,0.08), 0 0 0 1px rgba(0,0,0,0.04)'
    : '0 0 0 1px rgba(255,255,255,0.1)'
  const idleColor = lightTheme.textColorFaded
  const activeColor = lightTheme.textColor

  return (
    <Box
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 0.25,
        p: 0.35,
        borderRadius: 2,
        backgroundColor: trackBg,
        mr: 1,
      }}
    >
      {ITEMS.map((item) => {
        const active = item.isActive(routeName)
        return (
          <Tooltip key={item.id} title={item.label} enterDelay={600}>
            <ButtonBase
              onClick={() => go(item.route)}
              sx={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 0.75,
                px: { xs: 1, sm: 1.25 },
                py: 0.6,
                borderRadius: 1.5,
                fontSize: '0.8rem',
                fontWeight: active ? 600 : 500,
                color: active ? activeColor : idleColor,
                backgroundColor: active ? activeBg : 'transparent',
                boxShadow: active ? activeShadow : 'none',
                transition: 'background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease',
                '&:hover': {
                  backgroundColor: active ? activeBg : (isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)'),
                  color: activeColor,
                },
              }}
            >
              {item.icon}
              <Box component="span" sx={{ display: { xs: 'none', md: 'inline' } }}>
                {item.label}
              </Box>
            </ButtonBase>
          </Tooltip>
        )
      })}
    </Box>
  )
}

export default HelixOrgTopNav
