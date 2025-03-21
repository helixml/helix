import React, { FC, ReactElement, useState, useEffect, useMemo, useCallback } from 'react'
import { useTheme } from '@mui/material/styles'
import Box from '@mui/material/Box'
import Markdown from 'react-markdown'
import { Prism as SyntaxHighlighterTS } from 'react-syntax-highlighter'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import { keyframes } from '@mui/material/styles'
// you can change the theme by picking one from here
// https://react-syntax-highlighter.github.io/react-syntax-highlighter/demo/prism.html
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { ISession } from '../../types'
// Import the escapeRegExp helper from session.ts
import { escapeRegExp } from '../../utils/session'
import DOMPurify from 'dompurify'

// Import the new Citation component
import Citation, { Excerpt } from './Citation'

const SyntaxHighlighter = SyntaxHighlighterTS as any

// Create a rainbow shadow animation
const rainbowShadow = keyframes`
  0% { box-shadow: 0 0 12px 4px rgba(255, 0, 0, 0.5), 0 0 20px 8px rgba(255, 0, 255, 0.3); }
  20% { box-shadow: 0 0 12px 4px rgba(255, 0, 255, 0.5), 0 0 20px 8px rgba(0, 0, 255, 0.3); }
  40% { box-shadow: 0 0 12px 4px rgba(0, 0, 255, 0.5), 0 0 20px 8px rgba(0, 255, 255, 0.3); }
  60% { box-shadow: 0 0 12px 4px rgba(0, 255, 255, 0.5), 0 0 20px 8px rgba(0, 255, 0, 0.3); }
  80% { box-shadow: 0 0 12px 4px rgba(0, 255, 0, 0.5), 0 0 20px 8px rgba(255, 255, 0, 0.3); }
  100% { box-shadow: 0 0 12px 4px rgba(255, 255, 0, 0.5), 0 0 20px 8px rgba(255, 0, 0, 0.3); }
`

// Create a blinking animation for the cursor
const blink = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
`

// Create a pulsing animation for partial citations
const pulseFade = keyframes`
  0% { opacity: 0.7; }
  50% { opacity: 0.9; }
  100% { opacity: 0.7; }
`

// Create a shimmer animation for loading indicators
const shimmer = keyframes`
  0% { background-position: 100% 0; }
  100% { background-position: -100% 0; }
`

// Create a subtle bounce animation for loading content
const subtleBounce = keyframes`
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-1px); }
`

// Create a fade-in animation for citation boxes
const fadeIn = keyframes`
  0% { opacity: 0; transform: translateX(10px); }
  100% { opacity: 1; transform: translateX(0); }
