import React from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Alert from '@mui/material/Alert';
import KnowledgeEditor from '../KnowledgeEditor';
import { IKnowledgeSource } from '../../types';

interface KnowledgeManagerProps {
  knowledgeSources: IKnowledgeSource[];
  onUpdate: (updatedKnowledge: IKnowledgeSource[]) => void;
  onRefresh: (id: string) => void;
  disabled: boolean;
  knowledgeList: IKnowledgeSource[];
  showErrors: boolean;
  knowledgeErrors: boolean;
}

const KnowledgeManager: React.FC<KnowledgeManagerProps> = ({
  knowledgeSources,
  onUpdate,
  onRefresh,
  disabled,
  knowledgeList,
  showErrors,
  knowledgeErrors,
}) => {
  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Knowledge Sources
      </Typography>
      <KnowledgeEditor
        knowledgeSources={knowledgeSources}
        onUpdate={onUpdate}
        onRefresh={onRefresh}
        disabled={disabled}
        knowledgeList={knowledgeList}
      />
      {knowledgeErrors && showErrors && (
        <Alert severity="error" sx={{ mt: 2 }}>
          Please specify at least one URL for each knowledge source.
        </Alert>
      )}
    </Box>
  );
};

export default KnowledgeManager;