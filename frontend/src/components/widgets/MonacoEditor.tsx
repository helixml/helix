import React, { useRef, useEffect, useState } from 'react';
import * as monaco from 'monaco-editor';
import { Box, BoxProps } from '@mui/material';
import useThemeConfig from '../../hooks/useThemeConfig';

interface MonacoEditorProps extends Omit<BoxProps, 'onChange'> {
  value: string;
  onChange: (value: string) => void;
  language?: string;
  readOnly?: boolean;
  height?: string | number;
  minHeight?: string | number;
  maxHeight?: string | number;
  autoHeight?: boolean;
  theme?: 'vs-dark' | 'vs-light' | 'hc-black' | 'helix-dark';
  options?: monaco.editor.IStandaloneEditorConstructionOptions;
  onMount?: (editor: monaco.editor.IStandaloneCodeEditor) => void;
  onSave?: () => void; // Called when user presses Cmd+S / Ctrl+S
  onTest?: () => void; // Called when user presses Cmd+Enter / Ctrl+Enter
}

// Custom Helix theme for Monaco Editor
const createHelixTheme = (themeConfig: any) => ({
  base: 'vs-dark' as const,
  inherit: true,
  rules: [
    { token: 'comment', foreground: themeConfig.neutral400, fontStyle: 'italic' },
    { token: 'keyword', foreground: themeConfig.tealRoot, fontStyle: 'bold' },
    { token: 'string', foreground: themeConfig.yellowRoot },
    { token: 'number', foreground: themeConfig.magentaRoot },
    { token: 'regexp', foreground: themeConfig.greenRoot },
    { token: 'operator', foreground: themeConfig.tealRoot },
    { token: 'namespace', foreground: themeConfig.neutral300 },
    { token: 'type', foreground: themeConfig.tealRoot },
    { token: 'struct', foreground: themeConfig.tealRoot },
    { token: 'class', foreground: themeConfig.tealRoot },
    { token: 'interface', foreground: themeConfig.tealRoot },
    { token: 'parameter', foreground: themeConfig.neutral300 },
    { token: 'variable', foreground: themeConfig.neutral300 },
    { token: 'function', foreground: themeConfig.tealRoot },
    { token: 'method', foreground: themeConfig.tealRoot },
    { token: 'property', foreground: themeConfig.neutral300 },
    { token: 'constant', foreground: themeConfig.magentaRoot },
    { token: 'tag', foreground: themeConfig.tealRoot },
    { token: 'attribute.name', foreground: themeConfig.neutral300 },
    { token: 'attribute.value', foreground: themeConfig.yellowRoot },
    { token: 'delimiter', foreground: themeConfig.neutral400 },
    { token: 'delimiter.bracket', foreground: themeConfig.neutral300 },
    { token: 'delimiter.parenthesis', foreground: themeConfig.neutral300 },
    { token: 'delimiter.square', foreground: themeConfig.neutral300 },
    { token: 'delimiter.curly', foreground: themeConfig.neutral300 },
    { token: 'delimiter.angle', foreground: themeConfig.neutral300 },
    { token: 'punctuation', foreground: themeConfig.neutral400 },
    { token: 'punctuation.definition', foreground: themeConfig.neutral400 },
    { token: 'punctuation.terminator', foreground: themeConfig.neutral400 },
    { token: 'punctuation.separator', foreground: themeConfig.neutral400 },
    { token: 'punctuation.accessor', foreground: themeConfig.neutral400 },
    { token: 'identifier', foreground: themeConfig.neutral300 },
    { token: 'entity.name', foreground: themeConfig.tealRoot },
    { token: 'entity.other', foreground: themeConfig.neutral300 },
    { token: 'support.type', foreground: themeConfig.tealRoot },
    { token: 'support.class', foreground: themeConfig.tealRoot },
    { token: 'support.function', foreground: themeConfig.tealRoot },
    { token: 'support.constant', foreground: themeConfig.magentaRoot },
    { token: 'support.variable', foreground: themeConfig.neutral300 },
    { token: 'support.parameter', foreground: themeConfig.neutral300 },
    { token: 'support.property', foreground: themeConfig.neutral300 },
    { token: 'support.method', foreground: themeConfig.tealRoot },
    { token: 'invalid', foreground: themeConfig.redRoot },
    { token: 'invalid.deprecated', foreground: themeConfig.yellowRoot },
    { token: 'invalid.broken', foreground: themeConfig.redRoot },
    { token: 'invalid.unimplemented', foreground: themeConfig.redRoot },
    { token: 'invalid.duplicate', foreground: themeConfig.redRoot },
    { token: 'invalid.argument', foreground: themeConfig.redRoot },
    { token: 'invalid.return', foreground: themeConfig.redRoot },
    { token: 'invalid.control', foreground: themeConfig.redRoot },
    { token: 'invalid.operator', foreground: themeConfig.redRoot },
    { token: 'invalid.character', foreground: themeConfig.redRoot },
    { token: 'invalid.escape', foreground: themeConfig.redRoot },
    { token: 'invalid.string', foreground: themeConfig.redRoot },
    { token: 'invalid.number', foreground: themeConfig.redRoot },
    { token: 'invalid.regexp', foreground: themeConfig.redRoot },
    { token: 'invalid.attribute', foreground: themeConfig.redRoot },
    { token: 'invalid.attribute.name', foreground: themeConfig.redRoot },
    { token: 'invalid.attribute.value', foreground: themeConfig.redRoot },
    { token: 'invalid.delimiter', foreground: themeConfig.redRoot },
    { token: 'invalid.delimiter.bracket', foreground: themeConfig.redRoot },
    { token: 'invalid.delimiter.parenthesis', foreground: themeConfig.redRoot },
    { token: 'invalid.delimiter.square', foreground: themeConfig.redRoot },
    { token: 'invalid.delimiter.curly', foreground: themeConfig.redRoot },
    { token: 'invalid.delimiter.angle', foreground: themeConfig.redRoot },
    { token: 'invalid.punctuation', foreground: themeConfig.redRoot },
    { token: 'invalid.punctuation.definition', foreground: themeConfig.redRoot },
    { token: 'invalid.punctuation.terminator', foreground: themeConfig.redRoot },
    { token: 'invalid.punctuation.separator', foreground: themeConfig.redRoot },
    { token: 'invalid.punctuation.accessor', foreground: themeConfig.redRoot },
    { token: 'invalid.identifier', foreground: themeConfig.redRoot },
    { token: 'invalid.entity.name', foreground: themeConfig.redRoot },
    { token: 'invalid.entity.other', foreground: themeConfig.redRoot },
    { token: 'invalid.support.type', foreground: themeConfig.redRoot },
    { token: 'invalid.support.class', foreground: themeConfig.redRoot },
    { token: 'invalid.support.function', foreground: themeConfig.redRoot },
    { token: 'invalid.support.constant', foreground: themeConfig.redRoot },
    { token: 'invalid.support.variable', foreground: themeConfig.redRoot },
    { token: 'invalid.support.parameter', foreground: themeConfig.redRoot },
    { token: 'invalid.support.property', foreground: themeConfig.redRoot },
    { token: 'invalid.support.method', foreground: themeConfig.redRoot },
  ],
  colors: {
    'editor.background': themeConfig.darkPanel,
    'editor.foreground': themeConfig.darkText,
    'editorLineNumber.foreground': themeConfig.neutral500,
    'editorLineNumber.activeForeground': themeConfig.neutral300,
    'editor.selectionBackground': `${themeConfig.tealRoot}30`,
    'editor.selectionHighlightBackground': `${themeConfig.tealRoot}20`,
    'editor.inactiveSelectionBackground': `${themeConfig.tealRoot}20`,
    'editorCursor.foreground': themeConfig.tealRoot,
    'editorWhitespace.foreground': themeConfig.neutral600,
    'editorIndentGuide.background': themeConfig.neutral600,
    'editorIndentGuide.activeBackground': themeConfig.neutral500,
    'editorLineHighlight.background': `${themeConfig.neutral600}40`,
    'editorBracketMatch.background': `${themeConfig.tealRoot}20`,
    'editorBracketMatch.border': themeConfig.tealRoot,
    'editor.findMatchBackground': `${themeConfig.yellowRoot}40`,
    'editor.findMatchHighlightBackground': `${themeConfig.yellowRoot}20`,
    'editor.hoverHighlightBackground': `${themeConfig.tealRoot}20`,
    'editor.wordHighlightBackground': `${themeConfig.tealRoot}20`,
    'editor.wordHighlightStrongBackground': `${themeConfig.tealRoot}30`,
    'editorBracketHighlight.foreground1': themeConfig.tealRoot,
    'editorBracketHighlight.foreground2': themeConfig.magentaRoot,
    'editorBracketHighlight.foreground3': themeConfig.greenRoot,
    'editorBracketHighlight.foreground4': themeConfig.yellowRoot,
    'editorBracketHighlight.foreground5': themeConfig.redRoot,
    'editorBracketHighlight.foreground6': themeConfig.neutral300,
    'editorError.foreground': themeConfig.redRoot,
    'editorWarning.foreground': themeConfig.yellowRoot,
    'editorInfo.foreground': themeConfig.tealRoot,
    'editorHint.foreground': themeConfig.neutral400,
    'editorGutter.background': themeConfig.darkPanel,
    'editorGutter.modifiedBackground': themeConfig.yellowRoot,
    'editorGutter.addedBackground': themeConfig.greenRoot,
    'editorGutter.deletedBackground': themeConfig.redRoot,
    'editorWidget.background': themeConfig.darkPanel,
    'editorWidget.border': themeConfig.darkBorder,
    'editorWidget.foreground': themeConfig.darkText,
    'editorWidget.shadow': '0 2px 8px rgba(0,0,0,0.3)',
    'editorSuggestWidget.background': themeConfig.darkPanel,
    'editorSuggestWidget.border': themeConfig.darkBorder,
    'editorSuggestWidget.foreground': themeConfig.darkText,
    'editorSuggestWidget.highlightForeground': themeConfig.tealRoot,
    'editorSuggestWidget.selectedBackground': `${themeConfig.tealRoot}20`,
    'editorHoverWidget.background': themeConfig.darkPanel,
    'editorHoverWidget.border': themeConfig.darkBorder,
    'editorHoverWidget.foreground': themeConfig.darkText,
    'editorHoverWidget.highlightForeground': themeConfig.tealRoot,
    'editorHoverWidget.statusBarBackground': themeConfig.neutral600,
    'editorMarkerNavigation.background': themeConfig.darkPanel,
    'editorMarkerNavigationError.background': themeConfig.redRoot,
    'editorMarkerNavigationWarning.background': themeConfig.yellowRoot,
    'editorMarkerNavigationInfo.background': themeConfig.tealRoot,
    'editorOverviewRuler.border': 'transparent',
    'editorOverviewRuler.findMatchForeground': themeConfig.yellowRoot,
    'editorOverviewRuler.rangeHighlightForeground': themeConfig.tealRoot,
    'editorOverviewRuler.selectionHighlightForeground': themeConfig.tealRoot,
    'editorOverviewRuler.wordHighlightForeground': themeConfig.tealRoot,
    'editorOverviewRuler.bracketMatchForeground': themeConfig.tealRoot,
    'editorOverviewRuler.errorForeground': themeConfig.neutral500,
    'editorOverviewRuler.warningForeground': themeConfig.yellowRoot,
    'editorOverviewRuler.infoForeground': themeConfig.tealRoot,
    'editorOverviewRuler.currentContentForeground': themeConfig.tealRoot,
    'editorOverviewRuler.incomingContentForeground': themeConfig.greenRoot,
    'editorOverviewRuler.commonContentForeground': themeConfig.neutral400,
    'editorRuler.foreground': themeConfig.neutral600,
    'editorCodeLens.foreground': themeConfig.neutral400,
    'editorLightBulb.foreground': themeConfig.yellowRoot,
    'editorLightBulbAutoFix.foreground': themeConfig.tealRoot,
    'editorBracketPairGuide.background1': themeConfig.neutral600,
    'editorBracketPairGuide.background2': themeConfig.neutral600,
    'editorBracketPairGuide.background3': themeConfig.neutral600,
    'editorBracketPairGuide.background4': themeConfig.neutral600,
    'editorBracketPairGuide.background5': themeConfig.neutral600,
    'editorBracketPairGuide.background6': themeConfig.neutral600,
    'editorBracketPairGuide.activeBackground1': themeConfig.tealRoot,
    'editorBracketPairGuide.activeBackground2': themeConfig.magentaRoot,
    'editorBracketPairGuide.activeBackground3': themeConfig.greenRoot,
    'editorBracketPairGuide.activeBackground4': themeConfig.yellowRoot,
    'editorBracketPairGuide.activeBackground5': themeConfig.redRoot,
    'editorBracketPairGuide.activeBackground6': themeConfig.neutral300,
    'editorUnicodeHighlight.background': `${themeConfig.yellowRoot}40`,
    'editorUnicodeHighlight.border': themeConfig.yellowRoot,
    'editorUnicodeHighlight.foreground': themeConfig.darkText,
    'editorUnicodeHighlight.foreground1': themeConfig.tealRoot,
    'editorUnicodeHighlight.foreground2': themeConfig.magentaRoot,
    'editorUnicodeHighlight.foreground3': themeConfig.greenRoot,
    'editorUnicodeHighlight.foreground4': themeConfig.yellowRoot,
    'editorUnicodeHighlight.foreground5': themeConfig.redRoot,
    'editorUnicodeHighlight.foreground6': themeConfig.neutral300,
    'editorUnicodeHighlight.foreground7': themeConfig.neutral400,
    'editorUnicodeHighlight.foreground8': themeConfig.neutral500,
    'editorUnicodeHighlight.foreground9': themeConfig.neutral600,
    'editorUnicodeHighlight.foreground10': themeConfig.neutral700,
    'editorUnicodeHighlight.foreground11': themeConfig.neutral800,
    'editorUnicodeHighlight.foreground12': themeConfig.neutral900,
    'editorUnicodeHighlight.foreground13': themeConfig.darkText,
    'editorUnicodeHighlight.foreground14': themeConfig.darkTextFaded,
    'editorUnicodeHighlight.foreground15': themeConfig.neutral300,
    'editorUnicodeHighlight.foreground16': themeConfig.neutral400,
    'editorUnicodeHighlight.foreground17': themeConfig.neutral500,
    'editorUnicodeHighlight.foreground18': themeConfig.neutral600,
    'editorUnicodeHighlight.foreground19': themeConfig.neutral700,
    'editorUnicodeHighlight.foreground20': themeConfig.neutral800,
    'editorUnicodeHighlight.foreground21': themeConfig.neutral900,
    'editorUnicodeHighlight.foreground22': themeConfig.darkText,
    'editorUnicodeHighlight.foreground23': themeConfig.darkTextFaded,
    'editorUnicodeHighlight.foreground24': themeConfig.neutral300,
    'editorUnicodeHighlight.foreground25': themeConfig.neutral400,
    'editorUnicodeHighlight.foreground26': themeConfig.neutral500,
    'editorUnicodeHighlight.foreground27': themeConfig.neutral600,
    'editorUnicodeHighlight.foreground28': themeConfig.neutral700,
    'editorUnicodeHighlight.foreground29': themeConfig.neutral800,
    'editorUnicodeHighlight.foreground30': themeConfig.neutral900,
    'editorUnicodeHighlight.foreground31': themeConfig.darkText,
    'editorUnicodeHighlight.foreground32': themeConfig.darkTextFaded,
    'editorUnicodeHighlight.foreground33': themeConfig.neutral300,
    'editorUnicodeHighlight.foreground34': themeConfig.neutral400,
    'editorUnicodeHighlight.foreground35': themeConfig.neutral500,
    'editorUnicodeHighlight.foreground36': themeConfig.neutral600,
    'editorUnicodeHighlight.foreground37': themeConfig.neutral700,
    'editorUnicodeHighlight.foreground38': themeConfig.neutral800,
    'editorUnicodeHighlight.foreground39': themeConfig.neutral900,
    'editorUnicodeHighlight.foreground40': themeConfig.darkText,
    'editorUnicodeHighlight.foreground41': themeConfig.darkTextFaded,
    'editorUnicodeHighlight.foreground42': themeConfig.neutral300,
    'editorUnicodeHighlight.foreground43': themeConfig.neutral400,
    'editorUnicodeHighlight.foreground44': themeConfig.neutral500,
    'editorUnicodeHighlight.foreground45': themeConfig.neutral600,
    'editorUnicodeHighlight.foreground46': themeConfig.neutral700,
    'editorUnicodeHighlight.foreground47': themeConfig.neutral800,
    'editorUnicodeHighlight.foreground48': themeConfig.neutral900,
    'editorUnicodeHighlight.foreground49': themeConfig.darkText,
    'editorUnicodeHighlight.foreground50': themeConfig.darkTextFaded,
  },
});

