import React, { useState, useCallback } from 'react';
import { Box, Button, Container, Tooltip } from '@mui/material';
import { keyframes } from '@mui/material/styles';
import { FilterAlt, CheckCircle, Warning, Cancel, FileOpen } from '@mui/icons-material';
import { useTheme } from '@mui/material/styles'
import CitationComparisonModal from './CitationComparisonModal';
import { TypesSessionRAGResult } from '../../api/api';

// Reuse the same animations from the Markdown component
const pulseFade = keyframes`
  0% { opacity: 0.7; }
  50% { opacity: 0.9; }
  100% { opacity: 0.7; }
`;

const shimmer = keyframes`
  0% { background-position: 100% 0; }
  100% { background-position: -100% 0; }
`;

const subtleBounce = keyframes`
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-1px); }
`;

const fadeIn = keyframes`
  0% { opacity: 0; transform: translateX(10px); }
  100% { opacity: 1; transform: translateX(0); }
`;

// Define the excerpt type
export interface Excerpt {
    docId: string;
    snippet: string;
    filename: string;
    fileUrl: string;
    isPartial: boolean;
    citationNumber?: number;
    validationStatus?: 'exact' | 'fuzzy' | 'failed';
    validationMessage?: string;
}

interface CitationProps {
    excerpts: Excerpt[];
    isStreaming?: boolean;
    className?: string;
    onFilterDocument?: (docId: string) => void;
    ragResults?: TypesSessionRAGResult[];
}

