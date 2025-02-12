import React, { FC } from 'react'
import { useTheme } from '@mui/material/styles'
import Box from '@mui/material/Box'
import Markdown from 'react-markdown'
import {Prism as SyntaxHighlighterTS} from 'react-syntax-highlighter'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import { keyframes } from '@mui/material/styles'
// you can change the theme by picking one from here
// https://react-syntax-highlighter.github.io/react-syntax-highlighter/demo/prism.html
import {oneDark} from 'react-syntax-highlighter/dist/esm/styles/prism'

const SyntaxHighlighter = SyntaxHighlighterTS as any

const rainbowShadow = keyframes`
  0% { filter: drop-shadow(0 0 2px #ff0000) drop-shadow(0 0 4px #ff00ff); }
  20% { filter: drop-shadow(0 0 2px #ff00ff) drop-shadow(0 0 4px #0000ff); }
  40% { filter: drop-shadow(0 0 2px #0000ff) drop-shadow(0 0 4px #00ffff); }
  60% { filter: drop-shadow(0 0 2px #00ffff) drop-shadow(0 0 4px #00ff00); }
  80% { filter: drop-shadow(0 0 2px #00ff00) drop-shadow(0 0 4px #ffff00); }
  100% { filter: drop-shadow(0 0 2px #ffff00) drop-shadow(0 0 4px #ff0000); }
`

export const InteractionMarkdown: FC<{
  text: string,
}> = ({
  text,
}) => {
  const theme = useTheme()
  if(!text) return null

  // Fix markdown rendering for code blocks and process think tags
  const processContent = (text: string) => {
    // Fix code block indentation
    let processed = text.replace(/^\s*```/gm, '```');

    // Replace "---" with "</think>" if there's an unclosed think tag
    let openCount = 0;
    processed = processed.split('\n').map(line => {
      if (line.includes('<think>')) openCount++;
      if (line.includes('</think>')) openCount--;
      if (line.trim() === '---' && openCount > 0) {
        openCount--;
        return '</think>';
      }
      return line;
    }).join('\n');

    // Check if there's an unclosed think tag
    const openTagCount = (processed.match(/<think>/g) || []).length;
    const closeTagCount = (processed.match(/<\/think>/g) || []).length;
    const isThinking = openTagCount > closeTagCount;

    if (isThinking) {
      // If there's an unclosed tag, add the closing tag
      processed += '\n</think>';
    }

    // Convert <think> tags to styled divs, skipping empty ones
    processed = processed.replace(
      /<think>([\s\S]*?)<\/think>/g,
      (_, content) => {
        const trimmedContent = content.trim();
        if (!trimmedContent) return ''; // Skip empty think tags

        return `<div class="think-container${isThinking ? ' thinking' : ''}"><details${isThinking ? ' open' : ''}><summary class="think-header"><strong>Reasoning</strong></summary><div class="think-content">

${trimmedContent}

</div></details></div>`;
      }
    );

    return processed;
  };

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
        '& .think-container': {
          margin: '1em 0',
          padding: '1em',
          borderRadius: '8px',
          backgroundColor: theme.palette.mode === 'light' ? '#f5f5f5' : '#2a2a2a',
          border: `1px solid ${theme.palette.mode === 'light' ? '#ddd' : '#444'}`,
        },
        '& .think-container.thinking': {
          animation: `${rainbowShadow} 10s linear infinite`,
        },
        '& .think-header': {
          display: 'flex',
          alignItems: 'center',
          gap: '0.5em',
          cursor: 'pointer',
          '&::-webkit-details-marker': {
            display: 'none'
          },
          '&::before': {
            content: '"▶"',
            transition: 'transform 0.2s',
          }
        },
        '& details[open] .think-header::before': {
          content: '"▼"',
        },
        '& .think-content': {
          marginTop: '0.5em',
        },
      }}
    >
      <Markdown
        children={processContent(text)}
        remarkPlugins={[remarkGfm]}
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
          },
          p(props) {
            const { children } = props;
            return (
              <Box
                component="p"
                sx={{
                  my: 0.5,
                  lineHeight: 1.4,
                }}
              >
                {React.Children.map(children, child => {
                  if (typeof child === 'string') {
                    return child.split('\n').map((line, i, arr) => (
                      <React.Fragment key={i}>
                        {line}
                        {i < arr.length - 1 && <br />}
                      </React.Fragment>
                    ));
                  }
                  return child;
                })}
              </Box>
            );
          }
        }}
      />
    </Box>
  )
}

export default InteractionMarkdown