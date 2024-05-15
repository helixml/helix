import React, { FC, useRef, useState } from 'react'
import { useTheme } from '@mui/material/styles'
import Typography from '@mui/material/Typography'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'

const SelectOption: FC<{
  value: string,
  options: string[],
  onSetValue: (value: string) => void,
}> = ({
  value,
  options,
  onSetValue,
}) => {
  const theme = useTheme()
  const modeMenuRef = useRef<null | HTMLElement>(null)
  const [ modeMenuAnchorEl, setModeMenuAnchorEl ] = useState<null | HTMLElement>(null)

  return (
    <>
      <Typography
        onClick={(event: React.MouseEvent<HTMLElement>) => {
          setModeMenuAnchorEl(event.currentTarget)
        }}
        ref={modeMenuRef}
        className="inferenceTitle"
        variant="h6"
        color="inherit"
        noWrap
        sx={{
          flexGrow: 1,
          mx: 0,
          color: 'text.primary',
          borderRadius: '15px',
          padding: "3px",
          "&:hover": {
            backgroundColor: theme.palette.mode === 'light' ? "#efefef" : "#13132b",
          },
        }}
      >
        &nbsp;&nbsp;{ value } <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
      </Typography>
      <Menu
        id="mode-menu"
        open={ modeMenuAnchorEl ? true : false }
        onClose={ () => setModeMenuAnchorEl(null) }
        anchorEl={ modeMenuRef.current }
        sx={{
          marginTop:"50px",
          zIndex: 9999,
        }}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'center',
          horizontal: 'left',
        }}
      >
        {
          options.map((option) => (
            <MenuItem
              key={ option }
              selected={ value === option }
              onClick={() => {
                onSetValue(option)
                setModeMenuAnchorEl(null)
              }}
            >
              { option }
            </MenuItem>
          ))
        }
      </Menu>
    </>
  )
}

export default SelectOption
