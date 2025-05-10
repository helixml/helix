import React, { FC, useState, useEffect, useCallback } from 'react'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import IconButton from '@mui/material/IconButton'
import SendIcon from '@mui/icons-material/Send'
import Avatar from '@mui/material/Avatar'
import ContextMenuModal from '../widgets/ContextMenuModal'

interface InputFieldProps {
  initialValue: string;
  onSubmit: (value: string) => void;
  disabled?: boolean;
  label: string;
  isBigScreen: boolean;
  activeAssistantAvatar?: string;
  themeConfig: any;
  theme: any;
  loading: boolean;
  inputRef: React.MutableRefObject<HTMLTextAreaElement | undefined>;
  appID?: string | null;
  onInsertText: (text: string) => void;
}

const InputField: FC<InputFieldProps> = React.memo(({
  initialValue,
  onSubmit,
  disabled,
  label,
  isBigScreen,
  activeAssistantAvatar,
  themeConfig,
  theme,
  loading,
  inputRef,
  appID,
  onInsertText,
}) => {
  // Use completely internal state - don't propagate changes back up to parent
  const [localValue, setLocalValue] = useState(initialValue);
  const [showContextMenu, setShowContextMenu] = useState(false);

  // Update local value when initialValue changes from parent
  useEffect(() => {
    setLocalValue(initialValue);
  }, [initialValue]);

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = event.target.value;
    // Only update local state, no callback to parent
    performance.mark('input-start');
    setLocalValue(newValue);
    
    // Check if the last character typed was '@' to show context menu
    if (newValue.slice(-1) === '@') {
      setShowContextMenu(true);
    }
    
    // Measure typing performance
    requestAnimationFrame(() => {
      try {
        performance.mark('input-end');
        // Check if marks exist before measuring
        if (performance.getEntriesByName('input-start', 'mark').length > 0 &&
            performance.getEntriesByName('input-end', 'mark').length > 0) {
          performance.measure('input-latency', 'input-start', 'input-end');
          const latency = performance.getEntriesByName('input-latency').pop()?.duration;          
        }
        // Clean up
        performance.clearMarks('input-start');
        performance.clearMarks('input-end');
        performance.clearMeasures('input-latency');
      } catch (e) {
        console.warn('Error in performance measurement:', e);
      }
    });
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === '@') {
      setShowContextMenu(true);
    } else if (event.key === 'Enter') {
      if (event.shiftKey) {
        setLocalValue(current => current + "\n");
      } else {
        if (!loading && !showContextMenu) {  // Don't submit if context menu is open
          const currentValue = localValue;
          // Clear input field immediately for better user experience
          setLocalValue('');
          // Then call the parent's submission handler
          onSubmit(currentValue);
        }
      }
      event.preventDefault();
    } else if (event.key === 'Escape') {
      setShowContextMenu(false);
    }
  };

  // Handle text insertion from context menu
  const handleInsertText = useCallback((text: string) => {
    const newValue = localValue + text + ' ';
    setLocalValue(newValue);
    onInsertText(newValue); // Call parent's onInsertText with the full new value
    setShowContextMenu(false); // Hide context menu after insertion
  }, [localValue, onInsertText]);

  return (
    <>
      <ContextMenuModal
        appId={appID || ''}
        textAreaRef={inputRef}
        onInsertText={handleInsertText}
      />
      <TextField
        id="textEntry"
        fullWidth
        inputRef={inputRef}
        autoFocus={true}
        label={label + ""}
        value={localValue}
        disabled={disabled}
        onChange={handleInputChange}
        name="ai_submit"
        multiline={true}
        onKeyDown={handleKeyDown}
        InputProps={{
          startAdornment: isBigScreen && (
            activeAssistantAvatar ? (
              <Avatar
                src={activeAssistantAvatar}
                sx={{
                  width: '30px',
                  height: '30px',
                  mr: 1,
                }}
              />
            ) : null
          ),
          endAdornment: (
            <InputAdornment position="end">
              <IconButton
                id="send-button"
                aria-label="send"
                disabled={disabled}
                onClick={() => {
                  const currentValue = localValue;
                  setLocalValue('');
                  onSubmit(currentValue);
                }}
                sx={{
                  color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                }}
              >
                <SendIcon />
              </IconButton>
            </InputAdornment>
          ),
        }}
      />
    </>
  );
}, (prevProps, nextProps) => {
  // Custom comparison function to prevent unnecessary re-renders
  return (
    prevProps.initialValue === nextProps.initialValue &&
    prevProps.disabled === nextProps.disabled &&
    prevProps.loading === nextProps.loading
  );
});

export default InputField; 