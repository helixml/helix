import React, { FC } from 'react'
import { useTheme } from '@mui/material/styles'
import Box from '@mui/material/Box'
import Markdown from 'react-markdown'
import {Prism as SyntaxHighlighterTS} from 'react-syntax-highlighter'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize from 'rehype-sanitize'
// you can change the theme by picking one from here
// https://react-syntax-highlighter.github.io/react-syntax-highlighter/demo/prism.html
import {oneDark} from 'react-syntax-highlighter/dist/esm/styles/prism'

const SyntaxHighlighter = SyntaxHighlighterTS as any

export const InteractionMarkdown: FC<{
  text: string,
}> = ({
  text,
}) => {
  const theme = useTheme()
  if(!text) return null

  // Function to add <br> tags for single newlines, except inside fenced code blocks
  const addLineBreaks = (text: string) => {
    const lines = text.split('\n');
    let insideCodeBlock = false;
    let result = '';

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      
      if (line.trim().startsWith('```')) {
        insideCodeBlock = !insideCodeBlock;
      }

      result += line;

      if (!insideCodeBlock && i < lines.length - 1) {
        const nextLine = lines[i + 1];
        if (line.trim() !== '' && nextLine.trim() !== '' && !nextLine.trim().startsWith('```')) {
          result += '<br>';
        }
      }

      if (i < lines.length - 1) {
        result += '\n';
      }
    }

    return result;
  };

  // Apply the line break transformation to the text
  text = addLineBreaks(text);

  return (
    <Box
      sx={{
        '& pre': {
          backgroundColor: theme.palette.mode === 'light' ? '#f0f0f0' : '#1e1e1e',
          padding: '1em',
          borderRadius: '4px',
          overflowX: 'auto',
        },
        '& code': {
          backgroundColor: 'transparent',
          fontSize: '0.9rem',
        },
        '& :not(pre) > code': {
          backgroundColor: theme.palette.mode === 'light' ? '#ccc' : '#333',
          padding: '0.2em 0.4em',
          borderRadius: '3px',
        },
        '& a': {
          color: theme.palette.mode === 'light' ? '#333' : '#bbb',
        },
      }}
    >
      <Markdown
        children={text}
        remarkPlugins={[remarkGfm]}
        // TODO re-add rehypeSanitize while not breaking the flashing yellow cursor
        rehypePlugins={[rehypeRaw]}
        className="interactionMessage"
        components={{
          code(props) {
            const {children, className, node, ...rest} = props
            const match = /language-(\w+)/.exec(className || '')
            return match ? (
              <SyntaxHighlighter
                {...rest}
                PreTag="div"
                children={String(children).replace(/\n$/, '')}
                language={match[1]}
                style={oneDark}
              />
            ) : (
              <code {...rest} className={className}>
                {children}
              </code>
            )
          }
        }}
      />
    </Box>
  )
}

export default InteractionMarkdown