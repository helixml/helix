import React from 'react'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import { MenuProps } from '@mui/material'

/**
 * DarkMenu component - now optional since dark menu styling is applied globally
 * to all Menu components via the Material-UI theme.
 * 
 * You can use either:
 * 1. Regular <Menu> component (recommended) - automatically gets dark styling
 * 2. <DarkMenu> component - same as regular Menu but with aria-labelledby preset
 */
interface DarkMenuProps extends Omit<MenuProps, 'MenuListProps'> {
  children: React.ReactNode
  menuListProps?: MenuProps['MenuListProps']
}

const DarkMenu: React.FC<DarkMenuProps> = ({ 
  children, 
  menuListProps,
  ...menuProps 
}) => {
  return (
    <Menu
      {...menuProps}
      MenuListProps={{
        'aria-labelledby': 'dark-menu',
        ...menuListProps,
      }}
    >
      {children}
    </Menu>
  )
}

// Helper component for consistent menu items
export const DarkMenuItem: React.FC<{
  onClick?: () => void
  disabled?: boolean
  children: React.ReactNode
}> = ({ onClick, disabled, children }) => {
  return (
    <MenuItem onClick={onClick} disabled={disabled}>
      <Typography 
        variant="body2" 
        sx={{ 
          color: '#fff', 
          fontWeight: 500, 
          fontSize: '0.92rem' 
        }}
      >
        {children}
      </Typography>
    </MenuItem>
  )
}

export default DarkMenu 