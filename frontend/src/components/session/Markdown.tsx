import React, {
  FC,
  ReactElement,
  useState,
  useEffect,
  useMemo,
  useCallback,
  useRef,
} from "react";
import { useTheme } from "@mui/material/styles";
import Box from "@mui/material/Box";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import { TypesSession } from "../../api/api";

import DOMPurify from "dompurify";

// Import the new Citation component
import Citation, { Excerpt } from "./Citation";
import StreamingIndicator from "./StreamingIndicator";
import ThinkingWidget from "./ThinkingWidget";

// Import chat stats collector for performance monitoring
import { getGlobalStatsCollector } from "./ChatStatsOverlay";
import { APP_MONO_FONT_FAMILY, TYPOGRAPHY } from "../../styles/typography";
import { getChatColors } from "./chatStyles";
import MarkdownTable from "./MarkdownTable";
import MarkdownCodeBlock from "./MarkdownCodeBlock";
import ChangedFileIcon from "./ChangedFileIcon";
import { parseWorkspaceFileReference } from "../common/workspaceFileReferences";

export interface MessageProcessorOptions {
  session: TypesSession;
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
    validationStatus?: "exact" | "fuzzy" | "failed";
    validationMessage?: string;
    showQuotes: boolean;
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

    // Process filter mentions
    processedMessage = this.processFilterMentions(processedMessage);

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

