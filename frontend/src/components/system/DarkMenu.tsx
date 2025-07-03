import React from 'react'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import { MenuProps } from '@mui/material'

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
        sx: {
          p: 0,
          backgroundColor: 'rgba(26, 26, 26, 0.97)',
          backdropFilter: 'blur(10px)',
          minWidth: '160px',
          borderRadius: '10px',
          border: '1px solid rgba(255,255,255,0.10)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.32)',
          ...menuListProps?.sx,
        },
        ...menuListProps,
      }}
      sx={{
        '& .MuiMenuItem-root': {
          color: 'white',
          fontSize: '0.92rem',
          fontWeight: 500,
          px: 2,
          py: 1,
          minHeight: '32px',
          borderRadius: '6px',
          transition: 'background 0.15s',
          '&:hover': {
            backgroundColor: 'rgba(0,229,255,0.13)',
          },
          '&.Mui-selected': {
            backgroundColor: 'rgba(0,229,255,0.18)',
          },
        },
        '& .MuiDivider-root': {
          borderColor: 'rgba(255,255,255,0.10)',
          my: 0.5,
        },
        ...menuProps.sx,
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