const Citation: React.FC<CitationProps> = ({
    excerpts,
    isStreaming = false,
    className = '',
    onFilterDocument,
    ragResults = []
}) => {
    const theme = useTheme()
    const [comparisonModalOpen, setComparisonModalOpen] = useState(false);
    const [selectedExcerpt, setSelectedExcerpt] = useState<Excerpt | null>(null);

    // If there are no excerpts, return nothing
    if (!excerpts || excerpts.length === 0) {
        return null;
    }

    const isImage = (filename: string) => {
        return /\.(jpe?g)$/i.test(filename);
    }

    const originalFilenameForExcerpt = (excerpt: Excerpt) => {
        const ragResult = ragResults.find(r => r.document_id === excerpt.docId);
        return ragResult?.metadata?.original_filename;
    }

    const originalSourceFromExcerpt = (excerpt: Excerpt) => {
        const ragResult = ragResults.find(r => r.document_id === excerpt.docId);
        return ragResult?.metadata?.original_source;
    }

    const originalIDFromExcerpt = (excerpt: Excerpt) => {
        const ragResult = ragResults.find(r => r.document_id === excerpt.docId);
        return ragResult?.metadata?.document_id;
    }

    // Function to handle clicking on a validation icon
    const handleValidationClick = (excerpt: Excerpt) => {
        if (!excerpt.validationStatus) return;

        setSelectedExcerpt(excerpt);
        setComparisonModalOpen(true);
    };

    // Helper function to create a unique key for a RAG result (similar to backend logic)
    const createUniqueRagKey = useCallback((ragResult: any): string => {
        let key = `${ragResult.document_id}-${ragResult.content.substring(0, 50).replace(/\s+/g, '-')}`;

        // Add chunk identification if available in metadata
        if (ragResult.metadata) {
            if (ragResult.metadata.chunk_id) {
                key += `-chunk-${ragResult.metadata.chunk_id}`;
            } else if (ragResult.metadata.offset) {
                key += `-offset-${ragResult.metadata.offset}`;
            }
        }

        return key;
    }, []);

    // Memoize the normalize text function to avoid recreating it on every render
    const normalizeText = useCallback((text: string): string => {
        return text
            .replace(/[\r\n]+/g, ' ') // Replace newlines with spaces
            .replace(/#/g, ' ')       // Replace # with spaces
            .replace(/\s+/g, ' ')     // Normalize all whitespace
            .toLowerCase()
            .trim();
    }, []);

    // Find the best matching chunk for a given citation excerpt - memoize this expensive operation
    const findBestMatchingChunk = useCallback((excerpt: Excerpt, results: any[]): any | null => {
        // Filter by document ID first
        const matchingResults = results.filter(r => r.document_id === excerpt.docId);

        if (matchingResults.length === 0) {
            return null;
        }

        if (matchingResults.length === 1) {
            return matchingResults[0];
        }

        // For multiple chunks, find the best match based on content similarity
        const excerptText = normalizeText(excerpt.snippet);

        // First try exact match
        for (const result of matchingResults) {
            const resultText = normalizeText(result.content);
            if (resultText.includes(excerptText)) {
                return result; // Found an exact match
            }
        }

        // If no exact match, find the chunk with highest similarity
        let bestMatch = matchingResults[0];
        let bestSimilarity = 0;

        for (const result of matchingResults) {
            const resultText = normalizeText(result.content);

            // Simple similarity check - more sophisticated methods could be used
            let similarity = 0;

            // Count word matches
            const excerptWords = excerptText.split(/\s+/).filter(w => w.length > 3);
            const resultWords = resultText.split(/\s+/).filter(w => w.length > 3);

            const matchingWords = excerptWords.filter(word =>
                resultWords.some(rWord => rWord.includes(word) || word.includes(rWord))
            );

            similarity = excerptWords.length > 0 ? matchingWords.length / excerptWords.length : 0;

            if (similarity > bestSimilarity) {
                bestSimilarity = similarity;
                bestMatch = result;
            }
        }

        return bestMatch;
    }, [normalizeText]);

    // Function to truncate filename if it's too long
    const truncateFilename = (filename: string, maxLength = 25) => {
        if (!filename || filename.length <= maxLength) return filename;
        const extension = filename.lastIndexOf('.') > -1 ? filename.slice(filename.lastIndexOf('.')) : '';
        const name = filename.slice(0, filename.lastIndexOf('.'));
        return `${name.slice(0, maxLength - extension.length - 3)}...${extension}`;
    };

    return (
        <Box
            className={`citation-box${isStreaming ? ' streaming' : ''} ${className}`}
            sx={{
                float: 'right',
                width: '35%',
                maxWidth: '400px',
                margin: '0 0 28px 28px',
                clear: 'right',
                transition: 'opacity 0.3s ease',
                animation: `${fadeIn} 0.4s ease-out`,

                // Loading indicator for streaming
                '&.streaming::after': {
                    content: '""',
                    display: 'block',
                    height: '2px',
                    background: 'linear-gradient(90deg, rgba(88, 166, 255, 0.3), rgba(88, 166, 255, 0.8), rgba(88, 166, 255, 0.3))',
                    backgroundSize: '200% 100%',
                    borderRadius: '2px',
                    marginTop: '8px',
                    animation: `${shimmer} 2s infinite linear`,
                },

                // Responsive styles
                '@media (max-width: 768px)': {
                    width: '100%',
                    maxWidth: '100%',
                    float: 'none',
                    margin: '24px 0 28px 0',
                },

                // Quote styling
                '& .start-quote, & .end-quote': {
                    color: 'rgba(88, 166, 255, 1.0)',
                    fontFamily: 'Georgia, serif',
                    fontSize: '1.5em',
                    fontWeight: 'bold',
                    lineHeight: 0,
                    position: 'relative',
                },

                '& .start-quote': {
                    marginRight: '0.15em',
                    top: '0.1em',
                },

                '& .end-quote': {
                    marginLeft: '0.15em',
                    top: '0.1em',
                },

                // Loading states
                '& .loading-full': {
                    color: '#aaa',
                    fontStyle: 'italic',
                    animation: `${subtleBounce} 1.2s infinite ease-in-out`,
                },

                '& .loading-content': {
                    color: '#aaa',
                    fontStyle: 'italic',
                    '&::after': {
                        content: '"..."',
                        animation: `${subtleBounce} 1.2s infinite ease-in-out`,
                        display: 'inline-block',
                    }
                },

                '& .loading-search': {
                    color: '#aaa',
                    fontStyle: 'italic',
                    display: 'inline-block',
                    position: 'relative',
                    paddingRight: '20px',
                    '&::after': {
                        content: '""',
                        position: 'absolute',
                        right: 0,
                        top: '50%',
                        width: '12px',
                        height: '12px',
                        marginTop: '-6px',
                        borderRadius: '50%',
                        border: '2px solid rgba(88, 166, 255, 0.4)',
                        borderTopColor: 'rgba(88, 166, 255, 0.8)',
                        animation: 'spin 1s linear infinite',
                    }
                },

                // Loading item styles
                '& .loading-item': {
                    position: 'relative',
                    '&::before': {
                        content: '""',
                        position: 'absolute',
                        left: '-5px',
                        top: 0,
                        bottom: 0,
                        width: '5px',
                        background: 'rgba(88, 166, 255, 0.5)',
                        animation: `${pulseFade} 1.5s infinite ease-in-out`,
                    }
                },

                '@keyframes spin': {
                    '0%': { transform: 'rotate(0deg)' },
                    '100%': { transform: 'rotate(360deg)' },
                }
            }}
        >
            <Box
                sx={{
                    fontWeight: 600,
                    marginBottom: '16px',
                    fontSize: '0.85em',
                    color: '#aaa',
                    textTransform: 'uppercase',
                    letterSpacing: '0.08em',
                    textAlign: 'right',
                }}
            >
                SOURCES
            </Box>

            {excerpts.map((excerpt, index) => (
                <Box
                    key={`${excerpt.docId}-${index}`}
                    className={`${excerpt.isPartial ? ' loading-item' : ''}`}
                    sx={{
                        background: 'linear-gradient(to bottom, rgba(45, 48, 55, 0.7), rgba(35, 38, 45, 0.7))',
                        borderRadius: '10px',
                        padding: '18px 20px',
                        marginBottom: '18px',
                        boxShadow: '0 2px 8px rgba(0, 0, 0, 0.25), 0 1px 2px rgba(0, 0, 0, 0.3)',
                        position: 'relative',
                        borderLeft: '3px solid rgba(88, 166, 255, 0.6)',
                        transition: 'all 0.25s cubic-bezier(0.2, 0.8, 0.2, 1)',
                        '&.loading': {
                            animation: `${pulseFade} 2s infinite ease-in-out`,
                            borderLeftColor: 'rgba(170, 170, 170, 0.4)',
                            position: 'relative',
                            overflow: 'hidden',
                            '&::after': {
                                content: '""',
                                position: 'absolute',
                                top: 0,
                                left: 0,
                                right: 0,
                                bottom: 0,
                                background: 'linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.05), transparent)',
                                backgroundSize: '200% 100%',
                                animation: `${shimmer} 1.5s infinite`,
                                pointerEvents: 'none',
                            }
                        },
                        '&.loading-item': {
                            position: 'relative',
                            '&::before': {
                                content: '""',
                                position: 'absolute',
                                left: '-5px',
                                top: 0,
                                bottom: 0,
                                width: '5px',
                                background: 'rgba(88, 166, 255, 0.5)',
                                animation: `${pulseFade} 1.5s infinite ease-in-out`,
                            }
                        }
                    }}
                >
                    <Box
                        className={`${excerpt.isPartial ? ' loading-content' : ''}`}
                        component="p"
                        sx={{
                            fontStyle: 'italic',
                            lineHeight: 1.6,
                            margin: '0 0 12px 0',
                            fontSize: '0.95em',
                            color: '#e0e0e0',
                            position: 'relative',
                            textIndent: 0,
                            display: 'flex',
                            gap: '8px',
                            alignItems: 'flex-start',
                            '& .loading-content': {
                                color: '#aaa',
                                fontStyle: 'italic',
                                '&::after': {
                                    content: '"..."',
                                    animation: `${subtleBounce} 1.2s infinite ease-in-out`,
                                    display: 'inline-block',
                                }
                            },
                            '&.loading-full': {
                                color: '#aaa',
                                fontStyle: 'italic',
                                animation: `${subtleBounce} 1.2s infinite ease-in-out`,
                            }
                        }}
                    >
                        {excerpt.citationNumber && (
                            <Box
                                component="span"
                                sx={{
                                    color: theme.palette.mode === 'light' ? '#333' : '#bbb',
                                    fontSize: '0.85em',
                                    fontWeight: 'bold',
                                    padding: '2px 6px',
                                    flexShrink: 0,
                                    marginTop: '2px'
                                }}
                            >
                                [{excerpt.citationNumber}]
                            </Box>
                        )}
                        <Box component="span">
                            {isImage(excerpt.filename) ? (
                                null
                            ) :
                                <Box
                                    component="span"
                                    className="start-quote"
                                    sx={{
                                        color: 'rgba(88, 166, 255, 1.0)',
                                        fontFamily: 'Georgia, serif',
                                        fontSize: '1.5em',
                                        fontWeight: 'bold',
                                        lineHeight: 0,
                                        position: 'relative',
                                        marginRight: '0.15em',
                                        top: '0.1em',
                                        display: excerpt.validationStatus && excerpt.validationStatus !== 'exact' ? 'none' : 'inline'
                                    }}
                                >
                                    {'\u201C'}
                                </Box>
                            }

                            <span dangerouslySetInnerHTML={{ __html: excerpt.snippet }}></span>

                            {isImage(excerpt.filename) ? (
                                null
                            ) :
                                <Box
                                    component="span"
                                    className="end-quote"
                                    sx={{
                                        color: 'rgba(88, 166, 255, 1.0)',
                                        fontFamily: 'Georgia, serif',
                                        fontSize: '1.5em',
                                        fontWeight: 'bold',
                                        lineHeight: 0,
                                        position: 'relative',
                                        marginLeft: '0.15em',
                                        top: '0.1em',
                                        display: excerpt.validationStatus && excerpt.validationStatus !== 'exact' ? 'none' : 'inline'
                                    }}
                                >
                                    {'\u201D'}
                                </Box>
                            }
                            {/* Only show filter button if onFilterDocument is provided */}
                            {onFilterDocument && (
                                <FilterAlt
                                    sx={{
                                        fontSize: '1em',
                                        cursor: 'pointer',
                                        color: theme.palette.mode === 'light' ? '#333' : '#bbb',
                                        textDecoration: 'none',
                                        fontWeight: 500,
                                        opacity: 0.85,
                                        transition: 'all 0.2s ease',
                                        backgroundColor: 'rgba(88, 166, 255, 0.1)',
                                        '&:hover': {
                                            opacity: 1,
                                            backgroundColor: 'rgba(88, 166, 255, 0.2)',
                                            textDecoration: 'underline',
                                        }
                                    }}
                                    titleAccess="Add this document to filter over on the next search"
                                    onClick={() => {
                                        if (isImage(excerpt.filename)) {
                                            const originalID = originalIDFromExcerpt(excerpt)
                                            return onFilterDocument?.(originalID || '')
                                        }
                                        return onFilterDocument?.(excerpt.docId)
                                    }
                                    } />
                            )}

                            {/* Validation status indicator */}
                            {excerpt.validationStatus && (
                                <Tooltip title={excerpt.validationMessage || ''} placement="top">
                                    <Box
                                        component="span"
                                        sx={{
                                            display: 'inline-flex',
                                            alignItems: 'center',
                                            marginLeft: '5px',
                                            opacity: 0.9,
                                            cursor: 'help',
                                            '&:hover': {
                                                opacity: 1
                                            }
                                        }}
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            handleValidationClick(excerpt);
                                        }}
                                    >
                                        {excerpt.validationStatus === 'exact' && (
                                            <CheckCircle
                                                fontSize="small"
                                                sx={{
                                                    color: '#4caf50',
                                                    fontSize: '0.9rem',
                                                    cursor: 'pointer'
                                                }}
                                            />
                                        )}
                                        {excerpt.validationStatus === 'fuzzy' && (
                                            <Warning
                                                fontSize="small"
                                                sx={{
                                                    color: '#ff9800',
                                                    fontSize: '0.9rem',
                                                    cursor: 'pointer'
                                                }}
                                            />
                                        )}
                                        {excerpt.validationStatus === 'failed' && (
                                            <Cancel
                                                fontSize="small"
                                                sx={{
                                                    color: '#f44336',
                                                    fontSize: '0.9rem',
                                                    cursor: 'pointer'
                                                }}
                                            />
                                        )}
                                    </Box>
                                </Tooltip>
                            )}
                        </Box>
                    </Box>

                    <Box
                        component="div"
                        className="citation-source"
                        sx={{
                            fontSize: '0.8em',
                            margin: 0,
                            textAlign: 'right',
                            opacity: 0.8,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'flex-end',
                            gap: '0.4em',
                            paddingRight: '6px',
                            '&.loading-full': {
                                color: '#aaa',
                                fontStyle: 'italic',
                                animation: `${subtleBounce} 1.2s infinite ease-in-out`,
                            }
                        }}
                    >
                        {excerpt.isPartial && excerpt.filename === "Loading..." ? (
                            <Box
                                component="span"
                                className="loading-search"
                                sx={{
                                    color: '#aaa',
                                    fontStyle: 'italic',
                                    display: 'inline-block',
                                    position: 'relative',
                                    paddingRight: '20px',
                                    '&::after': {
                                        content: '""',
                                        position: 'absolute',
                                        right: 0,
                                        top: '50%',
                                        width: '12px',
                                        height: '12px',
                                        marginTop: '-6px',
                                        borderRadius: '50%',
                                        border: '2px solid rgba(88, 166, 255, 0.4)',
                                        borderTopColor: 'rgba(88, 166, 255, 0.8)',
                                        animation: 'spin 1s linear infinite',
                                    }
                                }}
                            >
                                Searching documents...
                            </Box>
                        ) : (
                            <Container
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'flex-end',
                                    gap: '0.4em',
                                    minWidth: 0,
                                }}
                            >
                                {isImage(excerpt.filename) ? (
                                    <Box
                                        sx={{
                                            display: 'inline-flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            gap: '4px',
                                            maxWidth: '100%',
                                        }}
                                    >
                                        <a 
                                            href={originalSourceFromExcerpt(excerpt) || excerpt.fileUrl}
                                            target="_blank"
                                            style={{
                                                textDecoration: 'none',
                                                cursor: 'pointer'
                                            }}
                                        >
                                            <img
                                                src={excerpt.fileUrl}
                                                alt={`${excerpt.filename} preview`}
                                                style={{
                                                    maxWidth: '200px',
                                                    maxHeight: '200px',
                                                    height: 'auto',
                                                    verticalAlign: 'middle',
                                                    borderRadius: '3px'
                                                }}
                                            />
                                        </a>
                                        <Box
                                            component="a"
                                            href={originalSourceFromExcerpt(excerpt) || excerpt.fileUrl}
                                            target="_blank"
                                            sx={{
                                                display: 'inline-flex',
                                                alignItems: 'center',
                                                gap: '5px',
                                                padding: '3px 8px',
                                                borderRadius: '15px',
                                                borderColor: 'rgba(88, 166, 255, 0.2)',
                                                borderWidth: '1px',
                                                borderStyle: 'solid',
                                                backgroundColor: 'rgba(88, 166, 255, 0.1)',
                                                color: theme.palette.mode === 'light' ? '#333' : '#bbb',
                                                textDecoration: 'none',
                                                fontWeight: 500,
                                                opacity: 0.85,
                                                transition: 'all 0.2s ease',
                                                '&:hover': {
                                                    opacity: 1,
                                                    backgroundColor: 'rgba(88, 166, 255, 0.2)',
                                                    textDecoration: 'underline',
                                                }
                                            }}
                                        >
                                            <FileOpen fontSize="small" />
                                            <Box component="span" sx={{
                                                whiteSpace: 'nowrap',
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                minWidth: 0,
                                            }}>
                                                {truncateFilename(originalFilenameForExcerpt(excerpt) || excerpt.filename)}
                                            </Box>
                                        </Box>
                                    </Box>
                                ) : (
                                    <Box
                                        component="a"
                                        href={excerpt.fileUrl}
                                        target="_blank"
                                        sx={{
                                            color: theme.palette.mode === 'light' ? '#333' : '#bbb',
                                            textDecoration: 'none',
                                            fontWeight: 500,
                                            opacity: 0.85,
                                            transition: 'all 0.2s ease',
                                            padding: '3px 8px',
                                            borderRadius: '4px',
                                            backgroundColor: 'rgba(88, 166, 255, 0.1)',
                                            display: 'inline-flex',
                                            alignItems: 'center',
                                            gap: '5px',
                                            maxWidth: '100%',
                                            '&:hover': {
                                                opacity: 1,
                                                backgroundColor: 'rgba(88, 166, 255, 0.2)',
                                                textDecoration: 'underline',
                                            }
                                        }}
                                    >
                                        <Box component="span" sx={{
                                            whiteSpace: 'nowrap',
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            minWidth: 0,
                                            textAlign: 'center',
                                            width: 'auto',
                                        }}>
                                            {truncateFilename(excerpt.filename)}
                                        </Box>
                                    </Box>
                                )}
                            </Container>
                        )}
                    </Box>
                </Box>
            ))
            }

            {/* Citation Comparison Modal */}
            {
                selectedExcerpt && (
                    <CitationComparisonModal
                        open={comparisonModalOpen}
                        onClose={() => setComparisonModalOpen(false)}
                        citation={{
                            docId: selectedExcerpt.docId,
                            snippet: selectedExcerpt.snippet,
                            validationStatus: selectedExcerpt.validationStatus || 'failed',
                            fileUrl: selectedExcerpt.fileUrl
                        }}
                        ragResults={ragResults}
                    />
                )
            }
        </Box >
    );
};

export default React.memo(Citation);