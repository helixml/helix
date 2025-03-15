import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Grid from '@mui/material/Grid';
import TextField from '@mui/material/TextField';
import Window from '../widgets/Window';
import { IAssistantGPTScript } from '../../types';

// Type for the editing GPT script state
export type EditingGPTScriptType = {
  tool: IAssistantGPTScript;
  index: number;
} | null;

// Props interface for the GPTScriptEditor component
interface GPTScriptEditorProps {
  // Current script being edited
  editingGptScript: EditingGPTScriptType;
  // Function to update the editing script state
  setEditingGptScript: (script: EditingGPTScriptType) => void;
  // Function to save the script
  onSaveGptScript: (tool: IAssistantGPTScript, index: number) => void;
  // Whether to show validation errors
  showErrors: boolean;
  // Whether the form should be read-only
  isReadOnly: boolean;
}

/**
 * Modal dialog component for editing GPT scripts
 * Extracted from App.tsx to reduce component complexity
 */
const GPTScriptEditor: React.FC<GPTScriptEditorProps> = ({
  editingGptScript,
  setEditingGptScript,
  onSaveGptScript,
  showErrors,
  isReadOnly,
}) => {
  // If no script is being edited, don't render anything
  if (!editingGptScript) return null;

  // Handlers for updating script fields
  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!editingGptScript) return;
    
    setEditingGptScript({
      ...editingGptScript,
      tool: { 
        ...editingGptScript.tool, 
        name: e.target.value 
      }
    });
  };

  const handleDescriptionChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!editingGptScript) return;
    
    setEditingGptScript({
      ...editingGptScript,
      tool: { 
        ...editingGptScript.tool, 
        description: e.target.value 
      }
    });
  };

  const handleContentChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!editingGptScript) return;
    
    setEditingGptScript({
      ...editingGptScript,
      tool: { 
        ...editingGptScript.tool, 
        content: e.target.value 
      }
    });
  };

  return (
    <Window
      title={`${editingGptScript.tool.name ? 'Edit' : 'Add'} GPTScript`}
      fullHeight
      size="lg"
      open
      withCancel
      cancelTitle="Close"
      onCancel={() => setEditingGptScript(null)}
      onSubmit={() => {
        if (editingGptScript?.tool) {
          onSaveGptScript(editingGptScript.tool, editingGptScript.index);
        }
      }}
    >
      <Box sx={{ p: 2 }}>
        <Typography variant="h6" sx={{ mb: 2 }}>
          GPTScript
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            <TextField
              value={editingGptScript?.tool.name}
              onChange={handleNameChange}
              label="Name"
              fullWidth
              error={showErrors && !editingGptScript?.tool.name}
              helperText={showErrors && !editingGptScript?.tool.name ? 'Please enter a name' : ''}
              disabled={isReadOnly}
            />
          </Grid>
          <Grid item xs={12}>
            <TextField
              value={editingGptScript?.tool.description}
              onChange={handleDescriptionChange}
              label="Description"
              fullWidth
              error={showErrors && !editingGptScript?.tool.description}
              helperText={showErrors && !editingGptScript?.tool.description ? "Description is required" : ""}
              disabled={isReadOnly}
            />
          </Grid>
          <Grid item xs={12}>
            <TextField
              value={editingGptScript?.tool.content}
              onChange={handleContentChange}
              label="Script Content"
              fullWidth
              multiline
              rows={10}
              error={showErrors && !editingGptScript?.tool.content}
              helperText={showErrors && !editingGptScript?.tool.content ? "Script content is required" : ""}
              disabled={isReadOnly}
            />
          </Grid>
        </Grid>
      </Box>
    </Window>
  );
};

export default GPTScriptEditor; 