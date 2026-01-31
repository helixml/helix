import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
    Menu,
    MenuItem,
    InputBase,
    Paper,
    MenuList,
    Box,
    Typography,
    Divider,
    ListSubheader
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import useApi from '../../hooks/useApi';
import { TypesContextMenuAction } from '../../api/api';

interface ContextMenuPosition {
    top: number;
    left: number;
}

interface UseContextMenuOptions {
    appId: string;
    textAreaRef?: React.RefObject<HTMLTextAreaElement | HTMLInputElement | undefined>;
    onInsertText?: (text: string) => void;
}

// Hook to manage context menu state and functionality.
//
// How It Works:
// 1. When a filter is selected, the code parses @filter([DOC_NAME:nvidia-10-k/nvidia-form-10-k.pdf][DOC_ID:b257cbb961])
// 2. Extracts the filename nvidia-form-10-k.pdf from the path
// 3. Displays @nvidia-form-10-k.pdf in the textarea
// 4. Stores the mapping @nvidia-form-10-k.pdf → full filter command
// 5. When sending, replaces all @filename occurrences with their full filter commands
// 6. Clears the filter map after sending
export const useContextMenu = ({
    appId,
    textAreaRef,
    onInsertText
}: UseContextMenuOptions) => {
    const [isOpen, setIsOpen] = useState(false);
    const [query, setQuery] = useState('');
    const [results, setResults] = useState<TypesContextMenuAction[]>([]);
    const [position, setPosition] = useState<ContextMenuPosition>({ top: 0, left: 0 });
    const [loading, setLoading] = useState(false);
    const [selectedIndex, setSelectedIndex] = useState(0);
    const menuRef = useRef<HTMLDivElement>(null);
    const inputRef = useRef<HTMLInputElement>(null);
    const coordsRef = useRef({ x: 0, y: 0 });
    const api = useApi();
    const apiClient = api.getApiClient();

    // Get cursor position from text area or mouse position
    const getCursorPosition = useCallback((): ContextMenuPosition => {
        if (textAreaRef?.current) {
            const textarea = textAreaRef.current;
            const rect = textarea.getBoundingClientRect();
            const cursorPosition = textarea.selectionEnd || 0;
            const textBeforeCursor = textarea.value.substring(0, cursorPosition);

            // Create a mirror div to calculate cursor position
            const div = document.createElement('div');
            const computedStyle = window.getComputedStyle(textarea);
            
            // Copy all relevant styles from textarea to div
            const stylesToCopy = [
                'font-family', 'font-size', 'font-weight', 'font-style',
                'letter-spacing', 'text-transform', 'word-spacing', 'text-indent',
                'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
                'border-top-width', 'border-right-width', 'border-bottom-width', 'border-left-width',
                'box-sizing', 'width', 'line-height'
            ];
            
            stylesToCopy.forEach(style => {
                div.style[style as any] = computedStyle[style as any];
            });
            
            div.style.position = 'absolute';
            div.style.visibility = 'hidden';
            div.style.whiteSpace = 'pre-wrap';
            div.style.wordWrap = 'break-word';
            div.style.overflow = 'hidden';
            div.style.top = '0';
            div.style.left = '0';

            // Add the text before cursor
            div.textContent = textBeforeCursor;

            // Add a span at the cursor position to measure its coordinates
            const span = document.createElement('span');
            span.textContent = '|';
            div.appendChild(span);

            document.body.appendChild(div);

            // Get the span's position
            const spanRect = span.getBoundingClientRect();
            const divRect = div.getBoundingClientRect();

            document.body.removeChild(div);

            // Calculate absolute position considering scroll
            const top = rect.top + (spanRect.top - divRect.top) - textarea.scrollTop + 25;
            const left = rect.left + (spanRect.left - divRect.left) - textarea.scrollLeft;

            return {
                top: Math.max(0, top),
                left: Math.max(0, left)
            };
        } else {
            // Fallback to getting mouse position
            return {
                top: coordsRef.current.y,
                left: coordsRef.current.x
            };
        }
    }, [textAreaRef]);

    // Function to open the context menu
    const openContextMenu = useCallback(() => {
        setIsOpen(true);
        setQuery('');
        setPosition(getCursorPosition());

        // Blur the parent element to prevent the input from being focused
        if (textAreaRef?.current) {
            textAreaRef.current.blur();
        }
    }, [getCursorPosition, textAreaRef]);

    // Function to close the context menu
    const closeContextMenu = useCallback(() => {
        setIsOpen(false);
        setQuery('');
        setResults([]);
        setSelectedIndex(0);

        // Use setTimeout to ensure focus happens after React updates
        setTimeout(() => {
            if (textAreaRef?.current) {
                textAreaRef.current.focus();
            }
        }, 0);
    }, [textAreaRef]);

    // Fetch suggestions from API when query changes
    useEffect(() => {
        if (!isOpen || !appId) {
            setResults([]);
            return;
        }

        const fetchData = async () => {
            setLoading(true);
            try {
                const params: any = {
                    app_id: appId,
                }
                // Remove the @ symbol from the query
                const q = query.trim().replace('@', '')
                if (q) {
                    params.q = q;
                }
                const response = await apiClient.v1ContextMenuList(params);
                setResults(response.data.data || []);
            } catch (error) {
                console.error('Error fetching context menu data:', error);
                setResults([]);
            } finally {
                setLoading(false);
            }
        };

        const timeoutId = setTimeout(fetchData, 300);
        return () => clearTimeout(timeoutId);
    }, [query, isOpen, appId]);

    // Reset selectedIndex when results change
    useEffect(() => {
        setSelectedIndex(0);
    }, [results]);

    // Handle global mouse move events
    useEffect(() => {
        const handleWindowMouseMove = (event: MouseEvent) => {
            coordsRef.current = {
                x: event.clientX,
                y: event.clientY,
            };
        };
        window.addEventListener('mousemove', handleWindowMouseMove);

        return () => {
            window.removeEventListener(
                'mousemove',
                handleWindowMouseMove,
            );
        };
    }, []);

    // Handle global keydown events
    useEffect(() => {
        // Check for @ key to trigger the menu
        const handleKeyDown = (e: KeyboardEvent) => {
            if (!appId) return;
            if (!isOpen && e.key === '@') {
                // Only trigger if the focused element is the textarea we're listening to
                if (textAreaRef?.current && document.activeElement === textAreaRef.current) {
                    const textarea = textAreaRef.current;
                    const cursorPosition = textarea.selectionEnd || 0;
                    const textBeforeCursor = textarea.value.substring(0, cursorPosition);
                    
                    // Check if @ is at the start or preceded by whitespace
                    const isAtStart = textBeforeCursor.length === 0;
                    const isPrecededByWhitespace = textBeforeCursor.length > 0 && /\s$/.test(textBeforeCursor);
                    
                    if (!isAtStart && !isPrecededByWhitespace) {
                        // Don't trigger the menu - let the @ be typed normally
                        return;
                    }
                    
                    // Don't prevent default - let the @ be inserted
                    // We'll open the menu after the @ is in the textarea
                    setTimeout(() => {
                        openContextMenu();
                    }, 0);
                }
            }
        };

        // Add event listeners
        document.addEventListener('keydown', handleKeyDown);

        // Clean up
        return () => {
            document.removeEventListener('keydown', handleKeyDown);
        };
    }, [isOpen, results, selectedIndex, openContextMenu, closeContextMenu, appId, textAreaRef]);

    // Handle special control keys to navigate the menu
    const handleInputKeyDown = useCallback((e: React.KeyboardEvent) => {
        switch (e.key) {
            case 'Enter':
                e.preventDefault();
                const selectedItem = results[selectedIndex];
                if (selectedItem) {
                    handleSelect(selectedItem);
                }
                closeContextMenu();
                break;
            case 'Escape':
                e.preventDefault();
                closeContextMenu();
                break;
            case 'ArrowDown':
                e.preventDefault();
                if (results.length > 0) {
                    setSelectedIndex(prevIndex =>
                        prevIndex >= results.length - 1 ? 0 : prevIndex + 1
                    );
                }
                break;
            case 'ArrowUp':
                e.preventDefault();
                if (results.length > 0) {
                    setSelectedIndex(prevIndex =>
                        prevIndex <= 0 ? results.length - 1 : prevIndex - 1
                    );
                }
                break;
            default:
                // Otherwise ignore and let it propagate to the onChange handler
                e.stopPropagation();
        }
    }, [results, selectedIndex]);

    // Handle clicking outside to close the menu
    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
                closeContextMenu();
            }
        };

        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
        }

        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [isOpen, closeContextMenu]);

    // Handle selection of an item
    const handleSelect = useCallback((item: TypesContextMenuAction) => {
        if (!item.value) return;

        if (onInsertText) {
            onInsertText(item.value);
        }
        closeContextMenu();
    }, [onInsertText, closeContextMenu]);

    // Component to render the context menu
    const ContextMenuComponent = () => {
        const theme = useTheme();

        if (!isOpen) return null;

        // Group results by action
        const groupedResults: { [key: string]: TypesContextMenuAction[] } = {};
        results.forEach(item => {
            const action = item.action_label || 'Other';
            if (!groupedResults[action]) {
                groupedResults[action] = [];
            }
            groupedResults[action].push(item);
        });

        // Flatten grouped results for keyboard navigation
        const flattenedResults = results;

        return (
            <Menu
                open={isOpen}
                onClose={closeContextMenu}
                anchorReference="anchorPosition"
                anchorPosition={{ top: position.top, left: position.left }}
                anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                ref={menuRef}
                // Keep the menu mounted when open to prevent flickering
                keepMounted={false}
                // Prevent restoring focus which can cause flickering
                disableRestoreFocus
                // Add transition to make it smoother
                TransitionProps={{ timeout: 0 }}
                // Important styles to prevent flickering
                MenuListProps={{
                    disablePadding: true,
                    sx: {
                        minWidth: 240,
                        maxHeight: 400,
                        overflow: 'auto'
                    }
                }}
            >
                <Box sx={{ p: 1 }}>
                    <Paper>
                        <InputBase
                            autoFocus
                            placeholder="Search..."
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            inputRef={inputRef}
                            fullWidth
                            sx={{ px: 1, py: 0.5 }}
                            onKeyDown={handleInputKeyDown}
                        />
                    </Paper>
                </Box>

                <MenuList>
                    {loading && (
                        <MenuItem disabled>
                            <Typography>Loading...</Typography>
                        </MenuItem>
                    )}

                    {!loading && results.length === 0 && (
                        <MenuItem disabled>
                            <Typography>No results</Typography>
                        </MenuItem>
                    )}

                    {Object.entries(groupedResults).map(([action, items]) => (
                        <React.Fragment key={action}>
                            <ListSubheader
                                sx={{
                                    backgroundColor: theme.palette.background.paper,
                                    lineHeight: '30px',
                                    color: theme.palette.primary.main,
                                    fontWeight: 'bold',
                                    pl: 2
                                }}
                            >
                                {action}
                            </ListSubheader>

                            {items.map((item, itemIndex) => {
                                // Find the index of this item in the flattened results array
                                const flatIndex = flattenedResults.findIndex(
                                    r => r === item
                                );

                                return (
                                    <MenuItem
                                        key={`${action}-${itemIndex}`}
                                        onClick={() => handleSelect(item)}
                                        selected={flatIndex === selectedIndex}
                                        sx={{ pl: 3 }} // Indent items to show hierarchy
                                    >
                                        {item.label || item.value}
                                    </MenuItem>
                                );
                            })}

                            {/* Add divider between groups */}
                            {Object.keys(groupedResults).indexOf(action) <
                                Object.keys(groupedResults).length - 1 && (
                                    <Divider />
                                )}
                        </React.Fragment>
                    ))}
                </MenuList>
            </Menu>
        );
    };

    return {
        isOpen,
        openContextMenu,
        closeContextMenu,
        ContextMenuComponent
    };
};

// Main component that can be used in other parts of the app
interface ContextMenuModalProps {
    appId: string;
    textAreaRef?: React.RefObject<HTMLTextAreaElement | HTMLInputElement | undefined>;
    onInsertText?: (text: string) => void;
    children?: React.ReactNode;
}

export const ContextMenuModal: React.FC<ContextMenuModalProps> = ({
    appId,
    textAreaRef,
    onInsertText,
    children
}) => {
    const { ContextMenuComponent } = useContextMenu({
        appId,
        textAreaRef,
        onInsertText
    });

    return (
        <>
            {children}
            <ContextMenuComponent />
        </>
    );
};

export default ContextMenuModal;
