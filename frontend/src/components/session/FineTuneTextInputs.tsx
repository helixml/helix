import React, { FC, useState, useCallback, useEffect } from 'react'
import { prettyBytes } from '../../utils/format'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import Grid from '@mui/material/Grid'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import useThemeConfig from '../../hooks/useThemeConfig'
import useTheme from '@mui/material/styles/useTheme'

import AddCircleIcon from '@mui/icons-material/AddCircle'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'

import FileUpload from '../widgets/FileUpload'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import Caption from '../widgets/Caption'
import Window from '../widgets/Window'

import showdown from 'showdown'
import showdownHighlight from 'showdown-highlight'

import useSnackbar from '../../hooks/useSnackbar'
import InteractionContainer from './InteractionContainer'

import {
  buttonStates,
} from '../../types'

import {
  mapFileExtension,
} from '../../utils/filestore'

let converter = new showdown.Converter({
  extensions: [showdownHighlight({
    pre: true, 
    auto_detection: true
  })]
})

const MARKDOWN_HELP = `

### alpaca

instruction; input(optional)
\`\`\`json
{"instruction": "...", "input": "...", "output": "..."}
\`\`\`

### sharegpt

conversations where \`from\` is \`human\`/\`gpt\`. (optional: \`system\` to override default system prompt)
\`\`\`json
{"conversations": [{"from": "...", "value": "..."}]}
\`\`\`

### llama-2

the json is the same format as \`sharegpt\` above, with the following config
\`\`\`yaml
datasets:
  - path: <your-path>
    type: sharegpt
    conversation: llama-2
\`\`\`

### completion

raw corpus
\`\`\`json
{"text": "..."}
\`\`\`


### jeopardy

question and answer
\`\`\`json
{"question": "...", "category": "...", "answer": "..."}
\`\`\`

### oasst

instruction
\`\`\`json
{"INSTRUCTION": "...", "RESPONSE": "..."}
\`\`\`

### gpteacher

instruction; input(optional)
\`\`\`json
{"instruction": "...", "input": "...", "response": "..."}
\`\`\`

### reflection

instruction with reflect; input(optional)
\`\`\`json
{"instruction": "...", "input": "...", "output": "...", "reflection": "...", "corrected": "..."}
\`\`\`

### explainchoice

question, choices, (solution OR explanation)
\`\`\`json
{"question": "...", "choices": ["..."], "solution": "...", "explanation": "..."}
\`\`\`

### concisechoice

question, choices, (solution OR explanation)
\`\`\`json
{"question": "...", "choices": ["..."], "solution": "...", "explanation": "..."}
\`\`\`

### summarizetldr

article and summary
\`\`\`json
{"article": "...", "summary": "..."}
\`\`\`

### alpaca_chat

basic instruct for alpaca chat
\`\`\`json
{"instruction": "...", "input": "...", "response": "..."}
\`\`\`

### alpaca_chat.load_qa

question and answer for alpaca chat
\`\`\`json
{"question": "...", "answer": "..."}
\`\`\`

### alpaca_chat.load_concise

question and answer for alpaca chat, for concise answers
\`\`\`json
{"instruction": "...", "input": "...", "response": "..."}
\`\`\`

### alpaca_chat.load_camel_ai

question and answer for alpaca chat, for load_camel_ai
\`\`\`json
{"message_1": "...", "message_2": "..."}
\`\`\`

### alpaca_w_system.load_open_orca

support for open orca datasets with included system prompts, instruct
\`\`\`json
{"system_prompt": "...", "question": "...", "response": "..."}
\`\`\`

### context_qa

in context question answering from an article
\`\`\`json
{"article": "...", "question": "...", "answer": "..."}
\`\`\`

### context_qa.load_v2

in context question answering (alternate)
\`\`\`json
{"context": "...", "question": "...", "answer": "..."}
\`\`\`

### context_qa.load_404

in context question answering from an article, with default response for no answer from context
\`\`\`json
{"article": "...", "unanswerable_question": "..."}
\`\`\`

### creative_acr.load_answer

instruction and revision
\`\`\`json
{"instruction": "...", "revision": "..."}
\`\`\`

### creative_acr.load_critique

critique
\`\`\`json
{"scores": "...", "critiques": "...", "instruction": "...", "answer": "..."}
\`\`\`

### creative_acr.load_revise

critique and revise
\`\`\`json
{"scores": "...", "critiques": "...", "instruction": "...", "answer": "...", "revision": "..."}
\`\`\`

### pygmalion

pygmalion
\`\`\`json
{"conversations": [{"role": "...", "value": "..."}]}
\`\`\`

### metharme

instruction, adds additional eos tokens
\`\`\`json
{"prompt": "...", "generation": "..."}
\`\`\`

### sharegpt.load_role

conversations where \`role\` is used instead of \`from\`
\`\`\`json
{"conversations": [{"role": "...", "value": "..."}]}
\`\`\`

### sharegpt.load_guanaco

conversations where \`from\` is \`prompter\`/\`assistant\` instead of default sharegpt
\`\`\`json
{"conversations": [{"from": "...", "value": "..."}]}
\`\`\`

### sharegpt_jokes

creates a chat where bot is asked to tell a joke, then explain why the joke is funny
\`\`\`json
{"conversations": [{"title": "...", "text": "...", "explanation": "..."}]}
\`\`\`
`

