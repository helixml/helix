import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { IAppFlatState, ITest, ITestStep } from '../../types';

interface TestsEditorProps {
  app: IAppFlatState;
  onUpdate: (updates: Partial<IAppFlatState>) => void;
  appId: string;
  navigate: (route: string) => void;
}

const TestsEditor: React.FC<TestsEditorProps> = ({
  app,
  onUpdate,
  appId,
  navigate,
}) => {
  const tests = app.tests || [];

  const handleAddTest = () => {
    const newTest: ITest = {
      name: '',
      steps: [{ prompt: '', expected_output: '' }]
    };
    onUpdate({ tests: [...tests, newTest] });
  };

  const handleDeleteTest = (testIndex: number) => {
    const updatedTests = tests.filter((_, index) => index !== testIndex);
    onUpdate({ tests: updatedTests });
  };

  const handleUpdateTest = (testIndex: number, updates: Partial<ITest>) => {
    const updatedTests = tests.map((test, index) =>
      index === testIndex ? { ...test, ...updates } : test
    );
    onUpdate({ tests: updatedTests });
  };

  const handleAddStep = (testIndex: number) => {
    const newStep: ITestStep = { prompt: '', expected_output: '' };
    const updatedTest = {
      ...tests[testIndex],
      steps: [...(tests[testIndex].steps || []), newStep]
    };
    handleUpdateTest(testIndex, updatedTest);
  };

  const handleDeleteStep = (testIndex: number, stepIndex: number) => {
    const updatedSteps = (tests[testIndex].steps || []).filter((_, index) => index !== stepIndex);
    handleUpdateTest(testIndex, { steps: updatedSteps });
  };

  const handleUpdateStep = (testIndex: number, stepIndex: number, updates: Partial<ITestStep>) => {
    const updatedSteps = (tests[testIndex].steps || []).map((step, index) =>
      index === stepIndex ? { ...step, ...updates } : step
    );
    handleUpdateTest(testIndex, { steps: updatedSteps });
  };

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Tests
      </Typography>
      
      <Typography variant="body2" sx={{ mb: 3, color: 'text.secondary' }}>
        Define tests to validate your app's behavior automatically. Each test consists of one or more steps with prompts and expected outputs.
      </Typography>

      {tests.map((test, testIndex) => (
        <Card key={testIndex} sx={{ mb: 3, backgroundColor: '#2a2d3e' }}>
          <CardContent>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <TextField
                fullWidth
                label="Test Name"
                value={test.name || ''}
                onChange={(e) => handleUpdateTest(testIndex, { name: e.target.value })}
                placeholder="e.g., addition_test"
                sx={{ mr: 2 }}
              />
              <IconButton
                onClick={() => handleDeleteTest(testIndex)}
                color="error"
                size="small"
              >
                <DeleteIcon />
              </IconButton>
            </Box>

            <Typography variant="subtitle2" sx={{ mb: 2 }}>
              Test Steps
            </Typography>

            {(test.steps || []).map((step, stepIndex) => (
              <Card key={stepIndex} sx={{ mb: 2, backgroundColor: '#1e1e2f' }}>
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'flex-start', mb: 2 }}>
                    <Typography variant="body2" sx={{ mr: 2, mt: 1, minWidth: '60px' }}>
                      Step {stepIndex + 1}
                    </Typography>
                    <IconButton
                      onClick={() => handleDeleteStep(testIndex, stepIndex)}
                      color="error"
                      size="small"
                      sx={{ ml: 'auto' }}
                    >
                      <DeleteIcon />
                    </IconButton>
                  </Box>
                  
                  <Grid container spacing={2}>
                    <Grid item xs={12} md={6}>
                      <TextField
                        fullWidth
                        multiline
                        rows={3}
                        label="Prompt"
                        value={step.prompt || ''}
                        onChange={(e) => handleUpdateStep(testIndex, stepIndex, { prompt: e.target.value })}
                        placeholder="User input or question..."
                      />
                    </Grid>
                    <Grid item xs={12} md={6}>
                      <TextField
                        fullWidth
                        multiline
                        rows={3}
                        label="Expected Output"
                        value={step.expected_output || ''}
                        onChange={(e) => handleUpdateStep(testIndex, stepIndex, { expected_output: e.target.value })}
                        placeholder="Expected assistant response..."
                      />
                    </Grid>
                  </Grid>
                </CardContent>
              </Card>
            ))}

            <Button
              startIcon={<AddIcon />}
              onClick={() => handleAddStep(testIndex)}
              variant="outlined"
              size="small"
              sx={{ mt: 1 }}
            >
              Add Step
            </Button>
          </CardContent>
        </Card>
      ))}

      <Button
        startIcon={<AddIcon />}
        onClick={handleAddTest}
        variant="contained"
        sx={{ mb: 4 }}
      >
        Add Test
      </Button>

      {/* CLI Instructions */}
      <Box sx={{ mt: 4, p: 3, backgroundColor: '#2a2d3e', borderRadius: 2 }}>
        <Typography variant="subtitle1" sx={{ mb: 2 }}>
          Running Tests with CLI
        </Typography>
        
        <Typography variant="body2" sx={{ mb: 2 }}>
          To run your tests, first export your app configuration to a YAML file:
        </Typography>
        
        <Box sx={{
          backgroundColor: '#1e1e2f',
          padding: '10px',
          borderRadius: '4px',
          fontFamily: 'monospace',
          fontSize: '0.9rem',
          mb: 2
        }}>
          helix app inspect {appId} &gt; my-app.yaml
        </Box>

        <Typography variant="body2" sx={{ mb: 2 }}>
          Then run your tests with:
        </Typography>
        
        <Box sx={{
          backgroundColor: '#1e1e2f',
          padding: '10px',
          borderRadius: '4px',
          fontFamily: 'monospace',
          fontSize: '0.9rem',
          mb: 3
        }}>
          helix test --file my-app.yaml
        </Box>
        <Typography variant="body2" sx={{ mb: 3 }}>
          Don't have the CLI installed? 
          <Link 
            onClick={() => navigate('account')}
            sx={{ ml: 1, textDecoration: 'underline', cursor: 'pointer' }}
          >
            Install it from your account page
          </Link>
        </Typography>

        <Typography variant="subtitle2" sx={{ mb: 2 }}>
          CI/CD Integration
        </Typography>

        <Typography variant="body2" sx={{ mb: 2 }}>
          Integrate testing into your CI/CD pipeline for continuous validation:
        </Typography>

        <Accordion sx={{ mb: 2, backgroundColor: '#1e1e2f' }}>
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            aria-controls="github-actions-content"
            id="github-actions-header"
          >
            <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
              GitHub Actions Example
            </Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Box sx={{
              backgroundColor: '#0d1117',
              padding: '12px',
              borderRadius: '4px',
              fontFamily: 'monospace',
              fontSize: '0.75rem',
              overflow: 'auto',
              whiteSpace: 'pre',
              lineHeight: 1.4
            }}>
{`name: Test Helix App
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install Helix CLI
        run: |
          curl -L "https://helixml.tech/install.sh" | bash
          echo "$HOME/.helix/bin" >> $GITHUB_PATH
      - name: Run Tests
        env:
          HELIX_API_KEY: \${{ secrets.HELIX_API_KEY }}
        run: helix test --file my-app.yaml`}
            </Box>
          </AccordionDetails>
        </Accordion>

        <Accordion sx={{ mb: 2, backgroundColor: '#1e1e2f' }}>
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            aria-controls="gitlab-ci-content"
            id="gitlab-ci-header"
          >
            <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
              GitLab CI Example
            </Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Box sx={{
              backgroundColor: '#0d1117',
              padding: '12px',
              borderRadius: '4px',
              fontFamily: 'monospace',
              fontSize: '0.75rem',
              overflow: 'auto',
              whiteSpace: 'pre',
              lineHeight: 1.4
            }}>
{`test-helix:
  stage: test
  image: ubuntu:latest
  before_script:
    - apt-get update && apt-get install -y curl
    - curl -L "https://helixml.tech/install.sh" | bash
    - export PATH="$HOME/.helix/bin:$PATH"
  script:
    - helix test --file my-app.yaml
  variables:
    HELIX_API_KEY: $HELIX_API_KEY`}
            </Box>
          </AccordionDetails>
        </Accordion>

        <Typography variant="body2" sx={{ mb: 2 }}>
          For more examples, see the{' '}
          <Link 
            href="https://github.com/helixml/testing-genai/blob/main/.github/workflows/helix.yml"
            target="_blank"
            rel="noopener noreferrer"
            sx={{ textDecoration: 'underline' }}
          >
            GitHub Actions example
          </Link>
          {' '}and{' '}
          <Link 
            href="https://github.com/helixml/helix-jira/blob/main/.gitlab-ci.yml"
            target="_blank"
            rel="noopener noreferrer"
            sx={{ textDecoration: 'underline' }}
          >
            GitLab CI example
          </Link>
          .
        </Typography>

      </Box>
    </Box>
  );
};

export default TestsEditor; 