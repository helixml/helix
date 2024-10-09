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

  return (
    <Box
      sx={{
        '& code': {
          backgroundColor: theme.palette.mode === 'light' ? '#ccc' : '#333',
          fontSize: '0.9rem',
          p: 0.5,
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