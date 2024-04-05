import React, { FC, useState } from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Button from '@mui/material/Button';
import Card from '@mui/material/Card';
import CardMedia from '@mui/material/CardMedia';
import IconButton from '@mui/material/IconButton';
import ZoomOutMapIcon from '@mui/icons-material/ZoomOutMap';
import TextField from '@mui/material/TextField';

// this is an example of how to convert upstream api data
// into the require format
// interface ActualUploadedImage {
//   ID: string,
//   imageURL: string,
//   comment: string,
// }

// const convertActualUploadedImage = (uploadedImage: UploadedImage): ActualUploadedImage => {
//   return {
//     ID: uploadedImage.id,
//     imageURL: uploadedImage.url,
//     comment: uploadedImage.description,
//   };
// }

interface UploadedImage {
  id: string,
  url: string,
  description: string,
}

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
}, {
  id: '3',
  url: 'https://www.pixelstalk.net/wp-content/uploads/2016/07/Free-Amazing-Background-Images-Nature.jpg',
  description: 'test image 3',
}]

const ObiMock: FC = () => {
  const [ zoomedImage, setZoomedImage ] = useState<string>('');

  const currentDate = new Date();
  const formattedDate = currentDate.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });

  
  return (
    <Container
      maxWidth="xl"
      sx={{
        mt: 12,
        height: 'calc(100% - 100px)',
       
        
      }}
    >
      <Box sx={{ display: 'flex', gap: '0.5rem' }}>
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
        <Typography variant="subtitle1" sx={{ fontWeight: 800, marginLeft: 'auto' ,   }}>
          {formattedDate}
        </Typography>
      </Box>
      
      <Box sx={{ mt: 2 }}>
        <Typography className="interactionMessage" gutterBottom>
          Describe in as much detail as you can, what is present in each image. Try to describe:
        </Typography>
        <Box component="ul" sx={{ pl: 4, mt: -1 }}>
          <Typography component="li">The subject of what the photo is doing</Typography>
          <Typography component="li">What else is visible in the image</Typography>
          <Typography component="li">The attributes of the image itself</Typography>
        </Box>
      </Box>
      <Grid container spacing={2}>
        {/* Create 8 cards and text fields */}
        {
          IMAGE_DATA.map((image, index) => {
            return (
              <Grid item xs={12} sm={6} md={4} lg={3} key={index}>
                <Card sx={{ position: 'relative', width: '100%', height: 250, backgroundColor: 'transparent', boxShadow: 'none', mt: 2 }}>
                  <CardMedia component="img" height="200" image={image.url} alt={image.description} />
                  <IconButton
                    sx={{
                      position: 'absolute',
                      bottom: 8,
                      right: 8,
                      backgroundColor: 'transparent',
                    }}
                    aria-label="zoom out"
                    onClick={ () => {
                      setZoomedImage(image.url)
                    }}
                  >
                    <ZoomOutMapIcon />
                  </IconButton>
                </Card>
                <TextField
                  sx={{ width: '100%', mt: 2 }} // Reduced margin-top for the TextField
                  placeholder={image.description}
                  multiline
                  rows={2}
                />
              </Grid>
            )
          })
        }
      </Grid>
      <Grid container spacing={3} direction="row" justifyContent="space-between" alignItems="center" sx={{ mt: 2, mb: 2, }}>
        <Grid item xs={6}>
          <Button
            component="button"
            onClick={() => {}}
            sx={{
              color: '#3BF959',
              textTransform: 'none',
              
            }}
          >
            Return to upload images
          </Button>
        </Grid>
        <Grid item xs={6} style={{ textAlign: 'right' }} >
          <Button
            sx={{
              bgcolor: '#3BF959',
              color: 'black',
              textTransform: 'none',
            }}
            variant="contained"
          >
            Start training
          </Button>
        </Grid>
      </Grid>
      {
        // you will need a close button that has "setZoomedImage('')" as the onClick handler
        zoomedImage && (
          <Box
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              height: '100%',
              backgroundColor: 'rgba(0, 0, 0, 0.8)',
            }}
          >
            <img src={ zoomedImage } />

          </Box>
        )
      }
    </Container>
  );
};

export default ObiMock;