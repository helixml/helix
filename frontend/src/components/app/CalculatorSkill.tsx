import React, { useState, useEffect } from 'react';
import {
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Alert,
} from '@mui/material';
import { IAppFlatState } from '../../types';
import { TypesAssistantCalculator } from '../../api/api';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';

interface CalculatorSkillProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

const NameTypography = styled(Typography)(({ theme }) => ({
  fontSize: '2rem',
  fontWeight: 700,
  color: '#F8FAFC',
  marginBottom: theme.spacing(1),
}));

const DescriptionTypography = styled(Typography)(({ theme }) => ({
  fontSize: '1.1rem',
  color: '#A0AEC0',
  marginBottom: theme.spacing(3),
}));

const CalculatorSkill: React.FC<CalculatorSkillProps> = ({
  open,
  onClose,
  onClosed,
  app,
  onUpdate,
}) => {
  const lightTheme = useLightTheme();
  const [error, setError] = useState<string | null>(null);
  const [calculatorConfig, setCalculatorConfig] = useState<TypesAssistantCalculator>({
    enabled: false,
  });

  useEffect(() => {
    if (app.calculatorTool) {
      setCalculatorConfig(app.calculatorTool);
    } else {
      setCalculatorConfig({
        enabled: false,
      });
    }
  }, [app.calculatorTool]);

  const handleEnable = async () => {
    try {
      setError(null);
      
      const appCopy = JSON.parse(JSON.stringify(app));

      const updatedConfig = {
        enabled: true,
      };
      
      appCopy.calculatorTool = updatedConfig;
      
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save calculator configuration');
    }
  };

  const handleDisable = async () => {
    try {
      const appCopy = JSON.parse(JSON.stringify(app));
      
      appCopy.calculatorTool = {
        enabled: false,
      };
      
      await onUpdate(appCopy);
      
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to disable calculator');
    }
  };

  const handleClose = () => {
    onClose();
  };

  return (
    <DarkDialog 
      open={open} 
      onClose={handleClose} 
      maxWidth="md" 
      fullWidth
      TransitionProps={{
        onExited: () => {
          setCalculatorConfig({
            enabled: false,
          });
          setError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ mt: 2 }}>
          <NameTypography>
            Calculator Skill
          </NameTypography>
          <DescriptionTypography>
            This skill provides the AI with the ability to perform math calculations using JavaScript expressions. It is useful when the LLM needs to perform slightly more complex calculations.
          </DescriptionTypography>
        </Box>
      </DialogContent>
      <DialogActions sx={{ background: '#181A20', borderTop: '1px solid #23262F', flexDirection: 'column', alignItems: 'stretch' }}>
        {error && (
          <Box sx={{ width: '100%', pl: 2, pr: 2, mb: 3 }}>
            <Alert variant="outlined" severity="error" sx={{ width: '100%' }}>
              {error}
            </Alert>
          </Box>
        )}
        <Box sx={{ display: 'flex', width: '100%' }}>
          <Button 
            onClick={handleClose} 
            size="small"
            variant="outlined"
            color="primary"
          >
            Cancel
          </Button>
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 1, mr: 2 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>
              {calculatorConfig.enabled ? (
                <Button
                  onClick={handleDisable}
                  size="small"
                  variant="outlined"
                  color="error"
                  sx={{ borderColor: '#EF4444', color: '#EF4444', '&:hover': { borderColor: '#DC2626', color: '#DC2626' } }}
                >
                  Disable
                </Button>
              ) : (
                <Button
                  onClick={handleEnable}
                  size="small"
                  variant="outlined"
                  color="secondary"
                >
                  Enable
                </Button>
              )}
            </Box>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default CalculatorSkill;
