import React, { useMemo, useRef, useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Typography,
  Box,
  Paper,
  Divider,
  useTheme,
  Chip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { TypesSessionRAGResult } from '../../api/api';

interface CitationComparisonModalProps {
  open: boolean;
  onClose: () => void;
  citation: {
    docId: string;
    snippet: string;
    validationStatus: 'exact' | 'fuzzy' | 'failed';
    fileUrl?: string;
  };
  ragResults: TypesSessionRAGResult[];
}

// Helper function to create a content-based key for RAG results
const createContentKey = (result: TypesSessionRAGResult): string => {
  // Start with document_id and hash of content
  let key = `${result.document_id}-${hashString(result.content || '')}`;

  // Add chunk identification if available in metadata
  if (result.metadata) {
    if (result.metadata.chunk_id) {
      key += `-chunk-${result.metadata.chunk_id}`;
    } else if (result.metadata.offset) {
      key += `-offset-${result.metadata.offset}`;
    }
  }

  return key;
};

// Simple string hash function
const hashString = (str: string): number => {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32bit integer
  }
  return Math.abs(hash);
};

// Calculate content similarity between two strings
const calculateSimilarity = (str1: string, str2: string): number => {
  // Normalize both strings - more aggressive normalization to handle formatting differences
  const normalize = (text: string): string => {
    return text
      .replace(/[\r\n]+/g, ' ') // Replace newlines with spaces
      .replace(/#/g, ' ')       // Replace # with spaces
      .replace(/\s+/g, ' ')     // Normalize all whitespace
      .replace(/[^\w\s]/g, '')  // Remove punctuation
      .toLowerCase()
      .trim();
  };

  const normalized1 = normalize(str1);
  const normalized2 = normalize(str2);

  // For very short strings, use a different approach
  if (normalized1.length < 10 || normalized2.length < 10) {
    return normalized1 === normalized2 ? 1.0 : 0.0;
  }

  // Check if one string contains the other
  if (normalized1.includes(normalized2)) return 0.9;
  if (normalized2.includes(normalized1)) return 0.9;

  // For longer strings, use both word-based and Jaccard similarity
  const words1 = normalized1.split(/\s+/).filter(w => w.length > 3);
  const words2 = normalized2.split(/\s+/).filter(w => w.length > 3);

  // Word-level match (more important for citations)
  const matchingWords = words1.filter(word =>
    words2.some(w2 => w2.includes(word) || word.includes(w2))
  );

  const wordSimilarity = words1.length > 0 ? matchingWords.length / words1.length : 0;

  // Standard Jaccard similarity as a backup
  const wordSet1 = new Set(words1);
  const wordSet2 = new Set(words2);

  const intersection = new Set([...wordSet1].filter(x => wordSet2.has(x)));
  const union = new Set([...wordSet1, ...wordSet2]);

  const jaccardSimilarity = union.size > 0 ? intersection.size / union.size : 0;

  // Use the better of the two similarity measures
  return Math.max(wordSimilarity, jaccardSimilarity);
};

const CitationComparisonModal: React.FC<CitationComparisonModalProps> = ({
  open,
  onClose,
  citation,
  ragResults
}) => {
  const theme = useTheme();
  const bestMatchRef = useRef<HTMLDivElement>(null);
  const leftPanelRef = useRef<HTMLDivElement>(null);
  const highlightedTextRef = useRef<HTMLSpanElement>(null);
  const [highlight, setHighlight] = useState(false);
  const [isFullyOpen, setIsFullyOpen] = useState(false);

  // Process and organize RAG results
  const { matchingResults, otherResults, bestMatchKey } = useMemo(() => {
    // Filter results that match the document ID
    const docMatches = ragResults.filter(r => r.document_id === citation.docId);

    // Find content matches and calculate similarity scores
    const resultScores = docMatches.map(result => {
      const contentKey = createContentKey(result);
      const similarity = calculateSimilarity(result.content || '', citation.snippet);
      return { result, contentKey, similarity };
    });

    // Sort by similarity score (highest first)
    resultScores.sort((a, b) => b.similarity - a.similarity);

    // Get the best match and all other results
    const bestMatch = resultScores.length > 0 ? resultScores[0] : null;
    const bestMatchKey = bestMatch?.contentKey || '';

    // Separate into matching results (with this document_id) and other results
    // Sort matching results so the best match appears first
    const matchingResults = resultScores.map(score => score.result);
    const otherResults = ragResults.filter(r => r.document_id !== citation.docId);

    return { matchingResults, otherResults, bestMatchKey };
  }, [ragResults, citation]);

  // Function to highlight the matched text in the RAG result content
  const highlightMatchedText = (content: string, citationText: string, isMatch: boolean): React.ReactNode => {
    if (citation.validationStatus === 'failed' || !isMatch) {
      return <Typography sx={{ whiteSpace: 'pre-wrap' }}>{content}</Typography>;
    }

    // For exact or fuzzy match, try to find and highlight the quoted text
    try {
      // Simple case-insensitive search
      // This is a more direct approach that handles special characters better
      const contentLower = content.toLowerCase();
      const citationLower = citationText.toLowerCase();

      // Try to find the citation text in the content
      const index = contentLower.indexOf(citationLower);

      if (index >= 0 && (citation.validationStatus === 'exact' || citation.validationStatus === 'fuzzy')) {
        // We found the citation text - use the original casing from content
        const beforeMatch = content.substring(0, index);
        const matchedText = content.substring(index, index + citationText.length);
        const afterMatch = content.substring(index + citationText.length);

        return (
          <Typography sx={{ whiteSpace: 'pre-wrap' }}>
            {beforeMatch}
            <Box
              component="span"
              ref={highlightedTextRef}
              sx={{
                backgroundColor: 'rgba(88, 166, 255, 0.3)',
                padding: '2px 4px',
                borderRadius: '2px',
              }}
            >
              {matchedText}
            </Box>
            {afterMatch}
          </Typography>
        );
      }

      // If the simple approach doesn't work, try with normalized text
      const normalize = (text: string): string => {
        return text
          .replace(/[\r\n]+/g, ' ')
          .replace(/\s+/g, ' ')
          .trim()
          .toLowerCase();
      };

      const normalizedContent = normalize(content);
      const normalizedCitation = normalize(citationText);

      if (normalizedContent.includes(normalizedCitation)) {
        // Try to find the best matching position in the original text
        // by matching word patterns
        const words = normalizedCitation.split(/\s+/);
        if (words.length > 3) {
          // Use first few words to anchor our search
          const startPattern = words.slice(0, 3).join('\\s+');
          const startRegex = new RegExp(startPattern, 'i');
          const startMatch = content.match(startRegex);

          if (startMatch && startMatch.index !== undefined) {
            // Find end position based on citation length - no buffer for exact match
            const startIdx = startMatch.index;

            // Find end pattern to get exact end position if possible
            const endWords = words.slice(-3).join('\\s+');
            const endRegex = new RegExp(endWords, 'i');
            const endMatch = content.substring(startIdx).match(endRegex);

            let endIdx;
            if (endMatch && endMatch.index !== undefined) {
              // Use precise end position
              endIdx = startIdx + endMatch.index + endMatch[0].length;
            } else {
              // Fallback: use citation length as guide, with no buffer
              endIdx = Math.min(startIdx + citationText.length, content.length);
            }

            // Highlight the region
            const beforeMatch = content.substring(0, startIdx);
            const matchedText = content.substring(startIdx, endIdx);
            const afterMatch = content.substring(endIdx);

            return (
              <Typography sx={{ whiteSpace: 'pre-wrap' }}>
                {beforeMatch}
                <Box
                  component="span"
                  ref={highlightedTextRef}
                  sx={{
                    backgroundColor: 'rgba(88, 166, 255, 0.3)',
                    padding: '2px 4px',
                    borderRadius: '2px',
                  }}
                >
                  {matchedText}
                </Box>
                {afterMatch}
              </Typography>
            );
          }
        }
      }

      // If the normalized approach doesn't work and we have a fuzzy match,
      // highlight individual significant words
      if (citation.validationStatus === 'fuzzy') {
        // Extract significant words from citation (longer than 3 chars)
        const significantWords = citationText
          .toLowerCase()
          .split(/\s+/)
          .filter(word => word.length > 3);

        // Don't highlight if too few significant words
        if (significantWords.length < 2) {
          return <Typography sx={{ whiteSpace: 'pre-wrap' }}>{content}</Typography>;
        }

        // Create a regex to find these words in the content
        // Using word boundaries and case insensitive matching
        const wordRegexes = significantWords.map(word =>
          new RegExp(`\\b${word.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`, 'gi')
        );

        // Make a copy of the content to highlight
        let highlightedContent = content;
        let placeholders: { [key: string]: string } = {};

        // Replace each significant word with a placeholder
        wordRegexes.forEach((regex, idx) => {
          const placeholder = `__HIGHLIGHT_${idx}__`;
          highlightedContent = highlightedContent.replace(regex, match => {
            placeholders[placeholder] = match;
            return placeholder;
          });
        });

        // Split the content by placeholders
        const parts: React.ReactNode[] = [];
        let lastIndex = 0;

        // Find all placeholders in the text
        const placeholderRegex = /__HIGHLIGHT_\d+__/g;
        let match;

        while ((match = placeholderRegex.exec(highlightedContent)) !== null) {
          // Add text before the placeholder
          if (match.index > lastIndex) {
            parts.push(highlightedContent.substring(lastIndex, match.index));
          }

          // Add the highlighted placeholder
          const placeholder = match[0];
          parts.push(
            <Box
              key={`highlight-${parts.length}`}
              component="span"
              sx={{
                backgroundColor: 'rgba(255, 152, 0, 0.2)',
                padding: '2px 2px',
                borderRadius: '2px',
              }}
            >
              {placeholders[placeholder]}
            </Box>
          );

          lastIndex = match.index + placeholder.length;
        }

        // Add any remaining text
        if (lastIndex < highlightedContent.length) {
          parts.push(highlightedContent.substring(lastIndex));
        }

        return <Typography sx={{ whiteSpace: 'pre-wrap' }}>{parts}</Typography>;
      }

      // Default case
      return <Typography sx={{ whiteSpace: 'pre-wrap' }}>{content}</Typography>;
    } catch (error) {
      console.error('Error highlighting text:', error);
      return <Typography sx={{ whiteSpace: 'pre-wrap' }}>{content}</Typography>;
    }
  };

  // Set state when dialog is fully open
  useEffect(() => {
    if (open) {
      // Mark dialog as fully open after a delay
      const timer = setTimeout(() => {
        setIsFullyOpen(true);
      }, 300);
      return () => {
        clearTimeout(timer);
        setIsFullyOpen(false);
      };
    }
  }, [open]);

  // Scroll to the highlighted text once dialog is open
  useEffect(() => {
    if (!isFullyOpen) return;

    // Set highlight effect
    setHighlight(true);
    const highlightTimer = setTimeout(() => setHighlight(false), 2000);

    // Primary scrolling method with increasing timeouts
    const scrollAttempts = [100, 300, 600, 1000, 1500];

    const scrollTimers = scrollAttempts.map(delay =>
      setTimeout(() => {
        try {
          console.log(`Attempt to scroll (${delay}ms)`);
          if (!leftPanelRef.current) {
            console.log('No left panel ref');
            return;
          }

          // First try direct scroll to highlighted text if available
          if (highlightedTextRef.current) {
            console.log('Found highlighted text element, scrolling...');
            const element = highlightedTextRef.current;
            const container = leftPanelRef.current;

            // Calculate where to scroll
            const elementRect = element.getBoundingClientRect();
            const containerRect = container.getBoundingClientRect();
            const relativeTop = elementRect.top - containerRect.top + container.scrollTop;

            // Scroll to position
            container.scrollTo({
              top: relativeTop - 100, // Position above the element for context
              behavior: 'smooth'
            });
            return;
          }

          // Fall back to scrolling to the best match container
          if (bestMatchRef.current) {
            console.log('Falling back to best match container');
            bestMatchRef.current.scrollIntoView({
              behavior: 'smooth',
              block: 'start'
            });
          }
        } catch (e) {
          console.error(`Scroll attempt failed (${delay}ms):`, e);
        }
      }, delay)
    );

    return () => {
      clearTimeout(highlightTimer);
      scrollTimers.forEach(timer => clearTimeout(timer));
    };
  }, [isFullyOpen]);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      TransitionProps={{
        onEntered: () => {
          // This ensures we attempt scrolling after the dialog animation completes
          setIsFullyOpen(true);
        }
      }}
    >
      <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography component="div" variant="subtitle1" sx={{ fontSize: '1.25rem', fontWeight: 500 }}>
          Citation Comparison
          {citation.validationStatus === 'exact' && (
            <Box component="span" sx={{ color: '#4caf50', ml: 1 }}> - Exact Match</Box>
          )}
          {citation.validationStatus === 'fuzzy' && (
            <Box component="span" sx={{ color: '#ff9800', ml: 1 }}> - Fuzzy Match</Box>
          )}
          {citation.validationStatus === 'failed' && (
            <Box component="span" sx={{ color: '#f44336', ml: 1 }}> - Failed Match</Box>
          )}
        </Typography>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{ color: (theme) => theme.palette.grey[500] }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent
        dividers
        sx={{
          display: 'flex',
          flexDirection: { xs: 'column', md: 'row' },
          maxHeight: '80vh'
        }}
      >
        {/* Left side - RAG results */}
        <Box
          ref={leftPanelRef}
          sx={{
            flex: 1,
            p: 2,
            overflow: 'auto',
            borderRight: { xs: 'none', md: `1px solid ${theme.palette.divider}` },
            mr: { xs: 0, md: 2 },
            mb: { xs: 2, md: 0 },
          }}
        >
          <Typography variant="h6" gutterBottom>
            Source Documents {matchingResults.length > 0 && (
              <Chip
                label={`${matchingResults.length} matching chunk${matchingResults.length > 1 ? 's' : ''}`}
                size="small"
                color="primary"
                sx={{ ml: 1 }}
              />
            )}
          </Typography>

          {/* First show matching results with the same document_id */}
          {matchingResults.length > 0 && (
            <>
              <Typography variant="subtitle2" gutterBottom color="text.secondary">
                Chunks from cited document:
              </Typography>
              {matchingResults.map((ragResult) => {
                const contentKey = createContentKey(ragResult);
                const isBestMatch = contentKey === bestMatchKey;
                return (
                  <Paper
                    key={contentKey}
                    ref={isBestMatch ? bestMatchRef : undefined}
                    elevation={3}
                    sx={{
                      p: 2,
                      mb: 2,
                      border: isBestMatch
                        ? highlight
                          ? `4px solid ${theme.palette.primary.main}`
                          : `2px solid ${theme.palette.primary.main}`
                        : '1px solid rgba(88, 166, 255, 0.3)',
                      position: 'relative',
                      backgroundColor: theme.palette.mode === 'dark' ?
                        'rgba(45, 48, 55, 0.7)' : 'rgba(245, 245, 245, 0.7)',
                      transition: 'border 0.3s ease-in-out',
                    }}
                  >
                    <Typography
                      variant="subtitle2"
                      color="text.secondary"
                      sx={{ mb: 1 }}
                    >
                      Source:
                      <Box
                        component="a"
                        href={ragResult.document_id === citation.docId && citation.fileUrl ?
                          citation.fileUrl : `#`}
                        target="_blank"
                        sx={{
                          color: theme.palette.mode === 'light' ? '#333' : '#bbb',
                          textDecoration: 'none',
                          fontWeight: 500,
                          opacity: 0.85,
                          transition: 'all 0.2s ease',
                          padding: '2px 6px',
                          ml: 1,
                          borderRadius: '4px',
                          backgroundColor: 'rgba(88, 166, 255, 0.1)',
                          '&:hover': {
                            opacity: 1,
                            backgroundColor: 'rgba(88, 166, 255, 0.2)',
                            textDecoration: 'underline',
                          }
                        }}
                      >
                        {ragResult.source}
                      </Box>
                      {ragResult.metadata?.offset && (
                        <> (Offset: {ragResult.metadata.offset})</>
                      )}
                    </Typography>

                    {/* Metadata Display */}
                    <Box
                      sx={{
                        mb: 1,
                        display: 'flex',
                        flexWrap: 'wrap',
                        gap: '8px',
                        fontSize: '0.75rem'
                      }}
                    >
                      <Chip
                        size="small"
                        label={`Doc ID: ${ragResult.document_id || 'N/A'}`}
                        sx={{
                          backgroundColor: 'rgba(88, 166, 255, 0.1)',
                          borderRadius: '4px',
                          height: 'auto',
                          '& .MuiChip-label': {
                            padding: '2px 8px',
                            whiteSpace: 'normal',
                            fontFamily: 'monospace'
                          }
                        }}
                      />

                      {ragResult.document_group_id && (
                        <Chip
                          size="small"
                          label={`Group ID: ${ragResult.document_group_id}`}
                          sx={{
                            backgroundColor: 'rgba(88, 166, 255, 0.1)',
                            borderRadius: '4px',
                            height: 'auto',
                            '& .MuiChip-label': {
                              padding: '2px 8px',
                              whiteSpace: 'normal',
                              fontFamily: 'monospace'
                            }
                          }}
                        />
                      )}

                      {ragResult.metadata?.chunk_id && (
                        <Chip
                          size="small"
                          label={`Chunk ID: ${ragResult.metadata.chunk_id}`}
                          sx={{
                            backgroundColor: 'rgba(88, 166, 255, 0.1)',
                            borderRadius: '4px',
                            height: 'auto',
                            '& .MuiChip-label': {
                              padding: '2px 8px',
                              whiteSpace: 'normal',
                              fontFamily: 'monospace'
                            }
                          }}
                        />
                      )}
                    </Box>

                    {/* Display all available metadata */}
                    {ragResult.metadata && Object.keys(ragResult.metadata).length > 0 && (
                      <Box sx={{ mb: 1 }}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{
                            display: 'block',
                            fontWeight: 'bold',
                            mb: 0.5
                          }}
                        >
                          Additional Metadata:
                        </Typography>
                        <Box
                          sx={{
                            backgroundColor: 'rgba(0, 0, 0, 0.05)',
                            borderRadius: '4px',
                            p: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.75rem',
                            overflowX: 'auto'
                          }}
                        >
                          {Object.entries(ragResult.metadata).map(([key, value]) => (
                            <Box
                              key={key}
                              sx={{
                                display: 'flex',
                                mb: 0.5,
                                "&:last-child": { mb: 0 }
                              }}
                            >
                              <Box sx={{ color: 'text.secondary', minWidth: '100px' }}>
                                {key}:
                              </Box>
                              <Box sx={{ ml: 1, wordBreak: 'break-all' }}>
                                {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                              </Box>
                            </Box>
                          ))}
                        </Box>
                      </Box>
                    )}

                    {/* Conditionally display image preview */}
                    {ragResult.document_id === citation.docId && citation.fileUrl && /\.(jpe?g)$/i.test(citation.fileUrl) && (
                      <Box sx={{ my: 1, textAlign: 'center' }}> {/* Added margin top/bottom and centered */}
                        <img
                          src={citation.fileUrl}
                          alt="Document Preview"
                          style={{
                            maxWidth: '100%', // Ensures image is responsive
                            maxHeight: '150px', // Limits the height
                            height: 'auto',     // Maintains aspect ratio
                            borderRadius: '4px', // Optional: adds rounded corners
                            border: `1px solid ${theme.palette.divider}` // Optional: adds a subtle border
                          }}
                        />
                      </Box>
                    )}

                    {isBestMatch && (
                      <Box
                        sx={{
                          position: 'absolute',
                          top: 0,
                          right: 0,
                          backgroundColor: theme.palette.primary.main,
                          color: theme.palette.primary.contrastText,
                          px: 1,
                          py: 0.5,
                          borderBottomLeftRadius: 4
                        }}
                      >
                        Best Match
                      </Box>
                    )}
                    <Divider sx={{ mb: 1 }} />
                    {highlightMatchedText(ragResult.content || '', citation.snippet, isBestMatch)}
                  </Paper>
                );
              })}
            </>
          )}

          {/* Then show other results */}
          {otherResults.length > 0 && (
            <>
              <Typography variant="subtitle2" gutterBottom mt={2} color="text.secondary">
                Other documents:
              </Typography>
              {otherResults.map((ragResult) => {
                const contentKey = createContentKey(ragResult);
                return (
                  <Paper
                    key={contentKey}
                    elevation={3}
                    sx={{
                      p: 2,
                      mb: 2,
                      opacity: 0.8,
                      position: 'relative',
                      backgroundColor: theme.palette.mode === 'dark' ?
                        'rgba(45, 48, 55, 0.6)' : 'rgba(245, 245, 245, 0.6)',
                    }}
                  >
                    <Typography
                      variant="subtitle2"
                      color="text.secondary"
                      sx={{ mb: 1 }}
                    >
                      Source:
                      <Box
                        component="a"
                        href={ragResult.document_id === citation.docId && citation.fileUrl ?
                          citation.fileUrl : `#`}
                        target="_blank"
                        sx={{
                          color: theme.palette.mode === 'light' ? '#333' : '#bbb',
                          textDecoration: 'none',
                          fontWeight: 500,
                          opacity: 0.85,
                          transition: 'all 0.2s ease',
                          padding: '2px 6px',
                          ml: 1,
                          borderRadius: '4px',
                          backgroundColor: 'rgba(88, 166, 255, 0.1)',
                          '&:hover': {
                            opacity: 1,
                            backgroundColor: 'rgba(88, 166, 255, 0.2)',
                            textDecoration: 'underline',
                          }
                        }}
                      >
                        {ragResult.source}
                      </Box>
                      {ragResult.metadata?.offset && (
                        <> (Offset: {ragResult.metadata.offset})</>
                      )}
                    </Typography>

                    {/* Metadata Display */}
                    <Box
                      sx={{
                        mb: 1,
                        display: 'flex',
                        flexWrap: 'wrap',
                        gap: '8px',
                        fontSize: '0.75rem'
                      }}
                    >
                      <Chip
                        size="small"
                        label={`Doc ID: ${ragResult.document_id || 'N/A'}`}
                        sx={{
                          backgroundColor: 'rgba(88, 166, 255, 0.1)',
                          borderRadius: '4px',
                          height: 'auto',
                          '& .MuiChip-label': {
                            padding: '2px 8px',
                            whiteSpace: 'normal',
                            fontFamily: 'monospace'
                          }
                        }}
                      />

                      {ragResult.document_group_id && (
                        <Chip
                          size="small"
                          label={`Group ID: ${ragResult.document_group_id}`}
                          sx={{
                            backgroundColor: 'rgba(88, 166, 255, 0.1)',
                            borderRadius: '4px',
                            height: 'auto',
                            '& .MuiChip-label': {
                              padding: '2px 8px',
                              whiteSpace: 'normal',
                              fontFamily: 'monospace'
                            }
                          }}
                        />
                      )}

                      {ragResult.metadata?.chunk_id && (
                        <Chip
                          size="small"
                          label={`Chunk ID: ${ragResult.metadata.chunk_id}`}
                          sx={{
                            backgroundColor: 'rgba(88, 166, 255, 0.1)',
                            borderRadius: '4px',
                            height: 'auto',
                            '& .MuiChip-label': {
                              padding: '2px 8px',
                              whiteSpace: 'normal',
                              fontFamily: 'monospace'
                            }
                          }}
                        />
                      )}
                    </Box>

                    {/* Display all available metadata */}
                    {ragResult.metadata && Object.keys(ragResult.metadata).length > 0 && (
                      <Box sx={{ mb: 1 }}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{
                            display: 'block',
                            fontWeight: 'bold',
                            mb: 0.5
                          }}
                        >
                          Additional Metadata:
                        </Typography>
                        <Box
                          sx={{
                            backgroundColor: 'rgba(0, 0, 0, 0.05)',
                            borderRadius: '4px',
                            p: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.75rem',
                            overflowX: 'auto'
                          }}
                        >
                          {Object.entries(ragResult.metadata).map(([key, value]) => (
                            <Box
                              key={key}
                              sx={{
                                display: 'flex',
                                mb: 0.5,
                                "&:last-child": { mb: 0 }
                              }}
                            >
                              <Box sx={{ color: 'text.secondary', minWidth: '100px' }}>
                                {key}:
                              </Box>
                              <Box sx={{ ml: 1, wordBreak: 'break-all' }}>
                                {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                              </Box>
                            </Box>
                          ))}
                        </Box>
                      </Box>
                    )}

                    <Divider sx={{ mb: 1 }} />
                    <Typography sx={{ whiteSpace: 'pre-wrap' }}>{ragResult.content}</Typography>
                  </Paper>
                );
              })}
            </>
          )}

          {ragResults.length === 0 && (
            <Typography color="text.secondary">No source documents available</Typography>
          )}
        </Box>

        {/* Right side - Citation */}
        <Box sx={{ flex: 1, p: 2, overflow: 'auto' }}>
          <Typography variant="h6" gutterBottom>
            Citation Text
          </Typography>
          <Paper
            elevation={3}
            sx={{
              p: 2,
              backgroundColor: theme.palette.mode === 'dark' ?
                'rgba(45, 48, 55, 0.7)' : 'rgba(245, 245, 245, 0.7)',
            }}
          >
            <Typography
              variant="subtitle2"
              color="text.secondary"
              sx={{ mb: 1 }}
            >
              Document ID:
            </Typography>

            {/* Document ID display */}
            <Box
              sx={{
                mb: 2,
                display: 'flex',
                flexWrap: 'wrap',
                gap: '8px',
                fontSize: '0.75rem'
              }}
            >
              <Box
                component="a"
                href={citation.fileUrl || `/api/v1/documents/view/${citation.docId}`}
                target="_blank"
                sx={{
                  textDecoration: 'none',
                }}
              >
                <Chip
                  size="small"
                  label={citation.docId}
                  sx={{
                    backgroundColor: 'rgba(88, 166, 255, 0.15)',
                    borderRadius: '4px',
                    height: 'auto',
                    cursor: 'pointer',
                    transition: 'all 0.2s ease',
                    '&:hover': {
                      backgroundColor: 'rgba(88, 166, 255, 0.3)',
                    },
                    '& .MuiChip-label': {
                      padding: '4px 8px',
                      whiteSpace: 'normal',
                      fontFamily: 'monospace',
                      fontWeight: 'bold'
                    }
                  }}
                />
              </Box>
            </Box>

            <Divider sx={{ mb: 1 }} />
            <Typography
              sx={{
                p: 2,
                backgroundColor: 'rgba(88, 166, 255, 0.1)',
                borderRadius: 1,
                whiteSpace: 'pre-wrap',
                fontStyle: 'italic'
              }}
            >
              <Box
                component="span"
                sx={{
                  color: 'rgba(88, 166, 255, 1.0)',
                  fontFamily: 'Georgia, serif',
                  fontSize: '1.5em',
                  fontWeight: 'bold',
                  lineHeight: 0,
                  position: 'relative',
                  marginRight: '0.15em',
                  top: '0.1em',
                }}
              >
                {'\u201C'}
              </Box>

              {citation.snippet}

              <Box
                component="span"
                sx={{
                  color: 'rgba(88, 166, 255, 1.0)',
                  fontFamily: 'Georgia, serif',
                  fontSize: '1.5em',
                  fontWeight: 'bold',
                  lineHeight: 0,
                  position: 'relative',
                  marginLeft: '0.15em',
                  top: '0.1em',
                }}
              >
                {'\u201D'}
              </Box>
            </Typography>
          </Paper>
        </Box>
      </DialogContent>
    </Dialog>
  );
};

export default React.memo(CitationComparisonModal); 