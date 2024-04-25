import React, { FC, useState, useCallback, useEffect } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'

import FileUpload from '../widgets/FileUpload'
import InteractionContainer from './InteractionContainer'
import CropOriginalIcon from '@mui/icons-material/CropOriginal';

import {
  buttonStates,
} from '../../types'
import { Drawer, List, ListItem, IconButton, ListItemIcon, ListItemText } from '@mui/material'

interface FineTuneImageInputsProps {
  initialFiles?: File[];
  showButton?: boolean;
  onChange?: (files: File[]) => void;
  onDone?: () => void;
}

const FineTuneImageInputs: FC<FineTuneImageInputsProps> = ({
  initialFiles = [],
  showButton = false,
  onChange,
  onDone,
}) => {
  const [files, setFiles] = useState<File[]>(initialFiles);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const theme = useTheme();
  const themeConfig = useThemeConfig();

  const handleDownloadFile = (file: File) => {
    // Implement file download logic here
    // For example, create a URL and trigger the browser to download
    const url = window.URL.createObjectURL(file);
    const link = document.createElement('a');
    link.href = url;
    link.download = file.name;
    link.click();
    window.URL.revokeObjectURL(url);
  };

  

  const handleRemoveFile = (index: number) => {
    const newFiles = [...files];
    newFiles.splice(index, 1);
    setFiles(newFiles);
    if (onChange) {
      onChange(newFiles);
    }
  };

  const onDropFiles = useCallback(async (newFiles: File[]) => {
    const existingFiles = files.reduce<Record<string, boolean>>((all, file) => {
      all[file.name] = true;
      return all;
    }, {});
    const filteredNewFiles = newFiles.filter(f => !existingFiles[f.name]);
    const updatedFiles = files.concat(filteredNewFiles);
    setFiles(updatedFiles);
    if (onChange) {
      onChange(updatedFiles);
    }
  }, [files, onChange]);
  return (
    <Box
    sx={{
      mt: 0,
    }}
  >

{
    <Typography className="interactionMessage">
    Upload some images you want your model to learn from
  </Typography>
}
    {/* {
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
          
          </InteractionContainer>
        </Box>
      )
    } */}
      <FileUpload
        sx={{
          width: '100%',
          mt: 4,
        }}
        onlyImages
        onUpload={ onDropFiles }
      >
        <Box
          sx={{
            border: '1px solid #333',
            p: 2,
            height: '250px',
            minHeight: '100px',
            cursor: 'pointer',
            backgroundColor: `${theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor}80`,
            width: '100%',
            display: 'flex', 
            flexDirection: 'column', 
            alignItems: 'center', 
            justifyContent: 'center',
          }}
        >
          {
            files.length <= 0 && (
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                  justifyContent: 'center',
                  marginBottom: '40px',
                }}
              >
                <Typography
                  sx={{
                    color: '#bbb',
                    textAlign: 'center',
                  }}
                  variant="caption"
                 >
                  Drag images here to upload
                </Typography>
                <Typography
                  sx={{
                    color: '#bbb',
                    textDecoration: 'underline',
                    cursor: 'pointer',
                    ml: 2,
                  }}
                >
                  upload manually
                </Typography>
              </Box>
            )
          }

          <Grid container spacing={3} direction="row" justifyContent="flex-start">
            {
              files.length > 0 && files.map((file) => {
                const objectURL = URL.createObjectURL(file)
                return (
                  <Grid item xs={4} md={4} key={Image.name}>
                    <Box
                      sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: '#999'
                      }}
                    >
                      <Box
                        component="img"
                        src={objectURL}
                        alt={file.name}
                        sx={{
                          height: '50px',
                          border: '1px solid #000000',
                          filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                          mb: 1,
                        }}
                      />
                      <Typography variant="caption">
                        {file.name}
                      </Typography>
                      <Typography variant="caption">
                        ({file.size} bytes)
                      </Typography>
                    </Box>
                  </Grid>
                )
              })
                
            }
          </Grid>
        </Box>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '30vh', justifyContent: 'space-between' }}></Box>
      </FileUpload>
      {files.length > 0 && (
        <Grid container spacing={3} direction="row" justifyContent="space-between" alignItems="center" sx={{ mt: 2, mb: 2 }}>
          <Grid item xs={6}>
            <Typography sx={{ display: 'inline-flex', alignItems: 'center' }}>
              {files.length} image{files.length !== 1 ? 's' : ''} added.
              <Button
                component="button"
                onClick={() => setDrawerOpen(true)}
                sx={{ ml: -0.5, textDecoration: 'underline', color:'#3BF959' }}
              >
                View or edit images
              </Button>
            </Typography>
          </Grid>
          <Grid item xs={6} style={{ textAlign: 'right' }} >
            {showButton && onDone && (
              <Button
                variant="contained"
                sx={{
                  backgroundColor: '#3BF959', 
                  color: 'black', 
                  '&:hover': {
                    backgroundColor: '#33a24f', 
                  },
                }}
                onClick={onDone}
              >
                Continue
              </Button>
            )}
          </Grid>
        </Grid>
      )}
      <Drawer
        anchor="right"
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
      >
        <Box
          sx={{
            width: 506,
            maxWidth: '100%',
          }}
          role="presentation"
        >
          <List>
            {files.map((file, index) => (
              <React.Fragment key={file.name}>
                <ListItem
                  secondaryAction={
                    <Box sx={{ display: 'flex' }}>
                      <IconButton edge="end" aria-label="download" onClick={() => handleDownloadFile(file)}>
                        {/* <FileDownloadIcon /> */}
                      </IconButton>
                      <IconButton edge="end" aria-label="delete" onClick={() => handleRemoveFile(index)}>
                        {/* <DeleteIcon /> */}
                      </IconButton>
                    </Box>
                  }
                >
                  <ListItemIcon>
                    {/* <ImageIcon /> */}
                  </ListItemIcon>
                  <ListItemText
                    primary={file.name}
                    sx={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
                  />
                </ListItem>
              </React.Fragment>
                ))}
                </List>
              </Box>
            </Drawer>
      
        
    </Box>
  )   
}

export default FineTuneImageInputs