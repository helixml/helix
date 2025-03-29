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
    validationStatus?: 'exact' | 'fuzzy' | 'failed';
    validationMessage?: string;
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
    
    // Process document IDs and convert to links
    processedMessage = this.processDocumentIds(processedMessage);
    
    // Process document group IDs and convert to links
    processedMessage = this.processDocumentGroupIds(processedMessage);
    
    // Process thinking tags
    processedMessage = this.processThinkingTags(processedMessage);
    
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
          excerpts: [],
          isStreaming: true
        };
        
        // Try to extract partial document ID and snippet
        const docIdMatch = partialExcerpts.match(/<document_id>(.*?)<\/document_id>/);
        const snippetMatch = partialExcerpts.match(/<snippet>([\s\S]*?)$/);
        
        if (docIdMatch && snippetMatch) {
          const docId = docIdMatch[1];
          const snippet = snippetMatch[1];
          let filename = "Loading...";
          let fileUrl = "#";
          
          // Try to find associated filename and URL for the document ID
          if (this.options.session.config?.document_ids) {
            const docIdsMap = this.options.session.config.document_ids;
            for (const fname in docIdsMap) {
              if (docIdsMap[fname] === docId) {
                // Extract just the basename from the path
                filename = fname.split('/').pop() || fname;
                
                // Check if fname is a URL
                const isURL = fname.startsWith('http://') || fname.startsWith('https://');
                
                // Use direct URL for web links, otherwise use filestore URL
                fileUrl = isURL ? fname : this.options.getFileURL(fname);
                break;
              }
            }
          }
          
          this.citationData.excerpts.push({
            docId,
            snippet,
            filename,
            fileUrl,
            isPartial: true
          });
        } else {
          // If we can't extract details, fall back to a generic loading state
          this.citationData.excerpts.push({
            docId: "loading",
            snippet: "Loading source information...",
            filename: "Loading...",
            fileUrl: "#",
            isPartial: true
          });
        }
        
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
      // Check for the newer nested <excerpt> tags format
      const excerptTags = match.match(/<excerpt>[\s\S]*?<\/excerpt>/g);
      
      if (excerptTags && excerptTags.length > 0) {
        // Process each individual excerpt
        for (const excerptTag of excerptTags) {
          this.processExcerptTag(excerptTag);
        }
      } else {
        // Handle the old format (direct children of <excerpts> tag)
        this.processExcerptTag(match);
      }
    }
    
    // Validate citations against RAG results if available
    if (this.citationData && this.options.session.config?.session_rag_results) {
      this.validateCitationsAgainstRagResults();
    }
    
    // Remove citation XML from the message
    return message.replace(citationRegex, '');
  }
  
  private processExcerptTag(excerptContent: string): void {
    const docIdMatch = excerptContent.match(/<document_id>(.*?)<\/document_id>/);
    const snippetMatch = excerptContent.match(/<snippet>([\s\S]*?)<\/snippet>/);
    
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
            
            // Check if fname is a URL
            const isURL = fname.startsWith('http://') || fname.startsWith('https://');
            
            // Use direct URL for web links, otherwise use filestore URL
            fileUrl = isURL ? fname : this.options.getFileURL(fname);
            break;
          }
        }
      }
      
      // Add to citation data
      if (this.citationData) {
        this.citationData.excerpts.push({
          docId,
          snippet,
          filename,
          fileUrl,
          isPartial: false
        });
      }
    }
  }

  private processDocumentIds(message: string): string {
    if (!this.options.session.config?.document_ids) {
      return message;
    }
    
    let processedMessage = message;
    const docIdsMap = this.options.session.config.document_ids;
    
    // Create reverse mapping from docId to filename
    const docIdToFilename: Record<string, string> = {};
    for (const [filename, docId] of Object.entries(docIdsMap)) {
      docIdToFilename[docId] = filename;
    }
    
    // Find all document ID references in the message
    const docIdPattern = /\[DOC_ID:([^\]]+)\]/g;
    const matches = [...processedMessage.matchAll(docIdPattern)];
    
    // Process document IDs in the order they appear in the message
    let docCounter = 1;
    
    // Create a map to associate docIds with citation numbers
    const citationMap: Record<string, number> = {};
    
    // Process each match in the order they appear in the message
    for (const match of matches) {
      const docId = match[1];
      const filename = docIdToFilename[docId];
      
      if (filename) {
        // Check if filename is a URL
        const isURL = filename.startsWith('http://') || filename.startsWith('https://');
        
        // Use direct URL for web links, otherwise use filestore URL
        const fileUrl = isURL ? filename : this.options.getFileURL(filename);
        
        // Only add to citation map if not already there
        if (!citationMap[docId]) {
          citationMap[docId] = docCounter++;
        }
        
        // Replace the document ID with a link
        const citation = citationMap[docId];
        const replacement = `<a target="_blank" href="${fileUrl}" class="doc-citation">[${citation}]</a>`;
        
        // Replace just this specific match
        processedMessage = processedMessage.replace(match[0], replacement);
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
      
      // Sort excerpts by citation number
      this.citationData.excerpts.sort((a, b) => {
        // Use the citation number if available
        if (a.citationNumber && b.citationNumber) {
          return a.citationNumber - b.citationNumber;
        }
        
        // If citation numbers not available for some excerpts,
        // keep original order by returning 0 (no change in sort order)
        return 0;
      });
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

  private validateCitationsAgainstRagResults(): void {
    if (!this.citationData || !this.options.session.config?.session_rag_results) {
      return;
    }
    
    const ragResults = this.options.session.config.session_rag_results;
    
    for (let i = 0; i < this.citationData.excerpts.length; i++) {
      const excerpt = this.citationData.excerpts[i];
      
      // Find all RAG results matching the document ID (can be multiple chunks)
      const matchingRagResults = ragResults.filter(r => r.document_id === excerpt.docId);
      
      if (matchingRagResults.length === 0) {
        // No matching RAG result found
        this.citationData.excerpts[i] = {
          ...excerpt,
          validationStatus: 'failed',
          validationMessage: 'No matching source document found in RAG results'
        };
        continue;
      }
      
      // Check each matching result to find the best validation status
      let bestValidationStatus: 'exact' | 'fuzzy' | 'failed' = 'failed';
      let bestValidationMessage = 'Citation not verified: text not found in source';
      let bestSimilarity = 0;
      
      // Clean the citation text for comparison
      const cleanSnippet = this.normalizeText(excerpt.snippet);
      
      // Check all chunks with this document_id
      for (const ragResult of matchingRagResults) {
        const cleanContent = this.normalizeText(ragResult.content);
        
        // Try exact match first
        if (cleanContent.includes(cleanSnippet)) {
          // Exact match found
          bestValidationStatus = 'exact';
          bestValidationMessage = 'Citation verified: exact match found in source';
          break; // Stop searching as we found an exact match
        }
        
        // If no exact match, try fuzzy match
        const similarity = this.calculateTextSimilarity(cleanSnippet, cleanContent);
        if (similarity > bestSimilarity) {
          bestSimilarity = similarity;
          
          // Update fuzzy status if similarity is high enough
          if (similarity > 0.7) {
            bestValidationStatus = 'fuzzy';
            bestValidationMessage = 'Citation partially verified: similar text found in source';
          }
        }
      }
      
      // After checking all chunks, assign the best validation status
      this.citationData.excerpts[i] = {
        ...excerpt,
        validationStatus: bestValidationStatus,
        validationMessage: bestValidationMessage
      };
    }
  }
  
  private normalizeText(text: string): string {
    return text
      .replace(/\s+/g, ' ') // Normalize whitespace
      .replace(/[^\w\s]/g, '') // Remove punctuation
      .toLowerCase()
      .trim();
  }
  
  private calculateTextSimilarity(str1: string, str2: string): number {
    if (str1.length > str2.length) {
      [str1, str2] = [str2, str1]; // Ensure str1 is the shorter string
    }
    
    if (str1.length < 10) {
      return 0; // Too short to be meaningful
    }
    
    const words1 = new Set(str1.split(/\s+/));
    
    let maxSimilarity = 0;
    
    for (let i = 0; i <= str2.length - str1.length; i += 10) { // Step by 10 chars for efficiency
      const windowEnd = Math.min(i + str1.length * 2, str2.length);
      const window = str2.substring(i, windowEnd);
      const words2 = new Set(window.split(/\s+/));
      
      const intersection = new Set([...words1].filter(x => words2.has(x)));
      const union = new Set([...words1, ...words2]);
      
      const similarity = intersection.size / union.size;
      maxSimilarity = Math.max(maxSimilarity, similarity);
      
      if (maxSimilarity > 0.9) break; // Early exit if we found a good match
    }
    
    return maxSimilarity;
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
            color: theme.palette.mode === 'light' ? '#333' : '#fff',
            backgroundColor: theme.palette.mode === 'light' ? '#f0f0f0' : '#333',
            padding: '0px 4px',
            borderRadius: '4px',
            fontWeight: 'bold',
            cursor: 'pointer',
            textDecoration: 'none',
            '&:hover': {
              backgroundColor: 'rgba(88, 166, 255, 0.3)',
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
            ragResults={session?.config?.session_rag_results || []}
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