export const FineTuneTextInputs: FC<{
  initialCounter?: number,
  initialFiles?: File[],
  showButton?: boolean,
  showSystemInteraction?: boolean,
  onChange?: {
    (counter: number, files: File[]): void
  },
  onDone?: {
    (manuallyReviewQuestions?: boolean): void
  },
}> = ({
  initialCounter,
  initialFiles,
  showButton = false,
  showSystemInteraction = true,
  onChange,
  onDone,
}) => {
  const snackbar = useSnackbar()

  const [manualTextFileCounter, setManualTextFileCounter] = useState(initialCounter || 0)
  const [manualTextFile, setManualTextFile] = useState('')
  const [manualURL, setManualURL] = useState('')
  const [addingQAPair, setAddingQAPair] = useState(false)
  const [qaPairType, setQAPairType] = useState('sharegpt')
  const [manuallyReviewQuestions, setManuallyReviewQuestions] = useState(false)
  const [files, setFiles] = useState<File[]>(initialFiles || [])
  const themeConfig = useThemeConfig()
  const theme = useTheme()

  const onAddURL = useCallback(() => {
    if(!manualURL.match(/^https?:\/\//i)) {
      snackbar.error(`Please enter a valid URL`)
      return
    }
    let useUrl = manualURL.replace(/\/$/i, '')
    useUrl = decodeURIComponent(useUrl)
    let fileTitle = useUrl
      .replace(/^https?:\/\//i, '')
      .replace(/^www\./i, '')
    const file = new File([
      new Blob([manualURL], { type: 'text/html' })
    ], `${fileTitle}.url`)
    setFiles(files.concat(file))
    setManualURL('')
  }, [
    manualURL,
    files,
  ])

  const onAddQAPairs = useCallback(() => {
    
  }, [
    manualURL,
    files,
  ])

  const onAddTextFile = useCallback(() => {
    const newCounter = manualTextFileCounter + 1
    setManualTextFileCounter(newCounter)
    const file = new File([
      new Blob([manualTextFile], { type: 'text/plain' })
    ], `textfile-${newCounter}.txt`)
    setFiles(files.concat(file))
    setManualTextFile('')
  }, [
    manualTextFile,
    manualTextFileCounter,
    files,
  ])

  const onDropFiles = useCallback(async (newFiles: File[]) => {
    const existingFiles = files.reduce<Record<string, string>>((all, file) => {
      all[file.name] = file.name
      return all
    }, {})
    const filteredNewFiles = newFiles.filter(f => !existingFiles[f.name])
    setFiles(files.concat(filteredNewFiles))
  }, [
    files,
  ])

  useEffect(() => {
    if(!onChange) return
    onChange(manualTextFileCounter, files)
  }, [
    manualTextFileCounter,
    files,
  ])

  return (
    <Box
      sx={{
        mt: 2,
        width: '100%',
      }}
    >
      {
        showSystemInteraction && (
          <Box
            sx={{
              mt: 4,
              mb: 4,
            }}
          >
            <InteractionContainer
              name="System"
            >
              <Typography className="interactionMessage">
                Add URLs, paste some text or upload some files you want your model to learn from:
              </Typography>
            </InteractionContainer>
          </Box>
        )
      }
      <Row
        sx={{
          width: '100%',
          display: 'flex',
          mb: 2,
          alignItems: 'flex-start',
          justifyContent: 'flex-start',
          flexDirection: {
            xs: 'column',
            sm: 'column',
            md: 'row'
          }
        }}
      >
        <Cell
          sx={{
            width: '100%',
            flexGrow: 1,
            pr: 2,
            pb: 1,
          }}
        >
          <TextField
            fullWidth
            label="Add link, for example https://google.com"
            value={ manualURL }
            onChange={ (e) => {
              setManualURL(e.target.value)
            }}
            sx={{
              pb: 1,
              backgroundColor: `${theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor}80`,
            }}
          />
        </Cell>
        <Cell
          sx={{
            width: '240px',
            minWidth: '240px',
          }}
        >
          <Button
            sx={{
              width: '100%',
            }}
            variant="contained"
            color={ buttonStates.addUrlColor }
            endIcon={<AddCircleIcon />}
            onClick={ onAddURL }
          >
            { buttonStates.addUrlLabel }
          </Button>
        </Cell>
      </Row>
      <Row
        sx={{
          mb: 2,
          alignItems: 'flex-start',
          flexDirection: {
            xs: 'column',
            sm: 'column',
            md: 'row'
          }
        }}
      >
        <Cell
          sx={{
            width: '100%',
            pb: 1,
            flexGrow: 1,
            pr: 2,
            alignItems: 'flex-start',
          }}
        >
          <TextField
            sx={{
              height: '100px',
              maxHeight: '100px',
              pb: 1,
              backgroundColor: `${theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor}80`,
            }}
            fullWidth
            label="or paste some text here"
            value={ manualTextFile }
            multiline
            rows={ 3 }
            onChange={ (e) => {
              setManualTextFile(e.target.value)
            }}
          />
        </Cell>
        <Cell
          sx={{
            flexGrow: 0,
            width: '240px',
            minWidth: '240px',
          }}
        >
          <Button
            sx={{
              width: '100%',
            }}
            variant="contained"
            color={ buttonStates.addTextColor }
            endIcon={<AddCircleIcon />}
            onClick={ onAddTextFile }
          >
            { buttonStates.addTextLabel }
          </Button>
        </Cell>        
      </Row>

      <FileUpload
        sx={{
          width: '100%',
        }}
        onlyDocuments
        onUpload={ onDropFiles }
      >
        <Row
          sx={{
            alignItems: 'flex-start',
            flexDirection: {
              xs: 'column',
              sm: 'column',
              md: 'row'
            }
          }}
        >
          <Cell
            sx={{
              width: '100%',
              flexGrow: 1,
              pr: 2,
              pb: 1,
            }}
          >
            <Box
              sx={{
                border: '1px solid #333333',
                borderRadius: '4px',
                p: 2,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'flex-start',
                height: '120px',
                minHeight: '120px',
                cursor: 'pointer',
                backgroundColor: `${theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor}80`,
              }}
            >
              
              <Typography
                sx={{
                  color: '#bbb',
                  width: '100%',
                }}
              >
                drop documents here to upload them ...
              </Typography>
              
            </Box>
          </Cell>
          <Cell
            sx={{
              flexGrow: 0,
              width: '240px',
              minWidth: '240px',
            }}
          >
            <Button
              sx={{
                width: '100%',
              }}
              variant="contained"
              color={ buttonStates.uploadFilesColor }
              endIcon={<CloudUploadIcon />}
            >
              { buttonStates.uploadFilesLabel }
            </Button>
          </Cell>
        </Row>
      </FileUpload>

      <Row
        sx={{
          width: '100%',
          display: 'flex',
          mb: 2,
          alignItems: 'flex-start',
          justifyContent: 'flex-start',
          flexDirection: {
            xs: 'column',
            sm: 'column',
            md: 'row'
          }
        }}
      >
        <Cell
          sx={{
            width: '100%',
            flexGrow: 1,
            pr: 2,
            pb: 1,
          }}
        >
          
        </Cell>
        <Cell
          sx={{
            width: '240px',
            minWidth: '240px',
          }}
        >
          <Button
            sx={{
              width: '100%',
            }}
            variant="contained"
            color={ buttonStates.addQAPairsColor }
            endIcon={<AddCircleIcon />}
            onClick={ () => setAddingQAPair(true) }
          >
            { buttonStates.addQAPairsLabel }
          </Button>
        </Cell>
      </Row>

      <Box
        sx={{
          mt: 2,
          mb: 2,
        }}
      >
        <Grid container spacing={3} direction="row" justifyContent="flex-start">
          {
            files.length > 0 && files.map((file) => {
              return (
                <Grid item xs={12} md={2} key={file.name}>
                  <Box
                    sx={{
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      justifyContent: 'center',
                      color: '#999'
                    }}
                  >
                    <span className={`fiv-viv fiv-size-md fiv-icon-${mapFileExtension(file.name)}`}></span>
                    <Caption sx={{ maxWidth: '100%'}}>
                      {file.name}
                    </Caption>
                    <Caption>
                      ({prettyBytes(file.size)})
                    </Caption>
                  </Box>
                </Grid>
              )
            })
          }
        </Grid>
      </Box>
      {
        files.length > 0 && showButton && onDone && (
          <Grid container spacing={3} direction="row" justifyContent="flex-start">
            <Grid item xs={ 12 }>
              <FormGroup>
                <FormControlLabel control={
                  <Checkbox
                    checked={manuallyReviewQuestions}
                    onChange={(event) => {
                      setManuallyReviewQuestions(event.target.checked)
                    }}
                  />
                } label="Manually review training data before fine-tuning?" />
              </FormGroup>
            </Grid>
            <Grid item xs={ 12 }>
              <Button
                sx={{
                  width: '100%',
                }}
                variant="contained"
                color="secondary"
                endIcon={<ArrowCircleRightIcon />}
                onClick={ () => onDone(manuallyReviewQuestions) }
              >
                Next Step
              </Button>
            </Grid>
          </Grid>
        )
      }
      {
        addingQAPair && (
          <Window
            size="lg"
            open
            title="Add manual Q&A pairs"
            onCancel={ () => setAddingQAPair(false) }
            onSubmit={ () => {
              setAddingQAPair(false)
            }}
          >
            <Box
              sx={{
                p: 2,
              }}
            >
              <Row>
                <Cell>
                  <Typography variant="body1">
                    Choose the format your questions and answers file is in.
                  </Typography>
                </Cell>
              </Row>
              <Row
                sx={{
                  alignItems: 'flex-start',
                  flexDirection: {
                    xs: 'column',
                    sm: 'column',
                    md: 'row'
                  }
                }}
              >
                <Cell
                  sx={{
                    width: '100%',
                    flexGrow: 1,
                    pr: 2,
                    pb: 1,
                  }}
                >
                  <Select
                    sx={{
                      color: 'white',
                    }}
                    value={qaPairType}
                    onChange={(e) => setQAPairType(e.target.value as string)}
                    fullWidth
                  >
                    <MenuItem value="alpaca">alpaca</MenuItem>
                    <MenuItem value="sharegpt">sharegpt</MenuItem>
                    <MenuItem value="llama-2">llama-2</MenuItem>
                    <MenuItem value="jeopardy">jeopardy</MenuItem>
                    <MenuItem value="oasst">oasst</MenuItem>
                    <MenuItem value="gpteacher">gpteacher</MenuItem>
                    <MenuItem value="reflection">reflection</MenuItem>
                    <MenuItem value="explainchoice">explainchoice</MenuItem>
                    <MenuItem value="concisechoice">concisechoice</MenuItem>
                    <MenuItem value="summarizetldr">summarizetldr</MenuItem>
                    <MenuItem value="alpaca_chat">alpaca_chat</MenuItem>
                    <MenuItem value="alpaca_chat.load_qa">alpaca_chat.load_qa</MenuItem>
                    <MenuItem value="alpaca_chat.load_concise">alpaca_chat.load_concise</MenuItem>
                    <MenuItem value="alpaca_chat.load_camel_ai">alpaca_chat.load_camel_ai</MenuItem>
                    <MenuItem value="alpaca_w_system.load_open_orca">alpaca_w_system.load_open_orca</MenuItem>
                    <MenuItem value="context_qa">context_qa</MenuItem>
                    <MenuItem value="context_qa.load_v2">context_qa.load_v2</MenuItem>
                    <MenuItem value="context_qa.load_404">context_qa.load_404</MenuItem>
                    <MenuItem value="creative_acr.load_answer">creative_acr.load_answer</MenuItem>
                    <MenuItem value="creative_acr.load_critique">creative_acr.load_critique</MenuItem>
                    <MenuItem value="creative_acr.load_revise">creative_acr.load_revise</MenuItem>
                    <MenuItem value="pygmalion">pygmalion</MenuItem>
                    <MenuItem value="metharme">metharme</MenuItem>
                    <MenuItem value="sharegpt.load_role">sharegpt.load_role</MenuItem>
                    <MenuItem value="sharegpt.load_guanaco">sharegpt.load_guanaco</MenuItem>
                    <MenuItem value="sharegpt_jokes">sharegpt_jokes</MenuItem>
                  </Select>
                </Cell>
                <Cell
                  sx={{
                    flexGrow: 0,
                    width: '240px',
                    minWidth: '240px',
                  }}
                >
                  <FileUpload
                    sx={{
                      width: '100%',
                    }}
                    onlyDocuments
                    onUpload={ onDropFiles }
                  >
                    <Button
                      sx={{
                        width: '100%',
                      }}
                      variant="contained"
                      color={ buttonStates.uploadFilesColor }
                      endIcon={<CloudUploadIcon />}
                    >
                      Choose JSONL File
                    </Button>
                  </FileUpload>
                </Cell>
              </Row>

              <Row>
                <Cell>
                  <div dangerouslySetInnerHTML={{__html: converter.makeHtml(MARKDOWN_HELP) }} />
                </Cell>
              </Row>
            </Box>
          </Window>
        )
      }
    </Box>
  )   
}

export default FineTuneTextInputs