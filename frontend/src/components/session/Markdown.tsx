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

// Create a blinking animation for the cursor
const blink = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
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
    // First check if the text already contains our citation container HTML
    // If it does, we need to temporarily replace it to prevent it from being escaped
    const hasCitationHTML = text.includes('<div class="rag-citations-container">');
    const hasEscapedCitationHTML = text.includes('&lt;div class="rag-citations-container"&gt;');
    let citationContainers: Array<string> = [];
    let processedWithPreservedCitations = text;
    
    // Handle proper HTML citations
    if (hasCitationHTML) {
      // Extract all citation containers and replace with placeholders
      let citationIndex = 0;
      const citationRegex = /<div class="rag-citations-container">[\s\S]*?<\/div>\s*<\/div>\s*<\/div>/g;
      processedWithPreservedCitations = text.replace(citationRegex, (match) => {
        const placeholder = `__CITATION_PLACEHOLDER_${citationIndex}__`;
        citationContainers.push(match);
        citationIndex++;
        return placeholder;
      });
      
      console.debug(`Preserved ${citationContainers.length} citation containers for rendering`);
    }
    
    // Handle already escaped HTML
    if (hasEscapedCitationHTML) {
      // Unescape the HTML for proper rendering
      processedWithPreservedCitations = processedWithPreservedCitations.replace(
        /&lt;div class="rag-citations-container"&gt;[\s\S]*?&lt;\/div&gt;\s*&lt;\/div&gt;\s*&lt;\/div&gt;/g,
        match => {
          // Convert &lt; to < and &gt; to >
          const unescaped = match
            .replace(/&lt;/g, '<')
            .replace(/&gt;/g, '>')
            .replace(/&quot;/g, '"')
            .replace(/&#039;/g, "'")
            .replace(/&amp;/g, '&');
            
          // Add to the containers array
          const placeholder = `__CITATION_PLACEHOLDER_${citationContainers.length}__`;
          citationContainers.push(unescaped);
          return placeholder;
        }
      );
    }
    
    // Also preserve any links added by replaceMessageText
    let linkIndex = 0;
    let links: Array<string> = [];
    const linkRegex = /<a\s+[^>]*?href=["'][^"']*?["'][^>]*?>.*?<\/a>/g;
    processedWithPreservedCitations = processedWithPreservedCitations.replace(linkRegex, (match) => {
      const placeholder = `__LINK_PLACEHOLDER_${linkIndex}__`;
      links.push(match);
      linkIndex++;
      return placeholder;
    });
    
    if (linkIndex > 0) {
      console.debug(`Preserved ${linkIndex} HTML links for rendering`);
    }

    // First ensure that all non-standard XML tags are escaped
    // This prevents browser warnings and errors about unrecognized tags
    let processed = processedWithPreservedCitations.replace(/<(\/?)(?!a>|span>|div>|code>|pre>|br>|strong>|em>|ul>|ol>|li>|p>|h[1-6]>|img>|table>|tr>|td>|th>)([^>]+)>/g, (match) => {
      // If it's already an HTML entity, leave it alone
      if (match.startsWith('&lt;')) {
        return match;
      }
      // Special handling for think tag which we process specially
      if (match.includes('think')) {
        return match;
      }
      // Convert to HTML entities
      return match.replace(/</g, '&lt;').replace(/>/g, '&gt;');
    });

    // Fix code block indentation
    processed = processed.replace(/^\s*```/gm, '```');

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
    
    // Restore the links first
    if (links.length > 0) {
      links.forEach((link, index) => {
        const placeholder = `__LINK_PLACEHOLDER_${index}__`;
        processed = processed.replace(placeholder, link);
      });
    }
    
    // Now restore the citation containers
    if (citationContainers.length > 0) {
      citationContainers.forEach((citation, index) => {
        const placeholder = `__CITATION_PLACEHOLDER_${index}__`;
        processed = processed.replace(placeholder, citation);
      });
    }

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
        // RAG Citations styling
        '& .rag-citations-container': {
          margin: '20px 0 10px 0',
          padding: '16px',
          backgroundColor: theme.palette.mode === 'light' ? 'rgba(240, 240, 240, 0.7)' : 'rgba(30, 30, 30, 0.4)',
          borderRadius: '8px',
          border: `1px solid ${theme.palette.mode === 'light' ? '#ddd' : '#444'}`,
        },
        '& .rag-citations-header': {
          fontWeight: 'bold',
          marginBottom: '12px',
          fontSize: '1.1em',
          color: theme.palette.mode === 'light' ? '#333' : '#eee',
        },
        '& .rag-citations-list': {
          display: 'flex',
          flexDirection: 'column',
          gap: '12px',
        },
        '& .rag-citation-item': {
          backgroundColor: theme.palette.mode === 'light' ? 'rgba(255, 255, 255, 0.7)' : 'rgba(50, 50, 50, 0.4)',
          borderRadius: '6px',
          padding: '12px',
          transition: 'background-color 0.2s',
          border: `1px solid ${theme.palette.mode === 'light' ? '#e0e0e0' : '#555'}`,
        },
        '& .rag-citation-item:hover': {
          backgroundColor: theme.palette.mode === 'light' ? 'rgba(250, 250, 250, 0.9)' : 'rgba(60, 60, 60, 0.5)',
          boxShadow: theme.palette.mode === 'light' 
            ? '0 2px 5px rgba(0, 0, 0, 0.05)' 
            : '0 2px 5px rgba(0, 0, 0, 0.15)',
        },
        '& .rag-citation-header': {
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          marginBottom: '8px',
        },
        '& .rag-citation-icon': {
          width: '20px',
          height: '20px',
          color: theme.palette.mode === 'light' ? '#666' : '#aaa',
        },
        '& .rag-citation-link': {
          color: theme.palette.mode === 'light' ? '#0366d6' : '#58a6ff',
          textDecoration: 'none',
          fontWeight: '500',
        },
        '& .rag-citation-link:hover': {
          textDecoration: 'underline',
        },
        '& .rag-citation-snippet': {
          fontSize: '0.9em',
          lineHeight: '1.5',
          marginLeft: '28px',
          paddingLeft: '10px',
          borderLeft: `2px solid ${theme.palette.mode === 'light' ? '#ddd' : '#555'}`,
        },
        '& .rag-citation-quote': {
          color: theme.palette.mode === 'light' ? '#444' : '#ddd',
          fontStyle: 'italic',
        },
        '& .rag-citation-snippet code': {
          backgroundColor: theme.palette.mode === 'light' ? 'rgba(0, 0, 0, 0.05)' : 'rgba(255, 255, 255, 0.1)',
          padding: '2px 4px',
          borderRadius: '3px',
          fontFamily: 'monospace',
          fontSize: '0.85em',
          color: theme.palette.mode === 'light' ? '#d63200' : '#ff9580',
        },
        // Blinker styling
        '& .blinker-class': {
          animation: `${blink} 1.2s step-end infinite`,
          marginLeft: '2px',
          color: theme.palette.mode === 'light' ? 'rgba(0, 0, 0, 0.7)' : 'rgba(255, 255, 255, 0.7)',
          fontWeight: 'normal',
          userSelect: 'none',
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