`

export interface MessageProcessorOptions {
  session: ISession;
  getFileURL: (filename: string) => string;
  isStreaming: boolean;
  showBlinker?: boolean;
  onFilterDocument?: (docId: string) => void;
}

export interface CitationData {
  excerpts: {
    docId: string;
    snippet: string;
    title?: string;
    page?: number;
    filename: string;
    fileUrl: string;
    isPartial: boolean;
    citationNumber?: number;
  }[];
  isStreaming?: boolean;
}

/**
 * Central message processor that handles all text formatting
 * Including document links, citations, thinking tags and blinkers
 */
export class MessageProcessor {
  private message: string;
  private options: MessageProcessorOptions;
  private citationData: CitationData | null = null;

  constructor(message: string, options: MessageProcessorOptions) {
    this.message = message;
    this.options = options;
  }

  process(): string {
    let processedMessage = this.message;
    
    // Process XML citations
    processedMessage = this.processXmlCitations(processedMessage);
    
    // Process HTML citations
    processedMessage = this.processHtmlCitations(processedMessage);
    
    // Process document IDs and convert to links
    processedMessage = this.processDocumentIds(processedMessage);
    
    // Process document group IDs and convert to links
    processedMessage = this.processDocumentGroupIds(processedMessage);
    
    // Process thinking tags
    processedMessage = this.processThinkingTags(processedMessage);
    
    // Fix code block indentation
    processedMessage = this.fixCodeBlockIndentation(processedMessage);
    
    // Remove trailing triple dash during streaming
    if (this.options.isStreaming) {
      processedMessage = this.removeTrailingTripleDash(processedMessage);
    }
    
    // Sanitize HTML
    processedMessage = this.sanitizeHtml(processedMessage);
    
    // Add blinker if requested and appropriate
    if (this.options.showBlinker && !this.citationData) {
      if (this.options.isStreaming) {
        processedMessage = this.addBlinker(processedMessage);
      }
    }
    
    // Add citation data as a special marker if present
    if (this.citationData) {
      processedMessage = this.addCitationData(processedMessage);
    }
    
    return processedMessage;
  }

  private processXmlCitations(message: string): string {
    // Look for XML citation format <excerpts>...</excerpts>
    const citationRegex = /<excerpts>([\s\S]*?)<\/excerpts>/g;
    const citationMatches = message.match(citationRegex);
    
    if (!citationMatches) {
      // Check for partial excerpts during streaming
      if (this.options.isStreaming && message.includes('<excerpts>')) {
        // Find the content after the opening tag
        const partialExcerpts = message.split('<excerpts>')[1];
        
        // Initialize citation data for streaming
        this.citationData = {
          excerpts: [{
            docId: "loading",
            snippet: "Loading source information...",
            filename: "Loading...",
            fileUrl: "#",
            isPartial: true
          }],
          isStreaming: true
        };
        
        // In streaming mode, remove the partial excerpts
        return message.split('<excerpts>')[0];
      }
      
      return message;
    }
    
    // Initialize citation data if not already done
    if (!this.citationData) {
      this.citationData = {
        excerpts: [],
        isStreaming: this.options.isStreaming && !message.includes('</excerpts>')
      };
    }
    
    // Process each citation match
    for (const match of citationMatches) {
      // Extract document ID and snippet from the XML
      const docIdMatch = match.match(/<document_id>(.*?)<\/document_id>/);
      const snippetMatch = match.match(/<snippet>([\s\S]*?)<\/snippet>/);
      
      if (docIdMatch && snippetMatch) {
        const docId = docIdMatch[1];
        const snippet = snippetMatch[1];
        // Find associated filename for this document ID
        let filename = "Document";
        let fileUrl = "#";
        
        if (this.options.session.config?.document_ids) {
          // Find the filename for this docId by checking the document_ids object
          const docIdsMap = this.options.session.config.document_ids;
          for (const fname in docIdsMap) {
            if (docIdsMap[fname] === docId) {
              // Extract just the basename from the path
              filename = fname.split('/').pop() || fname;
              fileUrl = this.options.getFileURL(fname);
              break;
            }
          }
        }
        
        // Add to citation data
        this.citationData.excerpts.push({
          docId,
          snippet,
          filename,
          fileUrl,
          isPartial: false
        });
      }
    }
    
    // Remove citation XML from the message
    return message.replace(citationRegex, '');
  }

  private processHtmlCitations(message: string): string {
    // Look for HTML citation format <div class="rag-citations-container">...</div>
    const citationRegex = /<div class="rag-citations-container">([\s\S]*?)<\/div>/g;
    const citationMatches = message.match(citationRegex);
    
    if (!citationMatches) {
      return message;
    }
    
    // Initialize citation data if not already done
    if (!this.citationData) {
      this.citationData = {
        excerpts: [],
        isStreaming: false
      };
    }
    
    // Process each citation match
    for (const match of citationMatches) {
      // Extract quotes
      const quoteRegex = /<div class="citation-quote">([\s\S]*?)<\/div>/g;
      const quoteMatches = [...match.matchAll(quoteRegex)];
      
      // Extract sources
      const sourceRegex = /<div class="citation-source"><a href="[^"]*">([^<]*)<\/a><\/div>/g;
      const sourceMatches = [...match.matchAll(sourceRegex)];
      
      // If we can't extract properly, create a default excerpt
      if (quoteMatches.length === 0 || sourceMatches.length === 0) {
        // Handle the case where regex doesn't match as expected
        // Create a default excerpt using the content of the citation container
        this.citationData.excerpts.push({
          docId: "quoted_passage",
          snippet: "This is a quoted passage",
          title: "Citation",
          filename: "Citation",
          fileUrl: "#",
          isPartial: false
        });
      } else {
        // Combine quotes with sources
        for (let i = 0; i < Math.min(quoteMatches.length, sourceMatches.length); i++) {
          const snippet = quoteMatches[i][1].replace(/^"|"$/g, ''); // Remove surrounding quotes
          const filename = sourceMatches[i][1];
          
          // Extract just the basename from the path
          const displayName = filename.split('/').pop() || filename;
          
          // Find document ID for this filename if available
          const docId = this.options.session.config?.document_ids?.[filename] || filename;
          const fileUrl = this.options.getFileURL(filename);
          
          // Add to citation data
          this.citationData.excerpts.push({
            docId,
            snippet,
            filename: displayName,
            fileUrl,
            isPartial: false
          });
        }
      }
    }
    
    // Remove citation HTML from the message
    return message.replace(citationRegex, '');
  }

  private processDocumentIds(message: string): string {
    if (!this.options.session.config?.document_ids) {
      return message;
    }
    
    let processedMessage = message;
    const docIds = Object.entries(this.options.session.config.document_ids);
    
    // Process document IDs
    let docCounter = 1;
    
    // Create a map to associate docIds with citation numbers
    const citationMap: Record<string, number> = {};
    
    for (const [filename, docId] of docIds) {
      // Match the entire pattern with brackets: [DOC_ID:id]
      const docRegex = new RegExp(`\\[DOC_ID:${escapeRegExp(docId)}\\]`, 'g');
      
      if (processedMessage.match(docRegex)) {
        const fileUrl = this.options.getFileURL(filename);
        citationMap[docId] = docCounter;
        
        // Replace the entire pattern including brackets
        processedMessage = processedMessage.replace(
          docRegex,
          `<a target="_blank" href="${fileUrl}" class="doc-citation">[${docCounter}]</a>`
        );
        
        docCounter++;
      }
    }
    
    // Add citation numbers to excerpts if we have citation data
    if (this.citationData && this.citationData.excerpts) {
      for (let i = 0; i < this.citationData.excerpts.length; i++) {
        const excerpt = this.citationData.excerpts[i];
        const citationNumber = citationMap[excerpt.docId];
        if (citationNumber) {
          // Create a new object with the citationNumber added
          this.citationData.excerpts[i] = {
            ...excerpt,
            citationNumber
          };
        }
      }
    }
    
    return processedMessage;
  }

  private processDocumentGroupIds(message: string): string {
    if (!this.options.session.config?.document_group_id) {
      return message;
    }
    
    const groupId = this.options.session.config.document_group_id;
    const groupRegex = new RegExp(`\\b${groupId}\\b`, 'g');
    
    // Replace group ID with link if it exists in the message
    if (message.match(groupRegex)) {
      return message.replace(
        groupRegex,
        `<a href="#" class="doc-group-link">[group]</a>`
      );
    }
    
    return message;
  }

  private processThinkingTags(message: string): string {
    // Check for any <think> tags
    if (!message.includes('<think>')) {
      return message;
    }
    
    // Fix code block indentation
    let processedMessage = message.replace(/^\s*```/gm, '```');

    // Handle triple dash as think tag closing delimiter during streaming
    if (this.options.isStreaming) {
      // Replace --- with </think> if it's in a thinking block
    let openCount = 0;
      processedMessage = processedMessage.split('\n').map(line => {
      if (line.includes('<think>')) openCount++;
      if (line.includes('</think>')) openCount--;
      if (line.trim() === '---' && openCount > 0) {
        openCount--;
        return '</think>';
      }
      return line;
    }).join('\n');
    }

    // Check if there's an unclosed think tag
    const openTagCount = (processedMessage.match(/<think>/g) || []).length;
    const closeTagCount = (processedMessage.match(/<\/think>/g) || []).length;
    const isThinking = openTagCount > closeTagCount;

    // Add closing tag if needed and not streaming
    if (isThinking && !this.options.isStreaming) {
      processedMessage += '\n</think>';
    }

    // Replace closed think tags with styled divs
    processedMessage = processedMessage.replace(
      /<think>([\s\S]*?)<\/think>/g,
      (_, content) => {
        const trimmedContent = content.trim();
        if (!trimmedContent) return ''; // Skip empty think tags

        // Closed thinking tags get a regular container with closed details
        return `<div class="think-container"><details><summary class="think-header"><strong>Reasoning</strong></summary><div class="think-content">

${trimmedContent}

</div></details></div>`;
      }
    );

    // Handle unclosed thinking tags during streaming
    if (isThinking && this.options.isStreaming) {
      // Find the last unclosed <think> tag
      const lastThinkTagMatch = processedMessage.match(/<think>([\s\S]*)$/);
      
      if (lastThinkTagMatch) {
        const content = lastThinkTagMatch[1].trim();
        if (content) {
          // Replace the unclosed <think> tag with a container that has the "thinking" class
          const replacement = `<div class="think-container thinking"><details open><summary class="think-header"><strong>Reasoning</strong></summary><div class="think-content">

${content}

</div></details></div>`;
          
          processedMessage = processedMessage.replace(
            /<think>([\s\S]*)$/,
            replacement
          );
        }
      }
    }

    return processedMessage;
  }

  private fixCodeBlockIndentation(message: string): string {
    // Find code blocks with indentation
    const codeBlockRegex = /^(\s+)```([\s\S]*?)^\1```/gm;
    
    // Remove the indentation at the start and end of code blocks
    return message.replace(codeBlockRegex, (match, indent, content) => {
      return '```' + content + '```';
    });
  }

  private removeTrailingTripleDash(message: string): string {
    // Remove triple dash at the end of content during streaming
    return message.replace(/\n---\s*$/, '');
  }

  private sanitizeHtml(message: string): string {
    // Use DOMPurify to sanitize HTML while preserving safe tags and attributes
    return DOMPurify.sanitize(message, {
      ALLOWED_TAGS: ['a', 'p', 'br', 'strong', 'em', 'div', 'span', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'code', 'pre', 'blockquote', 'details', 'summary'],
      ALLOWED_ATTR: ['href', 'target', 'class', 'style', 'title', 'id', 'aria-hidden', 'aria-label', 'role'],
      ADD_ATTR: ['target']
    });
  }

  private addBlinker(message: string): string {
    // Add blinker at the end of content
    return message + '<span class="blinker-class">┃</span>';
  }

  private addCitationData(message: string): string {
    // Add citation data as a special marker that can be picked up by React component
    const citationJson = JSON.stringify(this.citationData);
    return message + `__CITATION_DATA__${citationJson}__CITATION_DATA__`;
  }

  getCitationData(): CitationData | null {
    return this.citationData;
  }
}

