import React from 'react';
import { Box } from '@mui/material';
import { keyframes } from '@mui/material/styles';

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
}

interface CitationProps {
    excerpts: Excerpt[];
    isStreaming?: boolean;
    className?: string;
}

const Citation: React.FC<CitationProps> = ({
    excerpts,
    isStreaming = false,
    className = ''
}) => {
    // If there are no excerpts, return nothing
    if (!excerpts || excerpts.length === 0) {
        return null;
    }

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
                // Responsive adjustments for smaller screens
                '@media (max-width: 768px)': {
                    width: '100%',
                    maxWidth: '100%',
                    float: 'none',
                    margin: '24px 0 28px 0',
                    '& .citation-header': {
                        textAlign: 'left',
                    }
                }
            }}
        >
            <Box
                className="citation-header"
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
                    className={`citation-item${excerpt.isPartial ? ' loading-item' : ''}`}
                    sx={{
                        background: 'linear-gradient(to bottom, rgba(45, 48, 55, 0.7), rgba(35, 38, 45, 0.7))',
                        borderRadius: '10px',
                        padding: '18px 20px',
                        marginBottom: '18px',
                        boxShadow: '0 2px 8px rgba(0, 0, 0, 0.25), 0 1px 2px rgba(0, 0, 0, 0.3)',
                        position: 'relative',
                        borderLeft: '3px solid rgba(88, 166, 255, 0.6)',
                        transition: 'all 0.25s cubic-bezier(0.2, 0.8, 0.2, 1)',
                        '&:hover': {
                            transform: 'translateY(-3px) scale(1.01)',
                            boxShadow: '0 5px 15px rgba(0, 0, 0, 0.3), 0 2px 4px rgba(0, 0, 0, 0.2)',
                            borderLeftWidth: '4px',
                            borderLeftColor: 'rgba(88, 166, 255, 0.8)',
                        },
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
                        className={`citation-quote${excerpt.isPartial ? ' loading-content' : ''}`}
                        component="p"
                        sx={{
                            fontStyle: 'italic',
                            lineHeight: 1.6,
                            margin: '0 0 12px 0',
                            fontSize: '0.95em',
                            color: '#e0e0e0',
                            position: 'relative',
                            paddingLeft: '1.8em',
                            textIndent: 0,
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
                            }}
                        >
                            {'\u201C'}
                        </Box>

                        {excerpt.snippet}

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
                            }}
                        >
                            {'\u201D'}
                        </Box>
                    </Box>

                    <Box
                        component="p"
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
                            <Box
                                component="a"
                                href={excerpt.fileUrl}
                                target="_blank"
                                sx={{
                                    color: '#58a6ff',
                                    textDecoration: 'none',
                                    fontWeight: 500,
                                    opacity: 0.85,
                                    transition: 'all 0.2s ease',
                                    padding: '3px 8px',
                                    borderRadius: '4px',
                                    backgroundColor: 'rgba(88, 166, 255, 0.1)',
                                    '&:hover': {
                                        opacity: 1,
                                        backgroundColor: 'rgba(88, 166, 255, 0.2)',
                                        textDecoration: 'underline',
                                    }
                                }}
                            >
                                {excerpt.filename}
                            </Box>
                        )}
                    </Box>
                </Box>
            ))}
        </Box>
    );
};

export default Citation;