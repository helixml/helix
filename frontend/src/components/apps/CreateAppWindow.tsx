import React, { FC, useState, useMemo } from 'react'
import TextField from '@mui/material/TextField'
import CircularProgress from '@mui/material/CircularProgress'
import FormControl from '@mui/material/FormControl'
import Grid from '@mui/material/Grid'
import Alert from '@mui/material/Alert'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import IconButton from '@mui/material/IconButton'
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline'
import MenuItem from '@mui/material/MenuItem'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import GitHubIcon from '@mui/icons-material/GitHub'

import Window from '../widgets/Window'

import {
  IGithubStatus,
  IGithubRepo,
} from '../../types'

export const CreateAppWindow: FC<{  
  githubStatus: IGithubStatus,
  githubRepos: string[],
  githubReposLoading: boolean,
  connectLoading: boolean,
  connectError?: string,
  onCancel: () => void,
  onLoadRepos: () => void,
  onConnectRepo: (repo: string) => Promise<boolean>,
}> = ({
  githubStatus,
  githubRepos,
  githubReposLoading,
  connectError = '',
  connectLoading,
  onCancel,
  onLoadRepos,
  onConnectRepo,
}) => {
  const [ filterOrg, setFilterOrg ] = useState('any')
  const [ filterText, setFilterText ] = useState('')
  const [ activeRepo, setActiveRepo ] = useState('binocarlos/demo-recipes')

  const groups = useMemo(() => {
    const foundGroups: Record<string, boolean> = {}
    githubRepos.forEach(repo => {
      const group = repo.split('/')[0]
      foundGroups[group] = true
    })
    return Object.keys(foundGroups)
  }, [
    githubRepos,
  ])

  const useRepos = useMemo(() => {
    const seenRepos: Record<string, boolean> = {}
    return githubRepos
      .filter(repo => {
        if (filterOrg !== 'any' && !repo.startsWith(filterOrg)) {
          return false
        }
        if (filterText && !repo.includes(filterText)) {
          return false
        }
        return true
      })
      .reduce<string[]>((all, repo) => {
        if(seenRepos[repo]) return all
        seenRepos[repo] = true
        return all.concat([repo])
      }, [])
  }, [
    githubRepos,
    filterOrg,
    filterText,
  ])


  return (
    <Window
      title="New Github App"
      size="md"
      fullHeight
      open
      withCancel
      cancelTitle="Cancel"
      onCancel={ onCancel }
    >
      <Box
        sx={{
          p: 2,
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
        }}
      >
        <Box
          sx={{
            flexGrow: 0,
          }}
        >
          <Typography className="interactionMessage"
            sx={{
              mt: 2,
              mb: 2,
              textAlign: 'left',
            }}
          >
            Create a new app by linking a github repo with a helix.yaml file to configure the app.
          </Typography>
        </Box>

        {
          connectError && (
            <Box
              sx={{
                flexGrow: 0,
              }}
            >
              <Alert severity="error">{ connectError }</Alert>
            </Box>
          )
        }

        {
          githubStatus.has_token && (
            <>
              {
                githubReposLoading ? (
                  <Box
                    sx={{
                      p: 2,
                    }}
                  >
                    <Typography
                      variant="body1"
                      sx={{
                        mt: 2,
                        mb: 2,
                      }}
                    >
                      loading github repos...
                    </Typography>
                    <CircularProgress />
                  </Box>
                ) : 
                activeRepo ? (
                  <Box
                    sx={{
                      p: 2,
                    }}
                  >
                    {
                      connectLoading ? (
                        <Box
                          sx={{
                            p: 2,
                          }}
                        >
                          <Typography
                            variant="body1"
                            sx={{
                              mt: 2,
                              mb: 2,
                            }}
                          >
                            connecting github repo: { activeRepo }
                          </Typography>
                          <CircularProgress />
                        </Box>
                      ) : (
                        <Grid container spacing={ 2 }>
                          <Grid item xs={ 12 }>
                            <Typography gutterBottom variant="h5">
                              { activeRepo }
                            </Typography>
                          </Grid>
                          <Grid item xs={ 12 }>
                            <Typography gutterBottom variant="body1" component="div">
                              If you click "Connect Repo" - we will:
                              <ul>
                                <li>Generate a key pair and upload the public key to the repo</li>
                                <li>Create a new web-hook on the repo</li>
                              </ul>
                            </Typography>
                            <Typography gutterBottom variant="body1">
                              This means we will be able to read the contents of the repo and will be told when code changes.
                            </Typography>
                          </Grid>
                          <Grid item xs={ 12 }>
                            <Button
                              sx={{mr: 1}}
                              color="secondary"
                              variant="outlined"
                              size="small"
                              onClick={ () => {
                                setActiveRepo('')
                              }}
                            >
                              Change Repo
                            </Button>
                            &nbsp;
                            <Button
                              color="secondary"
                              variant="contained"
                              size="small"
                              disabled={ connectLoading }
                              onClick={ async () => onConnectRepo(activeRepo) }
                            >
                              Connect Repo
                            </Button>
                          </Grid>
                        </Grid>
                      )
                    }
                    
                  </Box>
                ) : (
                  <>
                    <Box
                      sx={{
                        flexGrow: 0,
                      }}
                    >
                      <Grid container spacing={ 2 }>
                        <Grid item xs={ 12 }>
                          <Typography gutterBottom>
                            Select a github repository for your project - if you do not have one, then please <Box component="a" href="https://github.com/new" target="_blank" sx={{color:'#fff'}}>create a new one</Box> and then return here...
                          </Typography>
                        </Grid>
                        <Grid item xs={ 6 }>
                          <Typography gutterBottom>
                            { useRepos.length } repos found!
                          </Typography>
                        </Grid>
                        <Grid item xs={ 6 }>
                          <Button
                            color="secondary"
                            variant="outlined"
                            size="small"
                            onClick={ () => {
                              onLoadRepos()
                            }}
                          >
                            Reload
                          </Button>
                        </Grid>
                        <Grid item xs={ 6 }>
                          <FormControl
                            sx={{
                              width: '100%'
                            }}
                          >
                            <InputLabel>Org</InputLabel>
                            <Select
                              sx={{
                                width: '100%'
                              }}
                              value={ filterOrg }
                              onChange={ (ev) => setFilterOrg(ev.target.value) }
                            >
                              <MenuItem value={'any'}>Any</MenuItem>
                              {
                                groups.map(group => (
                                  <MenuItem key={ group } value={ group }>{ group }</MenuItem>
                                ))
                              }
                            </Select>
                          </FormControl>
                        </Grid>
                        <Grid item xs={ 6 }>
                          <TextField
                            label="Filter"
                            helperText={`Find repos containing the text`}
                            value={ filterText }
                            fullWidth
                            onChange={ (e) => setFilterText(e.target.value) }
                          />
                        </Grid>
                      </Grid>
                    </Box>
                    <Box
                      sx={{
                        flexGrow: 1,
                        overflowY: 'auto',
                      }}
                    >
                      <List dense>
                        {
                          useRepos.map((repo, index) => {
                            return (
                              <ListItem key={index} button onClick={ () => {
                                setActiveRepo(repo)
                              }}>
                                <ListItemText primary={ repo } />
                                <ListItemSecondaryAction>
                                  <IconButton color="primary" component="span" onClick={ () => {
                                    setActiveRepo(repo)
                                  }}>
                                    <PlayCircleOutlineIcon />
                                  </IconButton>
                                </ListItemSecondaryAction>
                              </ListItem>
                            )
                          })
                        }
                      </List>
                    </Box>
                  </>
                )
              }
            </>
          )
        }
        {
          githubStatus.redirect_url && (
            <>
              <Button
                variant="contained"
                color="secondary"
                endIcon={<GitHubIcon />}
                onClick={ () => {
                  document.location = githubStatus.redirect_url
                }}
              >
                Connect Github Account
              </Button>
            </>
          ) 
        }

        {/* <TextField
          sx={{
            mb: 2,
          }}
          error={ showErrors && !url }
          value={ url }
          onChange={(e) => setURL(e.target.value)}
          fullWidth
          label="Endpoint URL"
          placeholder="https://api.example.com/v1/"
          helperText={ showErrors && !url ? "Please enter a URL" : "URL should be in the format: https://api.example.com/v1/endpoint" }
        />
        <TextField
          error={ showErrors && !schema }
          value={ schema }
          onChange={(e) => setSchema(e.target.value)}
          fullWidth
          multiline
          rows={10}
          label="OpenAPI (Swagger) schema"
          helperText={ showErrors && !schema ? "Please enter a schema" : "" }
        /> */}
      </Box>
    </Window>
  )  
}

export default CreateAppWindow