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
import Tooltip from '@mui/material/Tooltip';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import CheckIcon from '@mui/icons-material/Check';
import { IAppFlatState, ITest, ITestStep } from '../../types';
import { generateYamlFilename } from '../../utils/format';
import useSnackbar from '../../hooks/useSnackbar';

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
  const yamlFilename = generateYamlFilename(app.name || 'app');
  const snackbar = useSnackbar();
  const [testCopied, setTestCopied] = React.useState(false);
  const [githubCopied, setGithubCopied] = React.useState(false);
  const [gitlabCopied, setGitlabCopied] = React.useState(false);

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

  const handleCopyCommand = (command: string) => {
    navigator.clipboard.writeText(command)
      .then(() => {
        setTestCopied(true);
        setTimeout(() => setTestCopied(false), 2000);
        snackbar.success('Command copied to clipboard');
      })
      .catch((error) => {
        console.error('Failed to copy:', error);
        snackbar.error('Failed to copy to clipboard');
      });
  };

  const handleCopyConfig = (config: string, setCopied: (value: boolean) => void, configType: string) => {
    navigator.clipboard.writeText(config)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
        snackbar.success(`${configType} config copied to clipboard`);
      })
      .catch((error) => {
        console.error('Failed to copy:', error);
        snackbar.error('Failed to copy to clipboard');
      });
  };

  const testCommand = `helix agent inspect ${appId} > ${yamlFilename}\nhelix test -f ${yamlFilename}`;

  const githubActionsConfig = `name: CI for GenAI
on:
  push:
    branches: [ "main" ]
  pull_request:
    branches: [ "main" ]
  workflow_dispatch:

env:
  HELIX_URL: \${{ vars.HELIX_URL }}
  HELIX_API_KEY: \${{ secrets.HELIX_API_KEY }}

jobs:
  test-and-comment:
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    permissions:
      pull-requests: write
    steps:
      - uses: actions/checkout@v4

      - name: Install helix CLI
        run: curl -sL -O https://get.helix.ml/install.sh && bash install.sh --cli -y

      - name: Test the helix agent
        run: helix test -f ${yamlFilename}

      - name: PR comment with file
        if: always()
        uses: thollander/actions-comment-pull-request@v3
        with:
          file-path: summary_latest.md

  deploy:
    runs-on: ubuntu-latest
    if: github.event_name == 'push' || github.event_name == 'workflow_dispatch'
    steps:
      - uses: actions/checkout@v4

      - name: Install helix CLI
        run: curl -sL -O https://get.helix.ml/install.sh && bash install.sh --cli -y

      - name: Test the helix agent
        run: helix test -f ${yamlFilename}

      - name: Apply changes
        if: success()
        run: helix apply -f ${yamlFilename}`;

  const gitlabCiConfig = `stages:
  - test
  - apply

variables:
  HELIX_URL: \${HELIX_URL}
  HELIX_API_KEY: \${HELIX_API_KEY}

test_job:
  stage: test
  image: ubuntu:latest
  script:
    - apt-get update && apt-get install -y curl sudo
    - curl -sL -O https://get.helix.ml/install.sh && bash install.sh --cli -y
    - helix test
  artifacts:
    paths:
      - summary_latest.md
    when: always

apply_job:
  stage: apply
  image: ubuntu:latest
  script:
    - apt-get update && apt-get install -y curl sudo
    - curl -sL -O https://get.helix.ml/install.sh && bash install.sh --cli -y
    - helix apply -f ${yamlFilename}
  only:
    - main
    - pushes
    - web

comment_job:
  stage: apply
  image: ubuntu:latest
  script:
    - echo "Commenting on merge request with test results"
    - |
      if [ -f summary_latest.md ]; then
        comment=\$(cat summary_latest.md)
        curl --request POST \\
          --header "PRIVATE-TOKEN: \${GITLAB_API_TOKEN}" \\
          --header "Content-Type: application/json" \\
          --data "{\\"body\\": \\"\${comment}\\"}" \\
          "\${CI_API_V4_URL}/projects/\${CI_PROJECT_ID}/merge_requests/\${CI_MERGE_REQUEST_IID}/notes"
      else
        echo "summary_latest.md not found"
      fi
  only:
    - merge_requests`;

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
          Export your app configuration and run tests with this one-liner:
        </Typography>
        
        <Box sx={{
          backgroundColor: '#1e1e2f',
          padding: '10px',
          borderRadius: '4px',
          fontFamily: 'monospace',
          fontSize: '0.9rem',
          mb: 2,
          position: 'relative',
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between'
        }}>
          <pre style={{ margin: 0, whiteSpace: 'pre-wrap', flex: 1 }}>{testCommand}</pre>
          <Tooltip title={testCopied ? "Copied!" : "Copy command"} placement="top">
            <IconButton
              onClick={() => handleCopyCommand(testCommand)}
              sx={{
                color: 'white',
                padding: '4px',
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                },
              }}
              size="small"
            >
              {testCopied ? <CheckIcon sx={{ fontSize: 16 }} /> : <ContentCopyIcon sx={{ fontSize: 16 }} />}
            </IconButton>
          </Tooltip>
        </Box>

        <Typography variant="body2" sx={{ mb: 2 }}>
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
          Integrate testing into your CI/CD pipeline for continuous validation. Start by adding your agent yaml to your git repo, then add configuration to your CI/CD pipeline:
        </Typography>

        <Accordion sx={{ mb: 2, backgroundColor: '#1e1e2f' }}>
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            aria-controls="github-actions-content"
            id="github-actions-header"
          >
            <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
              GitHub Actions Example (<span style={{ fontFamily: 'monospace' }}>.github/workflows/helix.yml</span>)
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
              lineHeight: 1.4,
              position: 'relative'
            }}>
              <Tooltip title={githubCopied ? "Copied!" : "Copy config"} placement="top">
                <IconButton
                  onClick={() => handleCopyConfig(githubActionsConfig, setGithubCopied, 'GitHub Actions')}
                  sx={{
                    position: 'absolute',
                    top: 8,
                    right: 8,
                    color: 'white',
                    padding: '4px',
                    '&:hover': {
                      backgroundColor: 'rgba(255, 255, 255, 0.1)',
                    },
                  }}
                  size="small"
                >
                  {githubCopied ? <CheckIcon sx={{ fontSize: 16 }} /> : <ContentCopyIcon sx={{ fontSize: 16 }} />}
                </IconButton>
              </Tooltip>
              {githubActionsConfig}
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
              GitLab CI Example (<span style={{ fontFamily: 'monospace' }}>.gitlab-ci.yml</span>)
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
              lineHeight: 1.4,
              position: 'relative'
            }}>
              <Tooltip title={gitlabCopied ? "Copied!" : "Copy config"} placement="top">
                <IconButton
                  onClick={() => handleCopyConfig(gitlabCiConfig, setGitlabCopied, 'GitLab CI')}
                  sx={{
                    position: 'absolute',
                    top: 8,
                    right: 8,
                    color: 'white',
                    padding: '4px',
                    '&:hover': {
                      backgroundColor: 'rgba(255, 255, 255, 0.1)',
                    },
                  }}
                  size="small"
                >
                  {gitlabCopied ? <CheckIcon sx={{ fontSize: 16 }} /> : <ContentCopyIcon sx={{ fontSize: 16 }} />}
                </IconButton>
              </Tooltip>
              {gitlabCiConfig}
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