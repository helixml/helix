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
import { ISession } from '../../types'
// Import the escapeRegExp helper from session.ts
import { escapeRegExp } from '../../utils/session'

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

interface MessageProcessorOptions {
  session: ISession;
  getFileURL: (filename: string) => string;
  showBlinker?: boolean;
  isStreaming?: boolean;
}

/**
 * Central message processor that handles all text formatting
 * Including document links, citations, thinking tags and blinkers
 */
class MessageProcessor {
  private session: ISession;
  private getFileURL: (filename: string) => string;
  private showBlinker: boolean;
  private isStreaming: boolean;
  private documentReferenceCounter: number = 0;
  
  // Placeholders for content we want to preserve
  private preservedContent: Map<string, string> = new Map();
  private placeholderCounter: number = 0;
  
  // Elements we want to preserve during processing
  private documentLinks: string[] = [];
  private groupLinks: string[] = [];
  private citations: string[] = [];
  private blinker: string | null = null;
  
  // Input/output content
  private message: string;
  private mainContent: string;
  private resultContent: string;
  
  constructor(message: string, options: MessageProcessorOptions) {
    this.message = message;
    this.session = options.session;
    this.getFileURL = options.getFileURL;
    this.showBlinker = options.showBlinker || false;
    this.isStreaming = options.isStreaming || false;
    this.mainContent = message;
    this.resultContent = message;
  }
  
  /**
   * Main processing function that handles all message formatting
   */
  process(): string {
    // Don't process empty messages
    if (!this.message || this.message.trim() === '') {
      return '';
    }
    
    console.debug(`Processing message for session ${this.session.id}`);
    
    // Process in specific order:
    this.extractCitations();
    this.processDocumentIDLinks();
    this.processGroupIDLinks();
    this.addBlinkerIfNeeded();
    this.sanitizeHTML();
    this.processThinkingTags();
    this.restorePreservedContent();
    
    return this.resultContent;
  }
  
  /**
   * Creates a unique placeholder for content we want to preserve
   */
  private createPlaceholder(content: string, type: string): string {
    const id = this.placeholderCounter++;
    const placeholder = `__${type}_PLACEHOLDER_${id}__`;
    this.preservedContent.set(placeholder, content);
    return placeholder;
  }
  
