import React, { FC, useState } from 'react'
import { Box, Typography, Switch, FormControlLabel, IconButton, Tooltip } from '@mui/material'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'
import useSnackbar from '../../hooks/useSnackbar'

interface JsonViewProps {
  data: any,
  withFancyRendering?: boolean,
  withFancyRenderingControls?: boolean,
  scrolling?: boolean
}

const decodeHtmlEntities = (str: string): string => {
  const textarea = document.createElement('textarea');
  textarea.innerHTML = str;
  return textarea.value;
}

const formatJsonString = (jsonString: string): string => {
  return decodeHtmlEntities(jsonString.replace(/\\n/g, '\n').replace(/\\"/g, '"'));
}

const commonStyles = {
  fontFamily: '"Roboto Mono", monospace',
  color: 'white',
  fontSize: '0.775rem',
}

const renderJsonValue = (value: any): JSX.Element => {
  if (typeof value === 'string') {
    return <Typography component="span" style={{ ...commonStyles, whiteSpace: 'pre-wrap' }}>{formatJsonString(JSON.stringify(value).slice(1, -1))}</Typography>;
  } else if (typeof value === 'number' || typeof value === 'boolean') {
    return <Typography component="span" style={commonStyles}>{JSON.stringify(value)}</Typography>;
  } else if (value === null) {
    return <Typography component="span" style={commonStyles}>null</Typography>;
  } else if (Array.isArray(value)) {
    return (
      <Box component="span" style={commonStyles}>
        [
        <Box component="div" style={{ marginLeft: 20 }}>
          {value.map((item, index) => (
            <div key={index}>{renderJsonValue(item)}{index < value.length - 1 ? ',' : ''}</div>
          ))}
        </Box>
        ]
      </Box>
    );
  } else if (typeof value === 'object') {
    return (
      <Box component="span" style={commonStyles}>
        {'{'}
        <Box component="div" style={{ marginLeft: 20 }}>
          {Object.entries(value).map(([key, val], index, array) => (
            <div key={key}>
              <Typography component="span" fontWeight="bold" style={commonStyles}>{JSON.stringify(key)}</Typography>: {renderJsonValue(val)}
              {index < array.length - 1 ? ',' : ''}
            </div>
          ))}
        </Box>
        {'}'}
      </Box>
    );
  }
  return <Typography component="span" style={commonStyles}>{JSON.stringify(value)}</Typography>;
}

const JsonView: FC<React.PropsWithChildren<JsonViewProps>> = ({
  data,
  withFancyRendering = true,
  withFancyRenderingControls = true,
  scrolling = false
}) => {
  const [useFancyRendering, setUseFancyRendering] = useState(withFancyRendering);
  const [showCopied, setShowCopied] = useState(false);
  const snackbar = useSnackbar();

  const toggleRendering = () => {
    setUseFancyRendering(!useFancyRendering);
  };

  const handleCopy = () => {
    const textToCopy = typeof(data) === 'string' ? data : JSON.stringify(data, null, 2);
    navigator.clipboard.writeText(textToCopy)
      .then(() => {
        setShowCopied(true);
        setTimeout(() => setShowCopied(false), 2000);
        snackbar.success('Copied to clipboard');
      })
      .catch((error) => {
        console.error('Failed to copy:', error);
        snackbar.error('Failed to copy to clipboard');
      });
  };

  return (
    <Box>
      {
        withFancyRenderingControls && (
          <FormControlLabel
            control={<Switch checked={useFancyRendering} onChange={toggleRendering} />}
            label="Fancy Rendering"
          />
        )
      }
      <Box
        sx={{
          position: 'relative',
          fontFamily: '"Roboto Mono", monospace',
          whiteSpace: 'pre-wrap',
          overflowX: 'auto',
          overflowY: scrolling ? 'auto' : 'visible',
          maxHeight: scrolling ? '400px' : 'none',
          padding: 2,
          backgroundColor: '#121212',
          color: 'white',
          borderRadius: 1,
        }}
      >
        <Tooltip 
          title={showCopied ? "Copied!" : "Copy to clipboard"}
          placement="top"
          open={showCopied}
        >
          <IconButton
            onClick={handleCopy}
            sx={{
              position: 'absolute',
              top: 8,
              right: 8,
              color: 'white',
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.1)',
              },
            }}
          >
            {showCopied ? <CheckIcon /> : <ContentCopyIcon />}
          </IconButton>
        </Tooltip>
        {useFancyRendering
          ? renderJsonValue(data)
          : <pre>{JSON.stringify(data, null, 2)}</pre>}
      </Box>
    </Box>
  )
}

export default JsonView