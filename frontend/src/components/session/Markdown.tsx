import React, { FC, ReactElement, useState, useEffect } from 'react'
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

interface MessageProcessorOptions {
  session: ISession;
  getFileURL: (filename: string) => string;
  showBlinker?: boolean;
  isStreaming?: boolean;
  onFilterDocument?: (docId: string) => void;
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
  private onFilterDocument?: (docId: string) => void;

  // Placeholders for content we want to preserve
  private preservedContent: Map<string, string> = new Map();
  private placeholderCounter: number = 0;

  // Elements we want to preserve during processing
  private documentLinks: string[] = [];
  private groupLinks: string[] = [];
  private citations: Array<{
    excerpts: Excerpt[],
    placeholder?: string,
    tempMarker?: string,
    isStreaming?: boolean
  }> = [];
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
    this.onFilterDocument = options.onFilterDocument;
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
   * We identify and extract citations, then replace them with placeholders
   * to ensure they don't get processed by the sanitization steps
   */
  private extractCitations(): void {
    // Regular complete citation patterns
    const ragCitationRegex = /(?:---\s*)?\s*<excerpts>([\s\S]*?)<\/excerpts>\s*(?:---\s*)?$/;
    const directCitationHtmlRegex = /<div\s+class=["']rag-citations-container["'][\s\S]*?<\/div>\s*<\/div>\s*<\/div>/;

    // For streaming, also look for partial/incomplete citation patterns
    const partialExcerptsRegex = /<excerpts>([\s\S]*)$/;
    const partialCitationHtmlRegex = /<div\s+class=["']rag-citations-container["'][\s\S]*$/;

    // Try complete patterns first
    const ragMatch = this.message.match(ragCitationRegex);
    const directCitationMatch = this.message.match(directCitationHtmlRegex);

    // Then try partial patterns if streaming
    const partialRagMatch = this.isStreaming ? this.message.match(partialExcerptsRegex) : null;
    const partialCitationMatch = this.isStreaming ? this.message.match(partialCitationHtmlRegex) : null;

    let citationContent: string | null = null;
    let isPartial = false;

    // Handle complete citation HTML
    if (directCitationMatch) {
      console.debug(`Found complete direct citation HTML`);
      citationContent = directCitationMatch[0];
      this.mainContent = this.message.replace(citationContent, '');
      this.resultContent = this.mainContent;

      // Parse the HTML to extract excerpts
      const excerpts = this.parseDirectCitationHtml(citationContent);
      // Create a placeholder for the citation
      const placeholder = this.createPlaceholder(JSON.stringify(excerpts), 'CITATION');
      // Store the excerpts and placeholder
      this.citations.push({ excerpts, placeholder, isStreaming: false });
    }
    // Handle complete excerpts XML
    else if (ragMatch) {
      console.debug(`Found complete RAG citation block`);
      citationContent = ragMatch[0];
      const citationBody = ragMatch[1];

      this.mainContent = this.message.replace(citationContent, '');
      this.resultContent = this.mainContent;

      // Extract excerpts from XML
      const excerpts = this.extractExcerptsFromXml(citationBody, false);
      if (excerpts && excerpts.length > 0) {
        const placeholder = this.createPlaceholder(JSON.stringify(excerpts), 'CITATION');
        this.citations.push({ excerpts, placeholder, isStreaming: false });
      }
    }
    // Handle partial citation HTML during streaming
    else if (this.isStreaming && partialCitationMatch) {
      console.debug(`Found partial citation HTML during streaming`);
      citationContent = partialCitationMatch[0];
      isPartial = true;

      this.mainContent = this.message.replace(citationContent, '');
      this.resultContent = this.mainContent;

      // Create a placeholder loading excerpt
      const excerpts: Excerpt[] = [{
        docId: 'loading_1',
        snippet: 'Retrieving source information...',
        filename: 'Loading...',
        fileUrl: '#',
        isPartial: true
      }];
      const placeholder = this.createPlaceholder(JSON.stringify(excerpts), 'CITATION');
      this.citations.push({ excerpts, placeholder, isStreaming: true });
    }
    // Handle partial excerpts XML during streaming
    else if (this.isStreaming && partialRagMatch) {
      console.debug(`Found partial RAG citation block during streaming`);
      citationContent = partialRagMatch[0];
      const partialCitationBody = partialRagMatch[1];
      isPartial = true;

      this.mainContent = this.message.replace(citationContent, '');
      this.resultContent = this.mainContent;

      // Extract excerpts from the XML instead of formatted HTML
      const excerpts = this.extractExcerptsFromXml(partialCitationBody, true);
      if (excerpts && excerpts.length > 0) {
        // Create a placeholder for the excerpts
        const placeholder = this.createPlaceholder(JSON.stringify(excerpts), 'CITATION');
        // Store the excerpts and placeholder
        this.citations.push({ excerpts, placeholder, isStreaming: true });
      } else {
        // Fallback with a loading excerpt
        const loadingExcerpts: Excerpt[] = [{
          docId: "partial_1",
          snippet: "Retrieving source information...",
          filename: "Loading...",
          fileUrl: "#",
          isPartial: true
        }];
        const placeholder = this.createPlaceholder(JSON.stringify(loadingExcerpts), 'CITATION');
        this.citations.push({ excerpts: loadingExcerpts, placeholder, isStreaming: true });
      }
    }
  }

  /**
   * Parse direct citation HTML to extract excerpts
   * This is a helper method to extract data from HTML citations
   */
  private parseDirectCitationHtml(html: string): Excerpt[] {
    const excerpts: Excerpt[] = [];
    try {
      // Create a temporary DOM element to parse the HTML
      const tempDiv = document.createElement('div');
      tempDiv.innerHTML = html;

      // Find all citation items
      const citationItems = tempDiv.querySelectorAll('.citation-item');
      citationItems.forEach((item, index) => {
        const quoteElement = item.querySelector('.citation-quote');
        const sourceElement = item.querySelector('.citation-source a');

        if (quoteElement && sourceElement) {
          const snippet = quoteElement.textContent?.replace(/[""]/g, '') || '';
          const filename = sourceElement.textContent || '';
          const fileUrl = sourceElement.getAttribute('href') || '#';
          const isPartial = item.classList.contains('loading-item');

          excerpts.push({
            docId: `doc_${index + 1}`,
            snippet,
            filename,
            fileUrl,
            isPartial
          });
        }
      });
    } catch (error) {
      console.error('Error parsing citation HTML:', error);
    }

    // Return at least one excerpt even if parsing failed
    if (excerpts.length === 0) {
      excerpts.push({
        docId: 'error_1',
        snippet: 'Error parsing citation.',
        filename: 'Unknown source',
        fileUrl: '#',
        isPartial: false
      });
    }

    return excerpts;
  }

  /**
   * Extract excerpts from XML format
   * @param citationBody The XML citation content
   * @param isPartial Whether this is a partial citation during streaming
   */
  private extractExcerptsFromXml(citationBody: string, isPartial: boolean = false): Excerpt[] {
    try {
      // Escape any XML tags to prevent HTML parsing issues
      const escapedCitationsBody = citationBody.replace(
        /<(\/?)(?!(\/)?excerpt|document_id|snippet|\/document_id|\/snippet|\/excerpt)([^>]*)>/g,
        (match: string, p1: string, p2: string, p3: string) => `&lt;${p1}${p3}&gt;`
      );

      // Parse the individual excerpts - handle both complete and partial excerpts
      const excerptRegex = isPartial
        ? /<excerpt>([\s\S]*?)(?:<\/excerpt>|$)/g  // Either find closing tag or accept to end of string
        : /<excerpt>([\s\S]*?)<\/excerpt>/g;       // Only complete excerpts

      let excerptMatch;
      const document_ids = this.session.config.document_ids || {};
      const allNonTextFiles = this.getAllNonTextFiles();

      let excerpts: Excerpt[] = [];

      while ((excerptMatch = excerptRegex.exec(escapedCitationsBody)) !== null) {
        try {
          const excerptContent = excerptMatch[1];
          const excerptIsPartial = isPartial && !excerptContent.includes('</snippet>');

          // Use robust patterns to extract document_id and snippet
          let docId = '';
          let snippet = '';

          // Extract document_id - handle various possible formats
          const docIdRegex = isPartial
            ? /<document_id>([\s\S]*?)(?:<\/document_id>|$)/  // Accept partial document_id
            : /<document_id>([\s\S]*?)<\/document_id>/;       // Complete document_id only

          const docIdMatch = excerptContent.match(docIdRegex);
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

          // Extract snippet - handle partial snippets during streaming
          const snippetRegex = isPartial
            ? /<snippet>([\s\S]*?)(?:<\/snippet>|$)/  // Accept partial snippets
            : /<snippet>([\s\S]*?)<\/snippet>/;       // Complete snippets only

          const snippetMatch = excerptContent.match(snippetRegex);
          if (snippetMatch) {
            snippet = snippetMatch[1].trim();

            // For partial snippets in streaming, add an ellipsis indicator 
            // BUT NO HTML - we'll style it separately using CSS classes
            if (isPartial && !excerptContent.includes('</snippet>')) {
              snippet += ' ...';
            }

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
            snippet = isPartial ? "Loading..." : "No content available";
          }

          // Find a matching document for this ID or URL
          let filename = "";
          let fileUrl = "";
          let fileFound = false;

          // First try direct document_id match
          for (const [fname, id] of Object.entries(document_ids)) {
            if (id === docId) {
              // If the filename is a URL, use it directly
              if (fname.startsWith('http')) {
                fileUrl = fname; // Use the URL directly
                filename = fname.split('/').pop() || fname;
                fileFound = true;
                break;
              }

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

          // If still no match found, try to extract from the document_id if it contains a URL
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
            filename: isPartial && !fileFound ? "Loading..." : (filename || "Unknown document"),
            fileUrl: fileUrl || "#",
            isPartial: excerptIsPartial
          });
        } catch (error) {
          console.error("Error processing excerpt:", error);
          // If we're streaming, add a placeholder for the failed excerpt
          if (isPartial) {
            excerpts.push({
              docId: `partial_${excerpts.length + 1}`,
              snippet: "Loading citation data...",
              filename: "Loading...",
              fileUrl: "#",
              isPartial: true
            });
          }
        }
      }

      // If we have no excerpts but we're processing a partial citation, create a placeholder
      if (excerpts.length === 0 && isPartial) {
        excerpts.push({
          docId: "partial_1",
          snippet: "Loading citation data...",
          filename: "Loading...",
          fileUrl: "#",
          isPartial: true
        });
      }

      // If we still have no excerpts, return null to indicate failure
      if (excerpts.length === 0) {
        return [];
      }

      return excerpts;
    } catch (error) {
      console.error("Error extracting excerpts:", error);
      // Return a fallback excerpt
      if (isPartial) {
        return [{
          docId: "error_1",
          snippet: "Retrieving source information...",
          filename: "Loading...",
          fileUrl: "#",
          isPartial: true
        }];
      }
      return [];
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

      // If the filename is a URL, use it directly regardless of session type
      if (filename.startsWith('http')) {
        const displayName = filename.split('/').pop() || filename;
        link = `<a target="_blank" style="color: white;" href="${filename}" class="doc-link">[${this.documentReferenceCounter}]</a>`;
      } else if (this.session.parent_app) {
        // For app sessions with non-URL files
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

    // NEVER show blinker during streaming - the blinker should only appear after streaming is complete
    if (this.isStreaming) {
      console.debug('Hiding blinker because streaming is active');
      return;
    }

    // Also never show blinker when citations are present (even after streaming)
    if (this.citations.length > 0) {
      console.debug('Hiding blinker because citation content is present');
      return;
    }

    // Final check for any citation-related content
    if (this.resultContent.includes('<excerpts>') ||
      this.resultContent.includes('</excerpts>') ||
      this.resultContent.includes('rag-citations-container') ||
      this.resultContent.includes('citation-box') ||
      this.resultContent.match(/---\s*<excerpts>/)) {
      console.debug('Hiding blinker because citation content detected');
      return;
    }

    const blinkerHtml = `<span class="blinker-class">┃</span>`;

    // No citation, append at the end
    this.resultContent += blinkerHtml;
    this.blinker = blinkerHtml;
  }

  /**
   * Sanitize HTML content, preserving certain elements
   * This removes unsafe HTML while keeping our special elements intact
   */
  private sanitizeHTML(): void {
    // First, identify and handle citation delimiters (---)
    // These are often confused with horizontal rules in markdown
    if (this.isStreaming) {
      // Remove any standalone triple dashes, which are used as citation delimiters
      // but might be interpreted as horizontal rules
      this.resultContent = this.resultContent.replace(/^\s*---\s*$/gm, '');

      // Also handle cases where they might be at the beginning or end of excerpts blocks
      this.resultContent = this.resultContent.replace(/---\s*<excerpts>/g, '<excerpts>');
      this.resultContent = this.resultContent.replace(/<\/excerpts>\s*---/g, '</excerpts>');
    }

    // Find and temporarily replace citations to preserve them
    this.citations.forEach(citation => {
      if (citation.placeholder && this.resultContent.includes(citation.placeholder)) {
        // Instead of removing citations, mark them with a temporary marker
        // We'll restore them after sanitization
        const tempMarker = `PRESERVE_CITATION_${Math.random().toString(36).substring(2, 15)}`;
        this.resultContent = this.resultContent.replace(citation.placeholder, tempMarker);
        // Update the placeholder to the new temp marker
        citation.tempMarker = tempMarker;
      }
    });

    // During streaming, temporarily remove any standalone HR markdown that appears at the end
    // and might be part of an incomplete message
    if (this.isStreaming) {
      // Check for horizontal rules at the end of the content
      const lastLineHrRegex = /\n\s*(-{3,}|<hr\s*\/?>)\s*$/;
      if (lastLineHrRegex.test(this.resultContent)) {
        this.resultContent = this.resultContent.replace(lastLineHrRegex, '');
      }

      // Handle any triple-dash delimiters that might be interpreted as horizontal rules
      this.resultContent = this.resultContent.replace(/(?:^|\n)\s*---\s*(?:$|\n)/g, '\n\n');
    }

    // Continue with normal sanitization
    // Use DOMPurify to remove unsafe elements but keep our special ones
    this.resultContent = DOMPurify.sanitize(this.resultContent, {
      ALLOWED_TAGS: ['b', 'i', 'u', 'strong', 'em', 'code', 'pre', 'a', 'span', 'div', 'p',
        'ul', 'ol', 'li', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'details', 'summary',
        'blockquote', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'hr'],
      ALLOWED_ATTR: ['href', 'class', 'target', 'rel', 'id', 'style'],
      KEEP_CONTENT: true,
      RETURN_DOM: false,
      RETURN_DOM_FRAGMENT: false
    });

    // Re-add our temp markers for citations (they were sanitized out)
    this.citations.forEach(citation => {
      if (citation.tempMarker && citation.placeholder) {
        this.resultContent = this.resultContent.replace(citation.tempMarker, citation.placeholder);
      }
    });
  }

  /**
   * Process thinking tags in the message
   */
  private processThinkingTags(): void {
    // Fix code block indentation
    this.resultContent = this.resultContent.replace(/^\s*```/gm, '```');

    // Handle citation delimiters (---) that might be confused with horizontal rules
    // during streaming to prevent them from being rendered as <hr> tags
    if (this.isStreaming) {
      // Remove any standalone triple dashes that appear at the end, which
      // might be part of an incomplete citation or think tag delimiter
      const hrAtEndRegex = /\n-{3,}\s*$/;
      if (hrAtEndRegex.test(this.resultContent)) {
        this.resultContent = this.resultContent.replace(hrAtEndRegex, '');
      }

      // Convert citation-related triple dashes to spaces
      this.resultContent = this.resultContent.replace(/---\s*(?:<excerpts>|$)/g, ' <excerpts>');
      this.resultContent = this.resultContent.replace(/(?:<\/excerpts>)\s*---/g, '</excerpts> ');
    }

    // Replace "---" with "</think>" if there's an unclosed think tag
    let openCount = 0;
    this.resultContent = this.resultContent.split('\n').map(line => {
      if (line.includes('<think>')) openCount++;
      if (line.includes('</think>')) openCount--;
      if (line.trim() === '---' && openCount > 0) {
        openCount--;
        return '</think>';
      }
      // Ignore isolated horizontal rules during streaming if they appear at the end
      // as they might be part of an incomplete message
      if (this.isStreaming && line.trim().match(/^-{3,}$/) &&
        openCount === 0) {
        return ''; // Remove any isolated horizontal rules during streaming
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

    // After think tag processing, check for any remaining <hr> tags when streaming
    // and remove those at the end that might be incomplete
    if (this.isStreaming) {
      this.resultContent = this.resultContent.replace(/<hr\s*\/?>\s*$/g, '');
      // Also remove any <hr> tags that appear right before excerpts
      this.resultContent = this.resultContent.replace(/<hr\s*\/?>\s*<excerpts>/g, '<excerpts>');
    }
  }

  /**
   * Restore all preserved content with final formatting
   * This is the final step where we replace citation placeholders with the actual HTML
   */
  private restorePreservedContent(): void {
    // Prepare citation data for React component
    if (this.citations.length > 0) {
      console.debug(`Processing ${this.citations.length} citations for final display`);

      // Collect all excerpts from citations
      let allExcerpts: Excerpt[] = [];
      let isStreamingAny = false;

      this.citations.forEach(citation => {
        // Add excerpts to our collection
        if (citation.excerpts && citation.excerpts.length > 0) {
          allExcerpts = [...allExcerpts, ...citation.excerpts];
          if (citation.isStreaming) {
            isStreamingAny = true;
          }
        }

        // Remove the citation placeholder from the content
        if (citation.placeholder && this.resultContent.includes(citation.placeholder)) {
          this.resultContent = this.resultContent.replace(citation.placeholder, '');
        }
      });

      // Create citation data marker
      // This approach uses a special marker that will be recognized by the React component
      if (allExcerpts.length > 0) {
        const citationData = {
          excerpts: allExcerpts,
          isStreaming: isStreamingAny
        };
        // Store citation data in a special JSON format that can be recognized by the component
        const citationMarker = `__CITATION_DATA__${JSON.stringify(citationData)}__CITATION_DATA__`;
        // Add the marker at the beginning of the content
        this.resultContent = citationMarker + this.resultContent;
        console.debug('Citation data marker added to content');
      }
    }

    // Now restore any other preserved HTML elements (like doc links, etc.)
    this.preservedContent.forEach((html, placeholder) => {
      this.resultContent = this.resultContent.replace(placeholder, html);
    });
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
  onFilterDocument?: (docId: string) => void;
}

export const InteractionMarkdown: FC<InteractionMarkdownProps> = ({
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
      console.debug(`Markdown: Processing message for session ${session.id} with MessageProcessor`);
      const processor = new MessageProcessor(text, {
        session,
        getFileURL,
        showBlinker,
        isStreaming,
        onFilterDocument,
      });
      content = processor.process();
    } else {
      console.debug(`Markdown: Processing message without session context (basic processing)`);
      content = processBasicContent(text);
    }

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

    setProcessedContent(content);
  }, [text, session, getFileURL, showBlinker, isStreaming]);

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
    <>
      {/* Render Citation component if we have data */}
      {citationData && citationData.excerpts && citationData.excerpts.length > 0 && (
        <Citation
          excerpts={citationData.excerpts}
          isStreaming={citationData.isStreaming}
          onFilterDocument={onFilterDocument}
        />
      )}

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
          // Blinker styling
          '& .blinker-class': {
            animation: `${blink} 1.2s step-end infinite`,
            marginLeft: '2px',
            color: theme.palette.mode === 'light' ? 'rgba(0, 0, 0, 0.7)' : 'rgba(255, 255, 255, 0.7)',
            fontWeight: 'normal',
            userSelect: 'none',
          },
          '& .loading-text': {
            color: theme.palette.mode === 'light' ? '#777' : '#aaa',
            fontStyle: 'italic',
            animation: `${subtleBounce} 1.2s infinite ease-in-out`,
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
          // For better content flow
          '& .interactionMessage': {
            display: 'block',
            overflow: 'hidden', // Ensure container properly contains floated elements
            fontSize: '1rem',
            lineHeight: 1.6, // Improved line height for main content
            color: theme.palette.mode === 'light' ? '#333' : '#e0e0e0', // Better text readability
          },
          '& .interactionMessage > p': {
            marginBottom: '1.2em', // More spacing between paragraphs
            padding: '0 0.5em 0 0', // Add some right padding to text
          },
          '& .interactionMessage > p:first-of-type': {
            marginTop: '0.5em', // Add top margin to first paragraph
          },
          '& .interactionMessage > p:last-of-type': {
            marginBottom: '0.5em', // Less margin on last paragraph
          },
          '& .interactionMessage::after': {
            content: '""',
            display: 'table',
            clear: 'both',
          },
          // Add a bit of space when citations are present
          '& .interactionMessage .citation-box + p': {
            marginTop: '1em', // More space after citations
          },
          // Ensure proper spacing with code blocks
          '& .interactionMessage pre': {
            margin: '1.5em 0', // More space around code blocks
          },
          // Better spacing for lists
          '& .interactionMessage ul, & .interactionMessage ol': {
            paddingLeft: '2em',
            marginBottom: '1.2em',
          },
        }}
      >
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
    </>
  )
}

export default InteractionMarkdown