  /**
   * Extract citation blocks from message for special handling
   */
  private extractCitations(): void {
    // Check for XML style citation blocks
    const ragCitationRegex = /(?:---\s*)?\s*<excerpts>([\s\S]*?)<\/excerpts>\s*(?:---\s*)?$/;
    const ragMatch = this.message.match(ragCitationRegex);
    
    // Also check if the LLM directly output citation HTML (happens sometimes)
    const directCitationHtmlRegex = /<div\s+class=["']rag-citations-container["'][\s\S]*?<\/div>\s*<\/div>\s*<\/div>/;
    const directCitationMatch = this.message.match(directCitationHtmlRegex);
    
    let citationContent: string | null = null;
    
    if (directCitationMatch) {
      // If the LLM has directly output citation HTML, extract it
      console.debug(`Found direct citation HTML in message - extracting for separate processing`);
      citationContent = directCitationMatch[0];
      // Remove citation HTML from main content
      this.mainContent = this.message.replace(citationContent, '');
      this.resultContent = this.mainContent;
      
      // Preserve the citation HTML
      const placeholder = this.createPlaceholder(citationContent, 'CITATION');
      this.citations.push(citationContent);
    } else if (ragMatch) {
      console.debug(`Found RAG citation block in message - extracting for separate processing`);
      citationContent = ragMatch[0];
      const citationBody = ragMatch[1];
      
      // Remove citation block from main content to prevent document ID replacement in citations
      this.mainContent = this.message.replace(citationContent, '');
      this.resultContent = this.mainContent;
      
      // Process the citation content - format into HTML
      const formattedCitation = this.formatCitation(citationBody);
      if (formattedCitation) {
        this.citations.push(formattedCitation);
      } else {
        // If formatting failed, preserve the original citation
        this.citations.push(citationContent);
      }
    }
  }
  
  /**
   * Format citation XML into HTML
   */
  private formatCitation(citationBody: string): string | null {
    try {
      // First, escape any XML tags to prevent HTML parsing issues
      // This is especially important for malformed XML like in the example
      const escapedCitationsBody = citationBody.replace(
        /<(\/?)(?!(\/)?excerpt|document_id|snippet|\/document_id|\/snippet|\/excerpt)([^>]*)>/g, 
        (match: string, p1: string, p2: string, p3: string) => `&lt;${p1}${p3}&gt;`
      );
      
      // Parse the individual excerpts
      const excerptRegex = /<excerpt>([\s\S]*?)<\/excerpt>/g;
      let excerptMatch;
      type Excerpt = {
        docId: string, 
        snippet: string, 
        filename: string, 
        fileUrl: string
      };
      
      let excerpts: Excerpt[] = [];
      const document_ids = this.session.config.document_ids || {};
      const allNonTextFiles = this.getAllNonTextFiles();
      
      while ((excerptMatch = excerptRegex.exec(escapedCitationsBody)) !== null) {
        try {
          const excerptContent = excerptMatch[1];
          
          // Use robust patterns to extract document_id and snippet
          let docId = '';
          let snippet = '';
          
          // Extract document_id - handle various possible formats
          const docIdMatch = excerptContent.match(/<document_id>([\s\S]*?)<\/document_id>/);
          if (docIdMatch) {
            docId = docIdMatch[1].trim();
            
            // Special handling for cases where the LLM put an HTML link inside document_id
            if (docId.includes('<a') || docId.includes('href=')) {
              const linkDocIdMatch = docId.match(/\/([^\/]+?)\.pdf/) || 
                                  docId.match(/\/([^\/]+?)\.docx/) || 
                                  docId.match(/\/(app_[a-zA-Z0-9]+)/);
              
              if (linkDocIdMatch) {
                docId = linkDocIdMatch[1];
              } else {
                // If we can't extract a proper doc ID, generate a fallback ID
                docId = `doc_${excerpts.length + 1}`;
              }
            }
          } else {
            // If no document_id was found, create a fallback
            docId = `doc_${excerpts.length + 1}`;
          }
          
          // Extract snippet
          const snippetMatch = excerptContent.match(/<snippet>([\s\S]*?)<\/snippet>/);
          if (snippetMatch) {
            snippet = snippetMatch[1].trim();
            
            // Ensure we properly escape any HTML in the snippet
            snippet = snippet
              .replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;')
              .replace(/'/g, '&#039;');
            
            // Handle inline code with backticks
            snippet = snippet.replace(/`([^`]+)`/g, '<code>$1</code>');
          } else {
            snippet = "No content available";
          }
          
          // Find a matching document for this ID or URL
          let filename = "";
          let fileUrl = "";
          let fileFound = false;
          
          // First try direct document_id match
          for (const [fname, id] of Object.entries(document_ids)) {
            if (id === docId) {
              filename = fname.split('/').pop() || fname;
              
              // Create the file URL
              if (this.session.parent_app) {
                fileUrl = this.getFileURL(fname);
              } else {
                const baseFilename = fname.replace(/\.txt$/i, '');
                const sourceFilename = allNonTextFiles.find(f => f.indexOf(baseFilename) === 0);
                fileUrl = sourceFilename ? this.getFileURL(sourceFilename) : this.getFileURL(fname);
              }
              
              fileFound = true;
              break;
            }
          }
          
          // If no direct match was found, try to extract from the document_id if it contains a URL
          if (!fileFound && docId.includes('http')) {
            const urlFilenameMatch = docId.match(/\/([^\/]+\.[^\/\.]+)($|\?)/);
            if (urlFilenameMatch) {
              filename = urlFilenameMatch[1];
              fileUrl = docId;
              fileFound = true;
            }
          }
          
          // Add this excerpt to our processed array
          excerpts.push({
            docId,
            snippet,
            filename: filename || "Unknown document",
            fileUrl: fileUrl || "#"
          });
        } catch (error) {
          console.error("Error processing excerpt:", error);
        }
      }
      
      if (excerpts.length === 0) {
        return null;
      }
      
      // Convert the excerpts into a citation display
      let citationHtml = `<div class="rag-citations-container">
        <div class="rag-citations-header">Citations</div>
        <div class="rag-citations-list">`;
      
      // Add each excerpt as a citation item
      excerpts.forEach(excerpt => {
        // Determine icon based on file type
        let iconType = 'document';
        if (excerpt.filename) {
          const ext = excerpt.filename.split('.').pop()?.toLowerCase();
          if (ext === 'pdf') iconType = 'pdf';
          else if (['doc', 'docx'].includes(ext || '')) iconType = 'word';
          else if (['xls', 'xlsx'].includes(ext || '')) iconType = 'excel';
          else if (['ppt', 'pptx'].includes(ext || '')) iconType = 'powerpoint';
          else if (['jpg', 'jpeg', 'png', 'gif'].includes(ext || '')) iconType = 'image';
        }
        
        citationHtml += `
          <div class="rag-citation-item">
            <div class="rag-citation-icon ${iconType}-icon"></div>
            <div class="rag-citation-content">
              <div class="rag-citation-title">
                <a href="${excerpt.fileUrl}" target="_blank">${excerpt.filename}</a>
              </div>
              <div class="rag-citation-snippet">${excerpt.snippet}</div>
            </div>
          </div>`;
      });
      
      // Close the container
      citationHtml += `
        </div>
      </div>`;
      
      return citationHtml;
    } catch (error) {
      console.error("Error formatting citation:", error);
      return null;
    }
  }
  
  /**
   * Process document ID links in the message
   */
  private processDocumentIDLinks(): void {
    const document_ids = this.session.config.document_ids || {};
    this.documentReferenceCounter = 0;
    
    Object.keys(document_ids).forEach(filename => {
      const document_id = document_ids[filename];
      const escapedDocId = escapeRegExp(document_id);
      let searchPattern: RegExp | null = null;
      
      // Look for various document ID formats
      if (this.resultContent.indexOf(`[DOC_ID:${document_id}]`) >= 0) {
        searchPattern = new RegExp(`\\[DOC_ID:${escapedDocId}\\]`, 'g');
      } else if (this.resultContent.indexOf(`DOC_ID:${document_id}`) >= 0) {
        searchPattern = new RegExp(`\\[.*DOC_ID:${escapedDocId}.*?\\]`, 'g');
      } else if (this.resultContent.indexOf(document_id) >= 0) {
        searchPattern = new RegExp(`${escapedDocId}`, 'g');
      }
      
      if (!searchPattern) {
        return;
      }
      
      this.documentReferenceCounter++;
      
      // Create the document link
      let link: string;
      const allNonTextFiles = this.getAllNonTextFiles();
      
      if (this.session.parent_app) {
        // For app sessions
        const displayName = filename.split('/').pop() || filename;
        link = `<a target="_blank" style="color: white;" href="${this.getFileURL(filename)}" class="doc-link">[${this.documentReferenceCounter}]</a>`;
      } else {
        // Regular session - try to find the file in the interactions
        const baseFilename = filename.replace(/\.txt$/i, '');
        const sourceFilename = allNonTextFiles.find(f => f.indexOf(baseFilename) === 0);
        if (!sourceFilename) {
          link = `<a target="_blank" style="color: white;" href="${this.getFileURL(filename)}" class="doc-link">[${this.documentReferenceCounter}]</a>`;
        } else {
          link = `<a target="_blank" style="color: white;" href="${this.getFileURL(sourceFilename)}" class="doc-link">[${this.documentReferenceCounter}]</a>`;
        }
      }
      
      // Add to list of links to preserve and replace in the content
      this.documentLinks.push(link);
      this.resultContent = this.resultContent.replace(searchPattern, link);
    });
  }
  
  /**
   * Process document group ID links
   */
  private processGroupIDLinks(): void {
    const document_group_id = this.session.config.document_group_id || '';
    
    if (!document_group_id) {
      return;
    }
    
    const escapedGroupId = escapeRegExp(document_group_id);
    let groupSearchPattern: RegExp | null = null;
    
    if (this.resultContent.indexOf(`[DOC_GROUP:${document_group_id}]`) >= 0) {
      groupSearchPattern = new RegExp(`\\[DOC_GROUP:${escapedGroupId}\\]`, 'g');
    } else if (this.resultContent.indexOf(document_group_id) >= 0) {
      groupSearchPattern = new RegExp(`${escapedGroupId}`, 'g');
    }
    
    if (groupSearchPattern) {
      const link = `<a style="color: white;" href="javascript:_helixHighlightAllFiles()" class="doc-group-link">[group]</a>`;
      this.groupLinks.push(link);
      this.resultContent = this.resultContent.replace(groupSearchPattern, link);
    }
  }
  
  /**
   * Add the blinker to the message if needed
   */
  private addBlinkerIfNeeded(): void {
    if (!this.showBlinker) {
      return;
    }
    
    const blinkerHtml = `<span class="blinker-class">┃</span>`;
    
    // Check if content includes citations
    const hasCitation = 
      /<div class="rag-citations-container">/.test(this.resultContent) || 
      /&lt;div class="rag-citations-container"&gt;/.test(this.resultContent);
    
    if (hasCitation) {
      // Insert before citation container
      if (this.resultContent.includes('<div class="rag-citations-container">')) {
        this.resultContent = this.resultContent.replace(
          /<div class="rag-citations-container">/,
          `${blinkerHtml}<div class="rag-citations-container">`
        );
      } else {
        // Try with escaped version
        this.resultContent = this.resultContent.replace(
          /&lt;div class="rag-citations-container"&gt;/,
          `${blinkerHtml}&lt;div class="rag-citations-container"&gt;`
        );
      }
    } else {
      // No citation, append at the end
      this.resultContent += blinkerHtml;
    }
    
    this.blinker = blinkerHtml;
  }
  
  /**
   * Sanitize HTML content, escaping non-standard tags
   */
  private sanitizeHTML(): void {
    // Temporarily replace HTML we want to preserve with placeholders
    // First preserve all document links
    const allLinks = [...this.documentLinks, ...this.groupLinks];
    const linkPlaceholders: string[] = [];
    
    let tempContent = this.resultContent;
    
    // Preserve document and group links
    allLinks.forEach(link => {
      const placeholder = this.createPlaceholder(link, 'LINK');
      linkPlaceholders.push(placeholder);
      tempContent = tempContent.replace(link, placeholder);
    });
    
    // Preserve the blinker if present
    if (this.blinker) {
      const blinkerPlaceholder = this.createPlaceholder(this.blinker, 'BLINKER');
      tempContent = tempContent.replace(this.blinker, blinkerPlaceholder);
    }
    
    // Escape non-standard HTML tags
    this.resultContent = tempContent.replace(
      /<(\/?)(?!(\/)?a|span|div|code|pre|br|strong|em|ul|ol|li|p|h[1-6]|img|table|tr|td|th)([^>]+)>/g, 
      (match) => {
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
      }
    );
    
    // Restore the placeholders
    for (const [placeholder, content] of this.preservedContent.entries()) {
      this.resultContent = this.resultContent.replace(placeholder, content);
    }
    
    // Clear the placeholder map since we've restored everything
    this.preservedContent.clear();
  }
  
  /**
   * Process thinking tags in the message
   */
  private processThinkingTags(): void {
    // Fix code block indentation
    this.resultContent = this.resultContent.replace(/^\s*```/gm, '```');
    
    // Replace "---" with "</think>" if there's an unclosed think tag
    let openCount = 0;
    this.resultContent = this.resultContent.split('\n').map(line => {
      if (line.includes('<think>')) openCount++;
      if (line.includes('</think>')) openCount--;
      if (line.trim() === '---' && openCount > 0) {
        openCount--;
        return '</think>';
      }
      return line;
    }).join('\n');
    
    // Check if there's an unclosed think tag
    const openTagCount = (this.resultContent.match(/<think>/g) || []).length;
    const closeTagCount = (this.resultContent.match(/<\/think>/g) || []).length;
    const isThinking = openTagCount > closeTagCount;
    
    if (isThinking) {
      // If there's an unclosed tag, add the closing tag
      this.resultContent += '\n</think>';
    }
    
    // Convert <think> tags to styled divs, skipping empty ones
    this.resultContent = this.resultContent.replace(
      /<think>([\s\S]*?)<\/think>/g,
      (_, content) => {
        const trimmedContent = content.trim();
        if (!trimmedContent) return ''; // Skip empty think tags
        
        return `<div class="think-container${isThinking ? ' thinking' : ''}"><details${isThinking ? ' open' : ''}><summary class="think-header"><strong>Reasoning</strong></summary><div class="think-content">

${trimmedContent}

</div></details></div>`;
      }
    );
  }
  
  /**
   * Restore all preserved content with final formatting
   */
  private restorePreservedContent(): void {
    // Append citations at the end
    if (this.citations.length > 0) {
      this.resultContent += this.citations.join('\n');
    }
  }
  
  /**
   * Get all non-text files from the interactions
   */
  private getAllNonTextFiles(): string[] {
    return this.session.interactions.reduce((acc: string[], interaction) => {
      if (!interaction.files || interaction.files.length <= 0) return acc;
      return acc.concat(interaction.files.filter(f => f.match(/\.txt$/i) ? false : true));
    }, []);
  }
}

export interface InteractionMarkdownProps {
  text: string;
  session?: ISession;
  getFileURL?: (filename: string) => string;
  showBlinker?: boolean;
  isStreaming?: boolean;
}

export const InteractionMarkdown: FC<InteractionMarkdownProps> = ({
  text,
  session,
  getFileURL = (filename) => '#',
  showBlinker = false,
  isStreaming = false,
}) => {
  const theme = useTheme()
  if(!text) return null
  
  // Process the message content
  const processContent = (content: string): string => {
    // If we have session info, process with full functionality
    if (session) {
      console.debug(`Markdown: Processing message for session ${session.id} with MessageProcessor`);
      const processor = new MessageProcessor(content, {
        session,
        getFileURL,
        showBlinker,
        isStreaming
      });
      return processor.process();
    }
    
    console.debug(`Markdown: Processing message without session context (basic processing)`);
    // Basic processing without session-specific features
    return processBasicContent(content);
  };
  
  // Process content without session-specific features
  const processBasicContent = (content: string): string => {
    // Process think tags, code blocks, and sanitize HTML
    let processed = content;
    
    // Fix code block indentation
    processed = processed.replace(/^\s*```/gm, '```');
    
    // Process thinking tags with the same logic as above
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
    
    // Convert <think> tags to styled divs
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
    
    // Sanitize HTML - escape non-standard tags
    processed = processed.replace(
      /<(\/?)(?!(\/)?a|span|div|code|pre|br|strong|em|ul|ol|li|p|h[1-6]|img|table|tr|td|th)([^>]+)>/g, 
      (match) => {
        // If it's already an HTML entity, leave it alone
        if (match.startsWith('&lt;')) {
          return match;
        }
        // Convert to HTML entities
        return match.replace(/</g, '&lt;').replace(/>/g, '&gt;');
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
        // Document link styling
        '& .doc-link, & .doc-group-link': {
          color: theme.palette.mode === 'light' ? '#0366d6' : '#58a6ff',
          textDecoration: 'none',
          fontWeight: 'bold',
          padding: '2px 6px',
          borderRadius: '4px',
          backgroundColor: theme.palette.mode === 'light' ? 'rgba(0, 102, 204, 0.1)' : 'rgba(88, 166, 255, 0.1)',
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