import React, { FC, useState, useRef } from 'react'
import { useTheme } from '@mui/material/styles'
import Box from '@mui/material/Box'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useIsBigScreen from '../../hooks/useIsBigScreen'

import {
  ISessionMode,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
} from '../../types'

const SessionModeSwitch: FC<{
  mode?: ISessionMode,
  onSetMode: (mode: ISessionMode) => void,
}> = ({
  mode = SESSION_MODE_INFERENCE,
  onSetMode,
}) => {
  const theme = useTheme()
  const bigScreen = useIsBigScreen()
  const modeMenuRef = useRef<null | HTMLElement>(null)
  const [ modeMenuAnchorEl, setModeMenuAnchorEl ] = useState<null | HTMLElement>(null)

  return bigScreen ? (
    <Row>
      <Cell>
        <Typography
          sx={{
            color: mode === SESSION_MODE_INFERENCE ? 'text.primary' : 'text.secondary',
            fontWeight: mode === SESSION_MODE_INFERENCE ? 'bold' : 'normal',
            mr: 2,
            ml: 3,
            textAlign: 'right',
          }}
        >
            Inference
        </Typography>
      </Cell>
      <Cell>
        <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
          <Switch
            checked={mode === SESSION_MODE_FINETUNE}
            onChange={(event: any) => onSetMode(event.target.checked ? SESSION_MODE_FINETUNE : SESSION_MODE_INFERENCE)}
            name="modeSwitch"
            size="medium"
            sx={{
              transform: 'scale(1.6)',
              '& .MuiSwitch-thumb': {
              scale: 0.4,
              },
            }}
          />
        </Box>
      </Cell>
      <Cell>
        <Typography
          sx={{
            color: mode === SESSION_MODE_FINETUNE ? 'text.primary' : 'text.secondary',
            fontWeight: mode === SESSION_MODE_FINETUNE ? 'bold' : 'normal',
            marginLeft: 2,
            textAlign: 'left',
          }}
        >
          Fine-tuning
        </Typography>
      </Cell>
    </Row>
  ) : (
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
        &nbsp;&nbsp;{ mode === SESSION_MODE_FINETUNE ? 'Fine-tune' : 'Inference' } <KeyboardArrowDownIcon sx={{position:"relative", top:"5px"}}/>&nbsp;
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
        <MenuItem
          selected={ mode === SESSION_MODE_INFERENCE }
          onClick={() => {
            onSetMode(SESSION_MODE_INFERENCE)
            setModeMenuAnchorEl(null)
          }}
        >
          Inference
        </MenuItem>
        <MenuItem
          key={ SESSION_MODE_FINETUNE }
          selected={ mode === SESSION_MODE_FINETUNE }
          onClick={() => {
            onSetMode(SESSION_MODE_FINETUNE)
            setModeMenuAnchorEl(null)
          }}
        >
          Fine-tune
        </MenuItem>
      </Menu>
    </>
  )
}

export default SessionModeSwitch
