import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
    Menu,
    MenuItem,
    InputBase,
    Paper,
    MenuList,
    Box,
    Typography
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import useApi from '../../hooks/useApi';

// Define the context menu data structure based on the API response
interface ContextMenuData {
    label?: string;
    value?: string;
}

interface ContextMenuPosition {
    top: number;
    left: number;
}

interface UseContextMenuOptions {
    appId: string;
    textAreaRef?: React.RefObject<HTMLTextAreaElement | HTMLInputElement | undefined>;
    onInsertText?: (text: string) => void;
}

// Hook to manage context menu state and functionality
export const useContextMenu = ({
    appId,
    textAreaRef,
    onInsertText
}: UseContextMenuOptions) => {
    const [isOpen, setIsOpen] = useState(false);
    const [query, setQuery] = useState('');
    const [results, setResults] = useState<ContextMenuData[]>([]);
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

            // Get cursor position in textarea
            const cursorPosition = textarea.selectionEnd || 0;

            // Create a range to get the positioning
            const textBeforeCursor = textarea.value.substring(0, cursorPosition);

            // Create a temporary element to measure position
            const span = document.createElement('span');
            span.textContent = textBeforeCursor;
            span.style.position = 'absolute';
            span.style.visibility = 'hidden';
            span.style.whiteSpace = 'pre-wrap';
            span.style.wordWrap = 'break-word';
            span.style.font = window.getComputedStyle(textarea).font;
            span.style.width = window.getComputedStyle(textarea).width;

            document.body.appendChild(span);

            // Calculate position
            const rect = textarea.getBoundingClientRect();
            const lineHeight = parseInt(window.getComputedStyle(textarea).lineHeight);

            // Count newlines for vertical position
            const lines = textBeforeCursor.split('\n').length - 1;

            document.body.removeChild(span);

            return {
                top: rect.top + lines * lineHeight + 20, // Add some offset
                left: rect.left + span.offsetWidth % rect.width
            };
        } else {
            // Falback to getting mouse position
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

        // Return focus to the textarea if it exists
        if (textAreaRef?.current) {
            textAreaRef.current.focus();
        }
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
                e.preventDefault();
                e.stopPropagation();
                openContextMenu();
            }
        };

        // Add event listeners
        document.addEventListener('keydown', handleKeyDown);

        // Clean up
        return () => {
            document.removeEventListener('keydown', handleKeyDown);
        };
    }, [isOpen, results, selectedIndex, openContextMenu, closeContextMenu, appId]);

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
    const handleSelect = useCallback((item: ContextMenuData) => {
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

        return (
            <Menu
                open={isOpen}
                onClose={closeContextMenu}
                anchorReference="anchorPosition"
                anchorPosition={{ top: position.top, left: position.left }}
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
                        minWidth: 200,
                        maxHeight: 300,
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

                    {results.map((item, index) => (
                        <MenuItem
                            key={index}
                            onClick={() => handleSelect(item)}
                            selected={index === selectedIndex}
                        >
                            {item.label || item.value}
                        </MenuItem>
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