    // Note: Blinker is now rendered as a separate React component (StreamingIndicator)
    // instead of being injected into the markdown content. This fixes issues where
    // the blinker HTML would appear inside code blocks.

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
      if (this.options.isStreaming && message.includes("<excerpts>")) {
        // Find the content after the opening tag
        const partialExcerpts = message.split("<excerpts>")[1];

        // Initialize citation data for streaming
        this.citationData = {
          excerpts: [],
          isStreaming: true,
        };

        // Try to extract partial document ID and snippet
        const docIdMatch = partialExcerpts.match(
          /<document_id>(.*?)<\/document_id>/,
        );
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
                filename = fname.split("/").pop() || fname;

                // Check if fname is a URL
                const isURL =
                  fname.startsWith("http://") || fname.startsWith("https://");

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
            isPartial: true,
            showQuotes: false,
          });
        } else {
          // If we can't extract details, fall back to a generic loading state
          this.citationData.excerpts.push({
            docId: "loading",
            snippet: "Loading source information...",
            filename: "Loading...",
            fileUrl: "#",
            isPartial: true,
            showQuotes: false,
          });
        }

        // In streaming mode, remove the partial excerpts
        return message.split("<excerpts>")[0];
      }

      return message;
    }

    // Initialize citation data if not already done
    if (!this.citationData) {
      this.citationData = {
        excerpts: [],
        isStreaming:
          this.options.isStreaming && !message.includes("</excerpts>"),
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
    return message.replace(citationRegex, "");
  }

  private processExcerptTag(excerptContent: string): void {
    const docIdMatch = excerptContent.match(
      /<document_id>(.*?)<\/document_id>/,
    );
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
            filename = fname.split("/").pop() || fname;

            // Check if fname is a URL
            const isURL =
              fname.startsWith("http://") || fname.startsWith("https://");

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
          isPartial: false,
          showQuotes: false,
        });
      }
    }
  }

  private processFilterMentions(message: string): string {
    const filterPattern =
      /@filter\(\[DOC_NAME:([^\]]+)\]\[DOC_ID:([^\]]+)\]\)/g;
    const matches = [...message.matchAll(filterPattern)];

    let processedMessage = message;

    for (const match of matches) {
      const fullMatch = match[0];
      const docName = match[1];
      const docId = match[2];

      const basename = docName.split("/").pop() || docName;

      let fileUrl = "#";
      if (this.options.session.config?.document_ids) {
        const docIdsMap = this.options.session.config.document_ids;
        for (const fname in docIdsMap) {
          if (docIdsMap[fname] === docId) {
            const isURL =
              fname.startsWith("http://") || fname.startsWith("https://");
            fileUrl = isURL ? fname : this.options.getFileURL(fname);
            break;
          }
        }
      }

      // TODO: open a sidebar with the PDF itself on click
      const replacement = `<a href="#" class="filter-mention" title="Document ID: ${docId}">@${basename}</a>`;
      processedMessage = processedMessage.replace(fullMatch, replacement);
    }

    return processedMessage;
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
        const isURL =
          filename.startsWith("http://") || filename.startsWith("https://");

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
            citationNumber,
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
    const groupRegex = new RegExp(`\\b${groupId}\\b`, "g");

    // Replace group ID with link if it exists in the message
    if (message.match(groupRegex)) {
      return message.replace(
        groupRegex,
        `<a href="#" class="doc-group-link">[group]</a>`,
      );
    }

    return message;
  }

  private processThinkingTags(message: string): string {
    // Normalise <thinking> tags (emitted by Claude Code) to <think>
    message = message.replace(/<thinking>/g, "<think>").replace(/<\/thinking>/g, "</think>");

    // Check for any <think> tags
    if (!message.includes("<think>")) {
      return message;
    }

    // Fix code block indentation
    let processedMessage = message.replace(/^[ \t]*```/gm, "```");

    // Handle triple dash as think tag closing delimiter during streaming
    if (this.options.isStreaming) {
      // Replace --- with </think> if it's in a thinking block
      let openCount = 0;
      processedMessage = processedMessage
        .split("\n")
        .map((line) => {
          if (line.includes("<think>")) openCount++;
          if (line.includes("</think>")) openCount--;
          if (line.trim() === "---" && openCount > 0) {
            openCount--;
            return "</think>";
          }
          return line;
        })
        .join("\n");
    }

    // Check if there's an unclosed think tag
    const openTagCount = (processedMessage.match(/<think>/g) || []).length;
    const closeTagCount = (processedMessage.match(/<\/think>/g) || []).length;
    const isThinking = openTagCount > closeTagCount;

    // Add closing tag if needed and not streaming
    if (isThinking && !this.options.isStreaming) {
      processedMessage += "\n</think>";
    }

    // Collect all <think>...</think> sections
    const thinkSections: string[] = [];
    processedMessage = processedMessage.replace(
      /<think>([\s\S]*?)<\/think>/g,
      (_, content) => {
        const trimmedContent = content.trim();
        if (!trimmedContent) return "";
        thinkSections.push(trimmedContent);
        return "";
      },
    );

    // Handle unclosed thinking tags during streaming
    if (isThinking && this.options.isStreaming) {
      // Find the last unclosed <think> tag
      const lastThinkTagMatch = processedMessage.match(/<think>([\s\S]*)$/);
      if (lastThinkTagMatch) {
        const content = lastThinkTagMatch[1].trim();
        if (content) {
          thinkSections.push(content);
          processedMessage = processedMessage.replace(/<think>([\s\S]*)$/, "");
        }
      }
    }

    // If we found any think sections, insert a single marker with all joined by an empty line
    if (thinkSections.length > 0) {
      const joined = thinkSections.join("\n\n");
      processedMessage =
        `__THINKING_WIDGET__${joined}__THINKING_WIDGET__` + processedMessage;
    }

    return processedMessage;
  }

  private removeTrailingTripleDash(message: string): string {
    // Remove triple dash at the end of content during streaming
    return message.replace(/\n---\s*$/, "");
  }

  private sanitizeHtml(message: string): string {
    // Temporarily replace code blocks to protect them from sanitization
    const codeBlocks: string[] = [];
    let processedMessage = message.replace(
      /```(?:[\w]*)\n([\s\S]*?)```/g,
      (match, codeContent) => {
        codeBlocks.push(match);
        return `__CODE_BLOCK_${codeBlocks.length - 1}__`;
      },
    );

    // Also protect inline code spans
    const inlineCode: string[] = [];
    processedMessage = processedMessage.replace(/`([^`]+)`/g, (match) => {
      inlineCode.push(match);
      return `__INLINE_CODE_${inlineCode.length - 1}__`;
    });

    // Escape HTML-like tags that aren't in our allowlist BEFORE DOMPurify
    // This prevents malformed tags like <svg xmlns="... from breaking rendering
    const ALLOWED_TAG_NAMES = [
      "a",
      "p",
      "br",
      "strong",
      "em",
      "div",
      "span",
      "h1",
      "h2",
      "h3",
      "h4",
      "h5",
      "h6",
      "ul",
      "ol",
      "li",
      "code",
      "pre",
      "blockquote",
      "details",
      "summary",
      "table",
      "thead",
      "tbody",
      "tr",
      "th",
      "td",
    ];
    processedMessage = processedMessage.replace(
      /<(\/?)([\w-]+)/g,
      (match, slash, tagName) => {
        if (ALLOWED_TAG_NAMES.includes(tagName.toLowerCase())) {
          return match; // Keep allowed tags
        }
        return `&lt;${slash}${tagName}`; // Escape disallowed tags
      },
    );

    // Use DOMPurify to sanitize HTML while preserving safe tags and attributes
    processedMessage = DOMPurify.sanitize(processedMessage, {
      ALLOWED_TAGS: ALLOWED_TAG_NAMES,
      ALLOWED_ATTR: [
        "href",
        "target",
        "class",
        "style",
        "title",
        "id",
        "aria-hidden",
        "aria-label",
        "role",
      ],
      ADD_ATTR: ["target"],
    });

    // Restore inline code
    inlineCode.forEach((code, index) => {
      processedMessage = processedMessage.replace(
        `__INLINE_CODE_${index}__`,
        code,
      );
    });

    // Restore code blocks
    codeBlocks.forEach((codeBlock, index) => {
      processedMessage = processedMessage.replace(
        `__CODE_BLOCK_${index}__`,
        codeBlock,
      );
    });

    return processedMessage;
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
    if (
      !this.citationData ||
      !this.options.session.config?.session_rag_results
    ) {
      return;
    }

    const ragResults = this.options.session.config.session_rag_results;
    const imageExtensions = [
      ".png",
      ".jpg",
      ".jpeg",
      ".gif",
      ".webp",
      ".svg",
      ".bmp",
    ]; // Added image extensions list

    for (let i = 0; i < this.citationData.excerpts.length; i++) {
      const excerpt = this.citationData.excerpts[i];

      // --- Added check for image files ---
      const fileExtension = excerpt.filename
        .substring(excerpt.filename.lastIndexOf("."))
        .toLowerCase();
      if (imageExtensions.includes(fileExtension)) {
        // Skip validation for images
        this.citationData.excerpts[i] = {
          ...excerpt,
          validationStatus: undefined, // Explicitly set status to undefined or keep as is
          validationMessage: "Source is an image, validation skipped.",
          showQuotes: false, // Images don't have text snippets to quote
        };
        continue; // Move to the next excerpt
      }
      // --- End of added check ---

      // Find all RAG results matching the document ID (can be multiple chunks)
      const matchingRagResults = ragResults.filter(
        (r) => r.document_id === excerpt.docId,
      );

      if (matchingRagResults.length === 0) {
        // No matching RAG result found
        this.citationData.excerpts[i] = {
          ...excerpt,
          validationStatus: "failed",
          validationMessage: "No matching source document found in RAG results",
          showQuotes: false,
        };
        continue;
      }

      // Check each matching result to find the best validation status
      let bestValidationStatus: "exact" | "fuzzy" | "failed" = "failed";
      let bestValidationMessage =
        "Citation not verified: text not found in source";
      let bestSimilarity = 0;
      let showQuotes = false;

      // Clean the citation text for comparison
      const cleanSnippet = this.normalizeText(excerpt.snippet);
      const snippetWords = new Set(
        cleanSnippet.split(/\s+/).filter((word) => word.length > 3),
      );

      // Check all chunks with this document_id
      for (const ragResult of matchingRagResults) {
        const cleanContent = this.normalizeText(ragResult?.content || "");

        // Try exact match first (whole text contains)
        if (cleanContent.includes(cleanSnippet)) {
          // Exact match found
          bestValidationStatus = "exact";
          bestValidationMessage =
            "Citation verified: exact match found in source";
          showQuotes = true;
          break; // Stop searching as we found an exact match
        }

        // If no exact match, try word-based similarity
        const contentWords = cleanContent
          .split(/\s+/)
          .filter((word) => word.length > 3);
        const matchedWords = Array.from(snippetWords).filter((word) =>
          contentWords.some(
            (contentWord) =>
              contentWord.includes(word) || word.includes(contentWord),
          ),
        );

        const wordSimilarity =
          snippetWords.size > 0 ? matchedWords.length / snippetWords.size : 0;

        // Try character-based similarity as fallback
        const similarity = this.calculateTextSimilarity(
          cleanSnippet,
          cleanContent,
        );

        // Use the better of word-based or character-based similarity
        const combinedSimilarity = Math.max(wordSimilarity, similarity);

        if (combinedSimilarity > bestSimilarity) {
          bestSimilarity = combinedSimilarity;

          // Update fuzzy status if similarity is high enough
          // Lower threshold slightly from 0.7 to 0.6 to better handle these cases
          if (combinedSimilarity > 0.6) {
            bestValidationStatus = "fuzzy";
            bestValidationMessage =
              "Citation partially verified: similar text found in source";
            showQuotes = false; // Don't show quotes for fuzzy matches
          }
        }
      }

      // After checking all chunks, assign the best validation status
      this.citationData.excerpts[i] = {
        ...excerpt,
        validationStatus: bestValidationStatus,
        validationMessage: bestValidationMessage,
        showQuotes: showQuotes,
      };
    }
  }

  private normalizeText(text: string): string {
    return text
      .replace(/[\r\n]+/g, " ") // Replace newlines with spaces
      .replace(/#/g, " ") // Replace # with spaces
      .replace(/\s+/g, " ") // Normalize all whitespace
      .replace(/[^\w\s]/g, "") // Remove punctuation
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

    for (let i = 0; i <= str2.length - str1.length; i += 10) {
      // Step by 10 chars for efficiency
      const windowEnd = Math.min(i + str1.length * 2, str2.length);
      const window = str2.substring(i, windowEnd);
      const words2 = new Set(window.split(/\s+/));

      const intersection = new Set([...words1].filter((x) => words2.has(x)));
      const union = new Set([...words1, ...words2]);

      const similarity = intersection.size / union.size;
      maxSimilarity = Math.max(maxSimilarity, similarity);

      if (maxSimilarity > 0.9) break; // Early exit if we found a good match
    }

    return maxSimilarity;
  }
}

export interface InteractionMarkdownProps {
  text: string;
  session: TypesSession;
  getFileURL: (filename: string) => string;
  showBlinker?: boolean;
  isStreaming: boolean;
  onFilterDocument?: (docId: string) => void;
  compactThinking?: boolean;
  renderThinkingWidget?: boolean;
  renderContent?: boolean;
}

// Throttle interval for streaming updates (ms)
const STREAMING_THROTTLE_MS = 150;

// Helper to count code blocks in content
const countCodeBlocks = (text: string): number => {
  const matches = text.match(/```/g);
  return matches ? Math.floor(matches.length / 2) : 0;
};

