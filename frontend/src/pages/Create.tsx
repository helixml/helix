import React, { FC, useState, useCallback, useEffect, useRef, useMemo } from 'react'
import { styled, useTheme } from '@mui/material/styles'
import useMediaQuery from '@mui/material/useMediaQuery'
import bluebird from 'bluebird'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Divider from '@mui/material/Divider'
import Checkbox from '@mui/material/Checkbox'
import InputLabel from '@mui/material/InputLabel'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import FormControl from '@mui/material/FormControl'
import Switch from '@mui/material/Switch'
import SendIcon from '@mui/icons-material/Send'
import SwapHorizIcon from '@mui/icons-material/SwapHoriz'
import SettingsIcon from '@mui/icons-material/Settings'
import ConstructionIcon from '@mui/icons-material/Construction'
import InputAdornment from '@mui/material/InputAdornment'
import useThemeConfig from '../hooks/useThemeConfig'
import IconButton from '@mui/material/IconButton'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import { SelectChangeEvent } from '@mui/material/Select'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'

import BackgroundImageWrapper from '../components/widgets/BackgroundImageWrapper'
import FineTuneTextInputs from '../components/session/FineTuneTextInputs'
import FineTuneImageInputs from '../components/session/FineTuneImageInputs'
import FineTuneImageLabels from '../components/session/FineTuneImageLabels'
import Window from '../components/widgets/Window'
import Disclaimer from '../components/widgets/Disclaimer'
import UploadingOverlay from '../components/widgets/UploadingOverlay'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useTools from '../hooks/useTools'
import useLayout from '../hooks/useLayout'
import useSessions from '../hooks/useSessions'
import useFinetuneInputs from '../hooks/useFinetuneInputs'
import ButtonGroup from '@mui/material/ButtonGroup'
import useSessionConfig from '../hooks/useSessionConfig'
import useTracking from '../hooks/useTracking'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  BUTTON_STATES,
} from '../types'

const Create: FC = () => {
  return (
    <BackgroundImageWrapper>
      <Box sx={{m:20}}>hello4</Box>
    </BackgroundImageWrapper>
  )
}

export default Create