// Add an areEqual function for React.memo to optimize rendering
const arePropsEqual = (prevProps: InteractionMarkdownProps, nextProps: InteractionMarkdownProps) => {
  // Only re-render if these specific props change
  return prevProps.text === nextProps.text &&
    prevProps.isStreaming === nextProps.isStreaming &&
    prevProps.showBlinker === nextProps.showBlinker &&
    prevProps.session?.id === nextProps.session?.id;
};

export interface InteractionMarkdownProps {
  text: string;
  session: ISession;
  getFileURL: (filename: string) => string;
  showBlinker?: boolean;
  isStreaming: boolean;
  onFilterDocument?: (docId: string) => void;
}

// Main component
const InteractionMarkdown: FC<InteractionMarkdownProps> = ({
  text,
  session,
  getFileURL = (filename) => '#',
  showBlinker = false,
  isStreaming = false,
  onFilterDocument,
}) => {
  const theme = useTheme()
  const [processedContent, setProcessedContent] = useState<string>('');
  const [citationData, setCitationData] = useState<{ excerpts: Excerpt[], isStreaming: boolean } | null>(null);

  useEffect(() => {
    if (!text) {
      setProcessedContent('');
      setCitationData(null);
      return;
    }

    // Process the message content
    let content: string;
    if (session) {
      const processor = new MessageProcessor(text, {
        session,
        getFileURL,
        showBlinker,
        isStreaming,
        onFilterDocument,
      });
      content = processor.process();

    // Extract citation data if present
    const citationPattern = /__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/;
    const citationDataMatch = content.match(citationPattern);
    if (citationDataMatch) {
      try {
        const citationDataJson = citationDataMatch[1];
        const data = JSON.parse(citationDataJson);
        setCitationData(data);
        // Replace using the same pattern
        content = content.replace(/__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/, '');
      } catch (error) {
        console.error('Error parsing citation data:', error);
        setCitationData(null);
      }
    } else {
        setCitationData(null);
      }
    } else {
      content = processBasicContent(text);
      setCitationData(null);
    }

    setProcessedContent(content);
  }, [text, session, getFileURL, showBlinker, isStreaming, onFilterDocument]);

  return (
    <>
      <Box
        sx={{
          '& pre': {
            // backgroundColor: theme.palette.mode === 'light' ? '#f0f0f0' : '#1e1e1e',
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
            padding: '0',
            borderRadius: '3px',
          },
          '& a': {
            color: theme.palette.mode === 'light' ? '#333' : '#bbb',
          },
          '& .blinker-class': {
            animation: `${blink} 1.2s step-end infinite`,
            marginLeft: '2px',
            color: theme.palette.mode === 'light' ? 'rgba(0, 0, 0, 0.7)' : 'rgba(255, 255, 255, 0.7)',
            fontWeight: 'normal',
            userSelect: 'none',
          },
          '& .doc-citation': {
            color: theme.palette.mode === 'light' ? '#333' : '#bbb',
            fontWeight: 'bold',
            cursor: 'pointer',
            textDecoration: 'none',
            '&:hover': {
              backgroundColor: 'rgba(88, 166, 255, 0.1)',
            }
          },
          display: 'flow-root',
        }}
      >
        {/* Render Citation component if we have data */}
        {citationData && citationData.excerpts && citationData.excerpts.length > 0 && (
          <Citation
            excerpts={citationData.excerpts}
            isStreaming={citationData.isStreaming}
            onFilterDocument={onFilterDocument}
          />
        )}
        <Markdown
          children={processedContent}
          remarkPlugins={[remarkGfm]}
          rehypePlugins={[rehypeRaw]}
          className="interactionMessage"
          components={{
            code(props) {
              const { children, className, node, ...rest } = props
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
    </>
  );
}

function processBasicContent(text: string): string {
  // Implement basic processing logic here
  return text;
}

// Export with React.memo to prevent unnecessary re-renders
export default React.memo(InteractionMarkdown);