// Main component
const InteractionMarkdown: FC<InteractionMarkdownProps> = ({
  text,
  session,
  getFileURL = (filename) => "#",
  showBlinker = false,
  isStreaming = false,
  onFilterDocument,
  compactThinking = false,
  renderThinkingWidget = true,
  renderContent = true,
}) => {
  const theme = useTheme();
  const chatColors = getChatColors(theme);
  const [processedContent, setProcessedContent] = useState<string>("");
  const [citationData, setCitationData] = useState<{
    excerpts: Excerpt[];
    isStreaming: boolean;
  } | null>(null);
  const [thinkingWidgetContent, setThinkingWidgetContent] = useState<
    string | null
  >(null);

  // Throttling refs for streaming performance
  const throttleTimeoutRef = React.useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const lastProcessedTextRef = React.useRef<string>("");
  const pendingTextRef = React.useRef<string>("");

  // Track render timing for stats
  const renderStartTimeRef = useRef<number>(0);

  // Process content helper
  const processContent = useCallback(
    (textToProcess: string) => {
      if (!textToProcess) {
        setProcessedContent("");
        setCitationData(null);
        setThinkingWidgetContent(null);
        return;
      }

      // Start timing MessageProcessor
      const processStartTime = performance.now();

      let content: string;
      if (session) {
        const processor = new MessageProcessor(textToProcess, {
          session,
          getFileURL,
          showBlinker,
          isStreaming,
          onFilterDocument,
        });
        content = processor.process();

        // Record MessageProcessor timing
        const processEndTime = performance.now();
        const processTime = processEndTime - processStartTime;
        const statsCollector = getGlobalStatsCollector();
        statsCollector.recordProcessTime(processTime);
        statsCollector.recordUpdate(
          textToProcess.length,
          countCodeBlocks(textToProcess),
        );
        // Extract citation data if present
        const citationPattern = /__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/;
        const citationDataMatch = content.match(citationPattern);
        if (citationDataMatch) {
          try {
            const citationDataJson = citationDataMatch[1];
            const data = JSON.parse(citationDataJson);
            setCitationData(data);
            content = content.replace(
              /__CITATION_DATA__([\s\S]*?)__CITATION_DATA__/,
              "",
            );
          } catch (error) {
            console.error("Error parsing citation data:", error);
            setCitationData(null);
          }
        } else {
          setCitationData(null);
        }
        // Extract thinking widget content if present
        const thinkingPattern =
          /__THINKING_WIDGET__([\s\S]*?)__THINKING_WIDGET__/;
        const thinkingMatch = content.match(thinkingPattern);
        if (thinkingMatch) {
          setThinkingWidgetContent(thinkingMatch[1]);
          content = content.replace(thinkingPattern, "");
        } else {
          setThinkingWidgetContent(null);
        }
      } else {
        content = processBasicContent(textToProcess);
        setCitationData(null);
        setThinkingWidgetContent(null);
      }
      setProcessedContent(content);
      lastProcessedTextRef.current = textToProcess;

      // Record render start time (actual render timing measured in useEffect after state update)
      renderStartTimeRef.current = performance.now();
    },
    [session, getFileURL, showBlinker, isStreaming, onFilterDocument],
  );

  // Track render completion timing
  useEffect(() => {
    if (renderStartTimeRef.current > 0 && processedContent) {
      // Use requestAnimationFrame to measure after React has committed the update
      requestAnimationFrame(() => {
        const renderTime = performance.now() - renderStartTimeRef.current;
        const statsCollector = getGlobalStatsCollector();
        statsCollector.recordRenderTime(renderTime);
        renderStartTimeRef.current = 0;
      });
    }
  }, [processedContent]);

  // Track streaming status changes
  useEffect(() => {
    const statsCollector = getGlobalStatsCollector();
    statsCollector.setStreamingStatus(isStreaming);
    statsCollector.setThrottleStatus(isStreaming, STREAMING_THROTTLE_MS);
  }, [isStreaming]);

  useEffect(() => {
    // Store the latest text
    pendingTextRef.current = text;

    // If not streaming, process immediately
    if (!isStreaming) {
      // Clear any pending throttle
      if (throttleTimeoutRef.current) {
        clearTimeout(throttleTimeoutRef.current);
        throttleTimeoutRef.current = null;
      }
      processContent(text);
      return;
    }

    // During streaming, throttle updates
    // If no pending timeout, process immediately and start throttle window
    if (!throttleTimeoutRef.current) {
      processContent(text);
      throttleTimeoutRef.current = setTimeout(() => {
        throttleTimeoutRef.current = null;
        // Process any pending text that arrived during throttle window
        if (pendingTextRef.current !== lastProcessedTextRef.current) {
          processContent(pendingTextRef.current);
        }
      }, STREAMING_THROTTLE_MS);
    } else {
      // Record that we're skipping this update due to throttling
      const statsCollector = getGlobalStatsCollector();
      statsCollector.recordThrottleSkip();
    }
    // If there's already a pending timeout, the text will be processed when it fires

    return () => {
      if (throttleTimeoutRef.current) {
        clearTimeout(throttleTimeoutRef.current);
        throttleTimeoutRef.current = null;
      }
    };
  }, [text, isStreaming, processContent]);

  const hasVisibleContent =
    (renderContent && processedContent.trim().length > 0) ||
    (renderThinkingWidget && Boolean(thinkingWidgetContent?.trim())) ||
    Boolean(citationData?.excerpts?.length);

  return (
    <>
      <Box
        data-chat-markdown
        data-chat-markdown-visible={hasVisibleContent ? "true" : undefined}
        sx={{
          fontSize: TYPOGRAPHY.chatFontSize,
          lineHeight: TYPOGRAPHY.chatLineHeight,
          "& .interactionMessage > *": {
            marginTop: 0,
            marginBottom: 0,
          },
          "& .interactionMessage > * + *": {
            marginTop: 1.75,
          },
          // Empty response entries remain in the DOM, but must not receive
          // spacing of their own.
          "& + [data-chat-markdown][data-chat-markdown-visible='true']": {
            marginTop: 1.75,
          },
          "& pre": {
            padding: "1em",
            borderRadius: "4px",
            // Only allow horizontal scroll (for wide code)
            // Vertical scroll must go to parent container to prevent "getting stuck"
            // when scrolling the chat and momentum stops inside a code block
            overflowX: "auto",
            overflowY: "visible",
            position: "relative",
          },
          "& code": {
            backgroundColor: "transparent",
            fontSize: TYPOGRAPHY.chatFontSize,
            fontFamily: APP_MONO_FONT_FAMILY,
          },
          // Highlighted code owns its own surface — the block chrome lives on
          // the wrapper, so the inner pre/code stay flat and unpadded.
          "& [data-chat-code-block] pre": {
            margin: 0,
            border: "none",
            borderRadius: 0,
            background: "transparent",
          },
          "& pre > code": {
            border: "none",
            background: "transparent",
            padding: 0,
            fontSize: "inherit",
          },
          "& :not(pre) > code": {
            backgroundColor: chatColors.inlineCodeSurface,
            color: chatColors.inlineCodeForeground,
            border: `1px solid ${chatColors.inlineCodeBorder}`,
            padding: "0.1rem 0.35rem",
            borderRadius: "0.375rem",
            fontSize: TYPOGRAPHY.inlineCodeFontSize,
          },
          "& a": {
            color: theme.palette.mode === "light" ? "#333" : "inherit",
          },

          "& .doc-citation": {
            color: theme.palette.mode === "light" ? "#333" : "#fff",
            backgroundColor:
              theme.palette.mode === "light" ? "#f0f0f0" : "#333",
            padding: "0px 4px",
            borderRadius: "4px",
            fontWeight: "bold",
            cursor: "pointer",
            textDecoration: "none",
            "&:hover": {
              backgroundColor: "rgba(88, 166, 255, 0.3)",
            },
          },
          "& .filter-mention": {
            color: theme.palette.mode === "light" ? "#1976d2" : "#58a6ff",
            backgroundColor:
              theme.palette.mode === "light"
                ? "rgba(25, 118, 210, 0.08)"
                : "rgba(88, 166, 255, 0.15)",
            padding: "2px 6px",
            borderRadius: "4px",
            fontWeight: 500,
            cursor: "pointer",
            textDecoration: "none",
            transition: "all 0.2s ease",
            "&:hover": {
              backgroundColor:
                theme.palette.mode === "light"
                  ? "rgba(25, 118, 210, 0.15)"
                  : "rgba(88, 166, 255, 0.25)",
              textDecoration: "none",
            },
          },
          "& .chat-markdown-table-container": {
            maxWidth: "100%",
            margin: "1rem 0",
          },
          "& .chat-markdown-table-scroll": {
            maxWidth: "100%",
            overflowX: "auto",
            scrollbarWidth: "thin",
          },
          "& .chat-markdown-table-footer": {
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            minHeight: "24px",
            marginTop: "0.5rem",
          },
          "& .chat-markdown-table-action": {
            width: "24px",
            height: "24px",
            borderRadius: "6px",
            color: chatColors.subtle,
            "&:hover": {
              color: chatColors.foreground,
              backgroundColor: theme.palette.mode === "dark"
                ? "rgba(255, 255, 255, 0.06)"
                : "rgba(0, 0, 0, 0.05)",
            },
          },
          "& table": {
            borderCollapse: "collapse",
            width: "100%",
            minWidth: "max-content",
            margin: 0,
            overflowWrap: "normal",
            wordBreak: "normal",
            fontSize: "0.75rem",
          },
          "& th, & td": {
            padding: "0.45rem 0.75rem",
            textAlign: "left",
            border: 0,
          },
          "& thead th": {
            borderBottom: `1px solid ${chatColors.tableDivider}`,
            paddingTop: "0.55rem",
            paddingBottom: "0.55rem",
            fontWeight: "600",
            whiteSpace: "nowrap",
          },
          "& tbody td": {
            borderBottom: `1px solid ${chatColors.tableDivider}`,
          },
          "& .chat-markdown-table-container[data-expanded='false'] th, & .chat-markdown-table-container[data-expanded='false'] td": {
            overflow: "hidden",
            maxWidth: "24rem",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          },
          "& .chat-markdown-table-container[data-expanded='true'] td": {
            maxWidth: "24rem",
            overflowWrap: "anywhere",
          },
          display: "flow-root",
        }}
      >
        {renderThinkingWidget && thinkingWidgetContent && (
          <ThinkingWidget
            text={thinkingWidgetContent}
            isStreaming={isStreaming}
            compact={compactThinking}
          />
        )}
        {citationData &&
          citationData.excerpts &&
          citationData.excerpts.length > 0 && (
            <Citation
              excerpts={citationData.excerpts}
              isStreaming={citationData.isStreaming}
              onFilterDocument={onFilterDocument}
              ragResults={session?.config?.session_rag_results || []}
            />
          )}
        {renderContent && (
          <MemoizedMarkdownRenderer processedContent={processedContent} />
        )}
        {showBlinker && isStreaming && <StreamingIndicator />}
      </Box>
    </>
  );
};