const MonacoEditor: React.FC<MonacoEditorProps> = ({
  value,
  onChange,
  language = 'yaml',
  readOnly = false,
  height = 'auto',
  minHeight = 200,
  maxHeight = 800,
  autoHeight = true,
  theme = 'helix-dark',
  options = {},
  onMount,
  onSave,
  onTest,
  ...boxProps
}) => {
  const editorRef = useRef<HTMLDivElement>(null);
  const editorInstanceRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const [isEditorReady, setIsEditorReady] = useState(false);
  const themeConfig = useThemeConfig();

  // Use refs to always call latest callbacks (avoids stale closures in Monaco commands)
  const onSaveRef = useRef(onSave);
  const onTestRef = useRef(onTest);

  useEffect(() => {
    onSaveRef.current = onSave;
    onTestRef.current = onTest;
  }, [onSave, onTest]);

  useEffect(() => {
    if (!editorRef.current) return;

    // Register custom theme if it's the helix-dark theme
    if (theme === 'helix-dark') {
      const helixTheme = createHelixTheme(themeConfig);
      monaco.editor.defineTheme('helix-dark', helixTheme);
    }

    // Create editor instance
    const editor = monaco.editor.create(editorRef.current, {
      value,
      language,
      theme,
      readOnly,
      automaticLayout: true,
      scrollBeyondLastLine: false,
      minimap: { enabled: false },
      fontSize: 14,
      lineHeight: 20,
      fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
      wordWrap: 'on',
      wrappingIndent: 'indent',
      ...options,
    });

    editorInstanceRef.current = editor;
    setIsEditorReady(true);

    // Set up content change listener
    const disposable = editor.onDidChangeModelContent(() => {
      const newValue = editor.getValue();
      onChange(newValue);
    });

    // Call onMount callback if provided
    if (onMount) {
      onMount(editor);
    }

    // Register Cmd+S / Ctrl+S keyboard shortcut
    // Use ref to always call latest callback (prevents stale closures)
    if (onSave) {
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
        onSaveRef.current?.();
      });
    }

    // Register Cmd+Enter / Ctrl+Enter keyboard shortcut
    // Use ref to always call latest callback (prevents stale closures)
    if (onTest) {
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
        onTestRef.current?.();
      });
    }

    return () => {
      disposable.dispose();
      editor.dispose();
      editorInstanceRef.current = null;
      setIsEditorReady(false);
    };
  }, []);

  // Update editor value when prop changes
  useEffect(() => {
    if (editorInstanceRef.current && isEditorReady) {
      const currentValue = editorInstanceRef.current.getValue();
      if (currentValue !== value) {
        editorInstanceRef.current.setValue(value);
      }
    }
  }, [value, isEditorReady]);

  // Update read-only state
  useEffect(() => {
    if (editorInstanceRef.current) {
      editorInstanceRef.current.updateOptions({ readOnly });
    }
  }, [readOnly]);

  // Auto-resize functionality
  useEffect(() => {
    if (!editorInstanceRef.current || !autoHeight) return;

    const updateHeight = () => {
      const editor = editorInstanceRef.current;
      if (!editor) return;

      const contentHeight = editor.getContentHeight();
      const newHeight = Math.max(
        typeof minHeight === 'number' ? minHeight : parseInt(minHeight as string),
        Math.min(
          typeof maxHeight === 'number' ? maxHeight : parseInt(maxHeight as string),
          contentHeight
        )
      );

      editorRef.current!.style.height = `${newHeight}px`;
      editor.layout();
    };

    // Initial height calculation
    updateHeight();

    // Listen for content changes to update height
    const disposable = editorInstanceRef.current.onDidContentSizeChange(updateHeight);

    return () => {
      disposable.dispose();
    };
  }, [autoHeight, minHeight, maxHeight, isEditorReady]);

  // Update theme
  useEffect(() => {
    if (editorInstanceRef.current) {
      // Register custom theme if it's the helix-dark theme
      if (theme === 'helix-dark') {
        const helixTheme = createHelixTheme(themeConfig);
        monaco.editor.defineTheme('helix-dark', helixTheme);
      }
      monaco.editor.setTheme(theme);
    }
  }, [theme, themeConfig]);

  return (
    <Box
      ref={editorRef}
      sx={{
        width: '100%',
        height: autoHeight ? 'auto' : height,
        minHeight: autoHeight ? minHeight : undefined,
        maxHeight: autoHeight ? maxHeight : undefined,
        border: '1px solid #303047',
        borderRadius: 1,
        overflow: 'hidden',
        '& .monaco-editor': {
          borderRadius: 'inherit',
        },
        ...boxProps.sx,
      }}
      {...boxProps}
    />
  );
};

export default MonacoEditor;
