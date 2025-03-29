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
import { ISessionRAGResult } from '../../types';

interface CitationComparisonModalProps {
  open: boolean;
  onClose: () => void;
  citation: {
    docId: string;
    snippet: string;
    validationStatus: 'exact' | 'fuzzy' | 'failed';
  };
  ragResults: ISessionRAGResult[];
}

// Helper function to create a content-based key for RAG results
const createContentKey = (result: ISessionRAGResult): string => {
  return `${result.document_id}-${hashString(result.content)}`;
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
  // Normalize both strings
  const normalized1 = str1.replace(/\s+/g, ' ').toLowerCase().trim();
  const normalized2 = str2.replace(/\s+/g, ' ').toLowerCase().trim();
  
  // For very short strings, use a different approach
  if (normalized1.length < 10 || normalized2.length < 10) {
    return normalized1 === normalized2 ? 1.0 : 0.0;
  }
  
  // For longer strings, use Jaccard similarity
  const words1 = new Set(normalized1.split(/\s+/));
  const words2 = new Set(normalized2.split(/\s+/));
  
  const intersection = new Set([...words1].filter(x => words2.has(x)));
  const union = new Set([...words1, ...words2]);
  
  return intersection.size / union.size;
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
      const similarity = calculateSimilarity(result.content, citation.snippet);
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
      // Normalize both texts for comparison
      const normalizedContent = content.replace(/\s+/g, ' ').trim();
      const normalizedCitation = citationText.replace(/\s+/g, ' ').trim();
      
      // For exact match
      if (normalizedContent.includes(normalizedCitation)) {
        const parts = normalizedContent.split(normalizedCitation);
        return (
          <Typography sx={{ whiteSpace: 'pre-wrap' }}>
            {parts.map((part, index) => (
              <React.Fragment key={index}>
                {part}
                {index < parts.length - 1 && (
                  <Box 
                    component="span" 
                    ref={isMatch && index === 0 ? highlightedTextRef : undefined}
                    sx={{ 
                      backgroundColor: 'rgba(88, 166, 255, 0.3)',
                      padding: '2px 4px',
                      borderRadius: '2px',
                    }}
                  >
                    {normalizedCitation}
                  </Box>
                )}
              </React.Fragment>
            ))}
          </Typography>
        );
      }
      
      // If not an exact match but fuzzy, just return the plain content
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
        <Typography variant="h6">
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
                      Source: {ragResult.source}
                      {ragResult.metadata?.offset && (
                        <> (Offset: {ragResult.metadata.offset})</>
                      )}
                    </Typography>
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
                    {highlightMatchedText(ragResult.content, citation.snippet, isBestMatch)}
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
                      Source: {ragResult.source}
                      {ragResult.metadata?.offset && (
                        <> (Offset: {ragResult.metadata.offset})</>
                      )}
                    </Typography>
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
              Document ID: {citation.docId}
            </Typography>
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

export default CitationComparisonModal; 