/**
 * Memoized markdown renderer component.
 * Extracts the Markdown rendering to a separate component so it can be memoized
 * and avoid recreating the components object on every parent render.
 */
const MemoizedMarkdownRenderer: FC<{ processedContent: string }> = React.memo(
  ({ processedContent }) => {
    // Memoize the components object to prevent react-markdown from re-rendering
    // all code blocks when content changes. This is created once per component instance.
    const markdownComponents = useMemo(
      () => ({
        code(props: any) {
          const { children, className, node, ref, ...rest } = props;
          return (
            <code {...rest} className={className}>
              {children}
            </code>
          );
        },
        pre(props: any) {
          const { children, node, ref, ...rest } = props;
          const childNodes = React.Children.toArray(children);
          const child = childNodes.length === 1 ? childNodes[0] : null;
          if (!React.isValidElement<{ children?: React.ReactNode; className?: string }>(child)) {
            return <pre {...rest}>{children}</pre>;
          }
          const className = child.props.className || "";
          const language = /(?:^|\s)language-([^\s]+)/.exec(className)?.[1] || "text";
          return (
            <MarkdownCodeBlock language={language}>
              {String(child.props.children ?? "")}
            </MarkdownCodeBlock>
          );
        },
        a(props: any) {
          const { node, href, target, children, ...rest } = props;
          const label = React.Children.toArray(children)
            .filter((child): child is string | number => typeof child === "string" || typeof child === "number")
            .join("");
          const workspacePath = parseWorkspaceFileReference(href, label);
          if (workspacePath) {
            return <WorkspaceFileReference path={workspacePath} label={label} />;
          }
          // Internal action links (filter mentions, doc-group links) use
          // href="#" and have JS click handlers — leave them alone. Same
          // for in-page anchors like [Top](#section).
          if (!href || href === "#" || href.startsWith("#")) {
            return (
              <a href={href} {...rest}>
                {children}
              </a>
            );
          }
          // Respect an explicit target already set on the source HTML
          // (e.g. doc-citation links from processDocumentIds).
          if (target) {
            return (
              <a href={href} target={target} {...rest}>
                {children}
              </a>
            );
          }
          return (
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              {...rest}
            >
              {children}
            </a>
          );
        },
        table(props: any) {
          const { node, children, ...rest } = props;
          return (
            <MarkdownTable {...rest}>{children}</MarkdownTable>
          );
        },
      }),
      [],
    );

    // Memoize plugins arrays to prevent unnecessary re-renders
    const remarkPluginsArray = useMemo(() => [remarkGfm], []);
    const rehypePluginsArray = useMemo(() => [rehypeRaw], []);

    return (
      <Markdown
        children={processedContent}
        remarkPlugins={remarkPluginsArray}
        rehypePlugins={rehypePluginsArray}
        className="interactionMessage"
        components={markdownComponents}
      />
    );
  },
);

