import React, { FC, useState } from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardMedia from '@mui/material/CardMedia';
import IconButton from '@mui/material/IconButton';
import ZoomOutMapIcon from '@mui/icons-material/ZoomOutMap';
import TextField from '@mui/material/TextField';
import RefreshIcon from '@mui/icons-material/Refresh';
import CloseIcon from '@mui/icons-material/Close';
import { useTheme } from '@mui/material/styles';


  interface UploadedImage {
    id: string,
    url: string,
    description: string,
  }
  
  // Define the IMAGE_DATA array
  const IMAGE_DATA: UploadedImage[] = [{
    id: '1',
    url: 'https://www.pixelstalk.net/wp-content/uploads/2016/07/Free-Amazing-Background-Images-Nature.jpg',
    description: 'test image 1',
  }, {
    id: '2',
    url: 'https://www.pixelstalk.net/wp-content/uploads/2016/08/Best-Nature-Full-HD-Images-For-Desktop.jpg',
    description: 'test image 2',
  }, {
    id: '3',
    url: 'https://www.pixelstalk.net/wp-content/uploads/2016/07/Free-Amazing-Background-Images-Nature.jpg',
    description: 'test image 3',
  }, {
    id: '4',
    url: 'https://www.pixelstalk.net/wp-content/uploads/2016/08/Best-Nature-Full-HD-Images-For-Desktop.jpg',
    description: 'test image 4',
  },
];
  const ImageFineTuneMoreView: FC = () => {
  const [ zoomedImage, setZoomedImage ] = useState<string>('');
  const theme = useTheme();

  
  return (
    <Container
      maxWidth="xl"
      sx={{
        mt: 12,
        height: 'calc(100% - 100px)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundImage: theme.palette.mode === 'light' ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
        backgroundSize: '80%',
        backgroundPosition: 'center 130%',
        backgroundRepeat: 'no-repeat',
        zIndex: -1, // Ensure the background is behind all other content
        
       
        
      }}
      >  
 <Grid container spacing={2}>
        {IMAGE_DATA.map((image, index) => (
          <Grid item xs={12} sm={6} md={4} lg={3} key={image.id}>
            <Card sx={{ position: 'relative', width: '100%', height: 250, backgroundColor: 'transparent', boxShadow: 'none', mt: 2 }}>
              <img
                src={image.url}
                alt={image.description}
                style={{
                  width: '100%',
                  height: '100%',
                  objectFit: 'cover',
                }}
              />
              <IconButton
                sx={{
                  position: 'absolute',
                  bottom: 8,
                  right: 8,
                  backgroundColor: 'transparent',
                }}
                aria-label="zoom out"
                onClick={() => setZoomedImage(image.url)}
              >
                <ZoomOutMapIcon />
              </IconButton>
            </Card>
            <TextField
              sx={{ width: '100%', mt: 2 }}
              placeholder={image.description}
              multiline
              rows={2}
            />
          </Grid>
        ))}
      </Grid>

      {
          zoomedImage && (
            <Box
              sx={{
                position: 'fixed',
                top: 0,
                left: 0,
                width: '100%',
                height: '100%',
                backgroundColor: 'rgba(0, 0, 0, 0.8)',
                zIndex: 1300,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
              onClick={() => setZoomedImage('')} // Close the zoomed image when the background is clicked
             >
              <img 
                src={zoomedImage} 
                alt="Zoomed" 
                onClick={(e) => e.stopPropagation()} // Prevent click from closing the image
                style={{ maxWidth: '70%', maxHeight: '70%' }} 
              />
              <IconButton
                sx={{
                  position: 'fixed',
                  top: 8,
                  right: 8,
                  color: 'white',
                }}
                onClick={() => setZoomedImage('')} // Close button handler
               >
                <CloseIcon />
              </IconButton>
            </Box>
          )
        }
      
       
     <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '0.5rem', mt: 2, width: '100%',  }}>
        <Button
            variant="contained"
            color="primary"
            size="small"
            sx={{
                textTransform: 'none',
                bgcolor: '#F0BEB0',
                color: 'black',
                fontWeight: 800,
                padding: '2px 8px',
                minWidth: 'auto',
                height: 'auto'
              }}
            >
            AI
        </Button>
            <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>
            Helix System
            </Typography>
            <Typography variant="subtitle1" sx={{ fontWeight: 800, marginLeft: 'auto', display: 'flex', alignItems: 'center' }}>
            <RefreshIcon sx={{ color: '#B4FDC0', mr: 0 }} /> 
            <span style={{ color: '#B4FDC0' }}>Restart</span>
            </Typography>
     </Box>
      
     <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', mt: 2, width: '100%' }}>
        <Typography className="interactionMessage" gutterBottom>
          You have completed a fine tuning session on these images.
       </Typography>
       <Box component="ul" sx={{ pl: 4, mt: -1 }}>
         <Typography component="li">You can now chat to your model, add some more documents and re-train or you can "Clone" from this point in time.</Typography>
         <Typography component="li">Describe what you want to see in an image. Use "a photo" to refer to fine tuned concepts, people or styles.</Typography>
       </Box>
     </Box>
      <Box sx={{ mt: 2, display: 'flex', justifyContent: 'flex-start', gap: 1  }}>
        <Button
          variant="contained"
          sx={{
              bgcolor: '#3BF959',
              color: 'black',
              textTransform: 'none',
          }}
          >
          Add more images
        </Button>
        <Button
          variant="contained"
          sx={{
              bgcolor: 'white',
              color: 'black',
              textTransform: 'none',
          }}
          >
          Clone
        </Button>
      </Box>
      <Box sx={{ mt: 10 }}> {/* Adjust the margin as needed */}
        <TextField
            fullWidth
            placeholder="Describe what you want to see in an image. (Shift + Enter to add new lines)"
            multiline
            InputLabelProps={{
            shrink: true, // This ensures the label does not overlap with the placeholder text
            }}
            InputProps={{
            style: {
                backgroundColor: 'transparent',
            },
            }}
        />
      </Box>
    </Container>
  );
};

export default ImageFineTuneMoreView;