MemoizedMarkdownRenderer.displayName = "MemoizedMarkdownRenderer";

const WorkspaceFileReference: FC<{ path: string; label: string }> = ({ path, label }) => {
  const theme = useTheme();
  const colors = getChatColors(theme);
  return (
    <Box
      component="span"
      title={path}
      data-workspace-file-reference={path}
      sx={{
        display: "inline-flex",
        alignItems: "center",
        gap: 0.5,
        maxWidth: "100%",
        px: 0.625,
        py: "1px",
        border: `1px solid ${colors.inlineCodeBorder}`,
        borderRadius: "5px",
        bgcolor: colors.inlineCodeSurface,
        color: colors.inlineCodeForeground,
        fontFamily: APP_MONO_FONT_FAMILY,
        fontSize: TYPOGRAPHY.inlineCodeFontSize,
        lineHeight: 1.5,
        verticalAlign: "text-bottom",
      }}
    >
      <ChangedFileIcon path={path} darkMode={theme.palette.mode === "dark"} size={13} />
      <Box component="span" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
        {label}
      </Box>
    </Box>
  );
};

function processBasicContent(text: string): string {
  // Implement basic processing logic here
  return text;
}

// Export with React.memo to prevent unnecessary re-renders
export default React.memo(InteractionMarkdown);
