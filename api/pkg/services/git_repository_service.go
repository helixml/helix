package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GitRepositoryService manages git repositories hosted on the Helix server
// Uses the filestore mount for persistent storage of git repositories
type GitRepositoryService struct {
	store           store.Store
	filestoreBase   string // Base path for filestore (e.g., "/tmp/helix/filestore")
	gitRepoBase     string // Base path for git repositories within filestore
	serverBaseURL   string // Base URL for git server (e.g., "http://api:8080")
	gitUserName     string // Default git user name
	gitUserEmail    string // Default git user email
	enableGitServer bool   // Whether to enable git server functionality
	testMode        bool   // Test mode for unit tests
}

// GitRepository represents a git repository hosted on the server
type GitRepository struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	OwnerID       string                 `json:"owner_id"`
	ProjectID     string                 `json:"project_id,omitempty"`
	SpecTaskID    string                 `json:"spec_task_id,omitempty"`
	RepoType      GitRepositoryType      `json:"repo_type"`
	Status        GitRepositoryStatus    `json:"status"`
	CloneURL      string                 `json:"clone_url"`
	LocalPath     string                 `json:"local_path"`
	DefaultBranch string                 `json:"default_branch"`
	Branches      []string               `json:"branches,omitempty"`
	LastActivity  time.Time              `json:"last_activity"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// GitRepositoryType defines the type of repository
type GitRepositoryType string

const (
	GitRepositoryTypeProject  GitRepositoryType = "project"   // User project repository
	GitRepositoryTypeSpecTask GitRepositoryType = "spec_task" // SpecTask-specific repository
	GitRepositoryTypeSample   GitRepositoryType = "sample"    // Sample/demo repository
	GitRepositoryTypeTemplate GitRepositoryType = "template"  // Template repository
)

// GitRepositoryStatus defines the status of a repository
type GitRepositoryStatus string

const (
	GitRepositoryStatusActive   GitRepositoryStatus = "active"
	GitRepositoryStatusArchived GitRepositoryStatus = "archived"
	GitRepositoryStatusDeleted  GitRepositoryStatus = "deleted"
)

// GitRepositoryCreateRequest represents a request to create a new repository
type GitRepositoryCreateRequest struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	RepoType      GitRepositoryType      `json:"repo_type"`
	OwnerID       string                 `json:"owner_id"`
	ProjectID     string                 `json:"project_id,omitempty"`
	SpecTaskID    string                 `json:"spec_task_id,omitempty"`
	InitialFiles  map[string]string      `json:"initial_files,omitempty"`
	DefaultBranch string                 `json:"default_branch,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// NewGitRepositoryService creates a new git repository service
func NewGitRepositoryService(
	store store.Store,
	filestoreBase string,
	serverBaseURL string,
	gitUserName string,
	gitUserEmail string,
) *GitRepositoryService {
	gitRepoBase := filepath.Join(filestoreBase, "git-repositories")

	// Ensure the git-repositories directory exists
	if err := os.MkdirAll(gitRepoBase, 0755); err != nil {
		log.Error().Err(err).Str("path", gitRepoBase).Msg("Failed to create git-repositories directory")
	}

	return &GitRepositoryService{
		store:           store,
		filestoreBase:   filestoreBase,
		gitRepoBase:     gitRepoBase,
		serverBaseURL:   strings.TrimSuffix(serverBaseURL, "/"),
		gitUserName:     gitUserName,
		gitUserEmail:    gitUserEmail,
		enableGitServer: true,
		testMode:        false,
	}
}

// SetTestMode enables or disables test mode
func (s *GitRepositoryService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// Initialize creates the git repository base directory and sets up git server
func (s *GitRepositoryService) Initialize(ctx context.Context) error {
	// Create git repositories base directory
	err := os.MkdirAll(s.gitRepoBase, 0755)
	if err != nil {
		return fmt.Errorf("failed to create git repositories directory: %w", err)
	}

	log.Info().
		Str("git_repo_base", s.gitRepoBase).
		Str("server_base_url", s.serverBaseURL).
		Msg("Initialized git repository service")

	return nil
}

// CreateRepository creates a new git repository
func (s *GitRepositoryService) CreateRepository(
	ctx context.Context,
	request *GitRepositoryCreateRequest,
) (*GitRepository, error) {
	// Generate repository ID
	repoID := s.generateRepositoryID(request.RepoType, request.Name)

	// Set default branch if not specified
	defaultBranch := request.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	// Create repository path
	repoPath := filepath.Join(s.gitRepoBase, repoID)

	// Create repository object
	gitRepo := &GitRepository{
		ID:            repoID,
		Name:          request.Name,
		Description:   request.Description,
		OwnerID:       request.OwnerID,
		ProjectID:     request.ProjectID,
		SpecTaskID:    request.SpecTaskID,
		RepoType:      request.RepoType,
		Status:        GitRepositoryStatusActive,
		CloneURL:      s.generateCloneURL(repoID),
		LocalPath:     repoPath,
		DefaultBranch: defaultBranch,
		LastActivity:  time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Metadata:      request.Metadata,
	}

	// Initialize git repository
	err := s.initializeGitRepository(repoPath, defaultBranch, request.InitialFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// Store repository metadata (if store supports it)
	err = s.storeRepositoryMetadata(ctx, gitRepo)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to store repository metadata")
	}

	log.Info().
		Str("repo_id", repoID).
		Str("repo_path", repoPath).
		Str("clone_url", gitRepo.CloneURL).
		Msg("Created git repository")

	return gitRepo, nil
}

// GetRepository retrieves repository information by ID
func (s *GitRepositoryService) GetRepository(ctx context.Context, repoID string) (*GitRepository, error) {
	repoPath := filepath.Join(s.gitRepoBase, repoID)

	// Check if repository exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("repository not found: %s", repoID)
	}

	// Try to get metadata from store first
	gitRepo, err := s.getRepositoryMetadata(ctx, repoID)
	if err != nil {
		// If not in store, create from filesystem
		gitRepo = &GitRepository{
			ID:        repoID,
			LocalPath: repoPath,
			CloneURL:  s.generateCloneURL(repoID),
			Status:    GitRepositoryStatusActive,
		}
	}

	// Update with current git information
	err = s.updateRepositoryFromGit(gitRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to update repository info from git: %w", err)
	}

	return gitRepo, nil
}

// ListRepositories lists all repositories
func (s *GitRepositoryService) ListRepositories(ctx context.Context, ownerID string) ([]*GitRepository, error) {
	// Try to list from database first
	if postgresStore, ok := s.store.(interface {
		ListGitRepositories(ctx context.Context, ownerID string) ([]*GitRepository, error)
	}); ok {
		repos, err := postgresStore.ListGitRepositories(ctx, ownerID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to list repositories from database, falling back to filesystem scan")
		} else {
			log.Info().
				Int("count", len(repos)).
				Str("owner_id", ownerID).
				Msg("Listed repositories from database")
			return repos, nil
		}
	}

	// Fallback to filesystem scan if database not available
	repositories := []*GitRepository{}

	// Walk the git repositories directory
	err := filepath.Walk(s.gitRepoBase, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Check if this is a git repository directory
		if info.IsDir() && s.isGitRepository(path) {
			repoID := filepath.Base(path)

			repo, err := s.GetRepository(ctx, repoID)
			if err != nil {
				log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to get repository info")
				return nil // Continue walking
			}

			// Filter by owner if specified
			if ownerID == "" || repo.OwnerID == ownerID {
				repositories = append(repositories, repo)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	log.Info().
		Int("count", len(repositories)).
		Str("owner_id", ownerID).
		Msg("Listed repositories from filesystem")

	return repositories, nil
}

// CreateSpecTaskRepository creates a repository specifically for a SpecTask
func (s *GitRepositoryService) CreateSpecTaskRepository(
	ctx context.Context,
	specTask *types.SpecTask,
	templateFiles map[string]string,
) (*GitRepository, error) {
	// Determine initial files for the repository
	initialFiles := make(map[string]string)

	// Add template files if provided
	for path, content := range templateFiles {
		initialFiles[path] = content
	}

	// Add basic project structure if no template provided
	if len(initialFiles) == 0 {
		initialFiles = s.getDefaultProjectFiles(specTask)
	}

	// Create repository request
	request := &GitRepositoryCreateRequest{
		Name:          fmt.Sprintf("%s-project", specTask.Name),
		Description:   fmt.Sprintf("Project repository for SpecTask: %s", specTask.Name),
		RepoType:      GitRepositoryTypeSpecTask,
		OwnerID:       specTask.CreatedBy,
		ProjectID:     specTask.ProjectID,
		SpecTaskID:    specTask.ID,
		InitialFiles:  initialFiles,
		DefaultBranch: "main",
		Metadata: map[string]interface{}{
			"spec_task_name": specTask.Name,
			"created_from":   "spec_task",
		},
	}

	return s.CreateRepository(ctx, request)
}

// CreateSampleRepository creates a sample/demo repository
func (s *GitRepositoryService) CreateSampleRepository(
	ctx context.Context,
	name string,
	description string,
	ownerID string,
	sampleType string,
) (*GitRepository, error) {
	// Get sample files based on type
	initialFiles := s.getSampleProjectFiles(sampleType)

	request := &GitRepositoryCreateRequest{
		Name:          name,
		Description:   description,
		RepoType:      GitRepositoryTypeSample,
		OwnerID:       ownerID,
		InitialFiles:  initialFiles,
		DefaultBranch: "main",
		Metadata: map[string]interface{}{
			"sample_type":  sampleType,
			"created_from": "sample",
		},
	}

	return s.CreateRepository(ctx, request)
}

// GetCloneCommand returns the git clone command for a repository
func (s *GitRepositoryService) GetCloneCommand(repoID string, targetDir string) string {
	cloneURL := s.generateCloneURL(repoID)
	if targetDir == "" {
		return fmt.Sprintf("git clone %s", cloneURL)
	}
	return fmt.Sprintf("git clone %s %s", cloneURL, targetDir)
}

// generateRepositoryID generates a unique repository ID
func (s *GitRepositoryService) generateRepositoryID(repoType GitRepositoryType, name string) string {
	// Sanitize name for filesystem
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	sanitizedName = strings.ReplaceAll(sanitizedName, "_", "-")

	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%s-%d", repoType, sanitizedName, timestamp)
}

// generateCloneURL generates the clone URL for a repository
func (s *GitRepositoryService) generateCloneURL(repoID string) string {
	// Use HTTP URLs for network access by Zed agents
	// Format: http://api-server/git/{repo_id}
	return fmt.Sprintf("%s/git/%s", s.serverBaseURL, repoID)
}

// initializeGitRepository initializes a new git repository with initial files
func (s *GitRepositoryService) initializeGitRepository(
	repoPath string,
	defaultBranch string,
	initialFiles map[string]string,
) error {
	// Create repository directory
	err := os.MkdirAll(repoPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create repository directory: %w", err)
	}

	// Initialize git repository
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// Write initial files
	for filePath, content := range initialFiles {
		fullPath := filepath.Join(repoPath, filePath)

		// Create directory if needed
		dir := filepath.Dir(fullPath)
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write file
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	// Add files to git
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add all files
	_, err = worktree.Add(".")
	if err != nil {
		return fmt.Errorf("failed to add files to git: %w", err)
	}

	// Initial commit
	signature := &object.Signature{
		Name:  s.gitUserName,
		Email: s.gitUserEmail,
		When:  time.Now(),
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author:    signature,
		Committer: signature,
	})
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}

// isGitRepository checks if a directory is a git repository
func (s *GitRepositoryService) isGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	return false
}

// updateRepositoryFromGit updates repository info from git metadata
func (s *GitRepositoryService) updateRepositoryFromGit(gitRepo *GitRepository) error {
	// Open git repository
	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get branches
	refs, err := repo.References()
	if err == nil {
		branches := []string{}
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name().IsBranch() {
				branchName := ref.Name().Short()
				branches = append(branches, branchName)
			}
			return nil
		})
		gitRepo.Branches = branches
	}

	// Update last activity (could be improved with actual git log)
	gitRepo.LastActivity = time.Now()
	gitRepo.UpdatedAt = time.Now()

	return nil
}

// getDefaultProjectFiles returns default files for a new project
func (s *GitRepositoryService) getDefaultProjectFiles(specTask *types.SpecTask) map[string]string {
	files := make(map[string]string)

	// README.md
	files["README.md"] = fmt.Sprintf(`# %s

%s

This project was created from SpecTask: %s

## Getting Started

<!-- Add your getting started instructions here -->

## Development

<!-- Add your development instructions here -->

## License

<!-- Add your license information here -->
`, specTask.Name, specTask.Description, specTask.ID)

	// .gitignore
	files[".gitignore"] = `# Logs
logs
*.log
npm-debug.log*

# Dependencies
node_modules/
vendor/

# Environment variables
.env
.env.local

# Build outputs
build/
dist/
target/

# IDE files
.vscode/
.idea/
*.swp
*.swo

# OS files
.DS_Store
Thumbs.db
`

	// docs directory structure
	files["docs/README.md"] = "# Project Documentation\n\nThis directory contains project documentation.\n"
	files["src/README.md"] = "# Source Code\n\nThis directory contains the main source code.\n"
	files["tests/README.md"] = "# Tests\n\nThis directory contains tests.\n"

	return files
}

// getSampleProjectFiles returns sample files based on project type
func (s *GitRepositoryService) getSampleProjectFiles(sampleType string) map[string]string {
	files := make(map[string]string)

	switch sampleType {
	case "empty":
		files["README.md"] = fmt.Sprintf("# %s\n\nAn empty project repository ready for development.\n\n## Getting Started\n\nThis repository was created as an empty project. Add your code and documentation as needed.\n", sampleType)
		// Just basic structure - no framework-specific files
		return files

	case "nodejs-todo":
		files["README.md"] = "# Node.js Todo App\n\nA simple todo application built with Node.js and Express.\n"
		files["package.json"] = `{
  "name": "nodejs-todo-app",
  "version": "1.0.0",
  "description": "A simple todo application",
  "main": "src/index.js",
  "scripts": {
    "start": "node src/index.js",
    "dev": "nodemon src/index.js",
    "test": "jest"
  },
  "dependencies": {
    "express": "^4.18.0"
  },
  "devDependencies": {
    "nodemon": "^2.0.0",
    "jest": "^28.0.0"
  }
}`
		files["src/index.js"] = `const express = require('express');
const app = express();
const PORT = process.env.PORT || 3000;

app.use(express.json());

// Simple todos array
let todos = [];

// Routes
app.get('/api/todos', (req, res) => {
  res.json(todos);
});

app.post('/api/todos', (req, res) => {
  const todo = {
    id: Date.now(),
    text: req.body.text,
    completed: false
  };
  todos.push(todo);
  res.status(201).json(todo);
});

app.listen(PORT, () => {
  console.log('Server running on port ' + PORT);
});
`

	case "python-api":
		files["README.md"] = "# Python API Service\n\nA FastAPI microservice with PostgreSQL.\n"
		files["requirements.txt"] = `fastapi==0.104.1
uvicorn==0.24.0
sqlalchemy==2.0.23
psycopg2-binary==2.9.9
pydantic==2.5.0
`
		files["app/main.py"] = `from fastapi import FastAPI
from app.routers import items

app = FastAPI(title="Python API Service")

app.include_router(items.router, prefix="/api/v1")

@app.get("/")
async def root():
    return {"message": "Python API Service"}

@app.get("/health")
async def health():
    return {"status": "healthy"}
`
		files["app/__init__.py"] = ""
		files["app/routers/__init__.py"] = ""
		files["app/routers/items.py"] = `from fastapi import APIRouter

router = APIRouter()

@router.get("/items")
async def get_items():
    return {"items": []}

@router.post("/items")
async def create_item(item: dict):
    return {"message": "Item created", "item": item}
`

	case "react-dashboard":
		files["README.md"] = "# React Dashboard\n\nA modern admin dashboard built with React and Material-UI.\n"
		files["package.json"] = `{
  "name": "react-dashboard",
  "version": "1.0.0",
  "description": "A modern admin dashboard",
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "@mui/material": "^5.14.0",
    "@mui/icons-material": "^5.14.0"
  },
  "scripts": {
    "start": "react-scripts start",
    "build": "react-scripts build",
    "test": "react-scripts test"
  }
}`
		files["src/App.js"] = `import React from 'react';
import { AppBar, Toolbar, Typography, Container, Grid, Paper } from '@mui/material';

function App() {
  return (
    <div>
      <AppBar position="static">
        <Toolbar>
          <Typography variant="h6">
            React Dashboard
          </Typography>
        </Toolbar>
      </AppBar>
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        <Grid container spacing={3}>
          <Grid item xs={12}>
            <Paper sx={{ p: 2 }}>
              <Typography variant="h4">Welcome to React Dashboard</Typography>
              <Typography variant="body1">
                This is a sample dashboard application.
              </Typography>
            </Paper>
          </Grid>
        </Grid>
      </Container>
    </div>
  );
}

export default App;
`
		files["public/index.html"] = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>React Dashboard</title>
</head>
<body>
    <div id="root"></div>
</body>
</html>
`

	case "linkedin-outreach":
		files["README.md"] = "# LinkedIn Outreach Campaign\n\nMulti-session campaign to reach out to 100 prospects using Helix LinkedIn integration.\n"
		files["campaign/README.md"] = `# LinkedIn Outreach Campaign

## Campaign Overview
Goal: Reach out to 100 qualified prospects using Helix LinkedIn skill integration

## Multi-Session Strategy
- **Session A**: Prospect research and list building
- **Session B**: Message personalization and outreach
- **Session C**: Follow-up tracking and relationship management
- **Session D**: Campaign analysis and optimization

## Campaign Structure
- Target: 100 prospects in AI/ML industry
- Personalized messaging based on profile analysis
- Multi-touch follow-up sequence
- Conversion tracking and analysis
`
		files["campaign/prospect-criteria.md"] = `# Prospect Criteria

## Ideal Customer Profile
- **Industry**: AI/ML, Software Development, Tech Startups
- **Role**: CTO, VP Engineering, AI Lead, Technical Founder
- **Company Size**: 10-500 employees
- **Location**: Global (English-speaking)
- **Activity**: Active on LinkedIn, posts about AI/ML

## Qualification Criteria
- Has engineering team (5+ developers)
- Shows interest in AI/automation tools
- Posts about development challenges
- Engages with technical content

## Exclusion Criteria
- Competitors or similar companies
- Non-technical decision makers only
- Inactive LinkedIn profiles (no posts in 6 months)
- Already contacted in previous campaigns
`
		files["campaign/message-templates.md"] = `# Message Templates

## Initial Outreach Message
Hi [First Name],

I noticed your recent post about [specific topic from their profile]. Your insights on [specific detail] really resonated with me.

I'm reaching out because we've built something that might interest you - Helix is an AI development platform that helps engineering teams like yours accelerate development with AI-powered coding assistants.

What caught my attention about your work is [personalized observation based on their posts/company].

Would you be open to a brief conversation about how teams like yours are using AI to enhance their development workflows?

Best regards,
[Your name]

## Follow-up Message 1 (1 week later)
Hi [First Name],

I hope you don't mind the follow-up. I shared some insights about AI-powered development workflows that might be relevant to [their company/challenges].

I noticed [specific recent activity or post], which aligns with what we're seeing across the industry.

Would a quick 15-minute call work to discuss how other teams are solving [specific challenge they've mentioned]?

## Follow-up Message 2 (2 weeks later)
[Personalized based on new activity/posts]
`
		files["campaign/tracking-template.md"] = `# Campaign Tracking

## Prospect Status Tracking
| Name | Company | Role | Outreach Date | Status | Response | Next Action |
|------|---------|------|---------------|--------|----------|-------------|
| | | | | pending | | |

## Response Categories
- **Interested**: Wants to learn more / schedule call
- **Not Now**: Interested but timing not right
- **Not Relevant**: Not a good fit
- **No Response**: No reply after 3 touches
- **Connected**: Accepted invitation, ready for outreach

## Campaign Metrics
- **Total Prospects**: 100
- **Messages Sent**: 0
- **Responses Received**: 0
- **Positive Responses**: 0
- **Meetings Scheduled**: 0
- **Response Rate**: 0%
- **Conversion Rate**: 0%
`

	case "helix-blog-posts":
		files["README.md"] = "# Helix Technical Blog Posts\n\nWrite 10 technical blog posts about the Helix system by analyzing the actual codebase.\n"
		files["blog-project/README.md"] = `# Helix Blog Post Project

## Project Overview
Write 10 comprehensive blog posts about different aspects of the Helix AI development platform by analyzing the actual codebase.

## Multi-Session Strategy
- **Session A**: Repository analysis and content planning
- **Session B**: Technical deep-dive posts (architecture, APIs)
- **Session C**: User-focused posts (tutorials, use cases)
- **Session D**: Advanced topics and future roadmap

## Blog Post Topics
1. "Getting Started with Helix: Your First AI Assistant"
2. "Understanding Helix Architecture: From API to Models"
3. "Building Custom Skills: Extending Helix with APIs"
4. "Multi-Model Orchestration in Helix"
5. "Helix Security: Authentication and Access Control"
6. "Scaling Helix: Deployment and Operations"
7. "Advanced Helix: Custom Agents and Workflows"
8. "Helix vs. Other AI Platforms: Technical Comparison"
9. "Contributing to Helix: Developer Guide"
10. "The Future of Helix: Roadmap and Vision"

## Content Strategy
- Technical accuracy through code analysis
- Practical examples and tutorials
- Screenshots and diagrams
- Code samples from actual implementation
- User journey and use case focus
`
		files["blog-project/helix-repo-analysis.md"] = `# Helix Repository Analysis Plan

## Repository Information
- **Source**: https://github.com/helixml/helix
- **Clone Strategy**: Use Zed agent git access for live code analysis
- **Update Frequency**: Re-clone periodically to capture latest changes

## Code Analysis Approach

### 1. Architecture Analysis
- Main entry points: cmd/, main.go
- API structure: api/pkg/
- Frontend: frontend/src/
- Documentation: docs/, README.md

### 2. Key Components to Analyze
- **Session Management**: api/pkg/controller/
- **Model Integration**: api/pkg/model/
- **Skills System**: api/pkg/agent/skill/
- **Authentication**: api/pkg/auth/
- **Storage**: api/pkg/store/
- **WebSocket**: api/pkg/pubsub/

### 3. Content Generation Strategy
- Extract code examples for blog posts
- Understand data flows and interactions
- Identify key features and capabilities
- Generate practical tutorials from actual usage patterns

## Git Commands for Zed Agent
` + "```bash" + `
# Clone Helix repository
git clone https://github.com/helixml/helix.git /workspace/helix-analysis
cd /workspace/helix-analysis

# Analyze codebase structure
find . -name "*.go" | head -20
find . -name "*.ts" -o -name "*.tsx" | head -20
cat README.md
cat docs/*.md

# Understand main components
ls -la api/pkg/
ls -la frontend/src/
cat main.go
` + "```" + `
`
		files["blog-project/post-templates/"] = ""
		files["blog-project/post-templates/technical-post-template.md"] = `# [Blog Post Title]

*Part [X] of the Helix Technical Series*

## Introduction
[Brief introduction explaining what this post covers and why it matters]

## Overview
[High-level explanation of the topic]

## Technical Deep Dive

### Architecture
[Explain the architecture with references to actual code]

### Implementation Details
[Code examples from the actual Helix repository]

` + "```go" + `
// Example from helix/api/pkg/[component]/[file].go
[actual code snippet with explanation]
` + "```" + `

### Key Features
[Highlight important features and capabilities]

## Practical Examples

### Basic Usage
[Step-by-step tutorial with real examples]

### Advanced Usage
[More complex scenarios and configurations]

## Best Practices
[Recommendations based on codebase analysis]

## Troubleshooting
[Common issues and solutions based on code understanding]

## Conclusion
[Summary and next steps]

## Related Resources
- [Link to relevant documentation]
- [Link to code examples]
- [Link to other blog posts in series]

---
*This post was generated by analyzing the Helix codebase at [commit hash] on [date]*
`
		files["blog-project/content-calendar.md"] = `# Blog Post Content Calendar

## Publishing Schedule
| Post # | Title | Focus Area | Target Date | Status | Session |
|--------|-------|------------|-------------|--------|---------|
| 1 | Getting Started with Helix | User Tutorial | Week 1 | Planned | Session C |
| 2 | Helix Architecture Deep Dive | Technical | Week 1 | Planned | Session B |
| 3 | Building Custom Skills | Developer Guide | Week 2 | Planned | Session B |
| 4 | Multi-Model Orchestration | Technical | Week 2 | Planned | Session B |
| 5 | Authentication & Security | Technical | Week 3 | Planned | Session B |
| 6 | Deployment & Operations | DevOps | Week 3 | Planned | Session D |
| 7 | Advanced Workflows | Advanced | Week 4 | Planned | Session D |
| 8 | Platform Comparison | Business | Week 4 | Planned | Session C |
| 9 | Contributing Guide | Community | Week 5 | Planned | Session C |
| 10 | Future Roadmap | Vision | Week 5 | Planned | Session D |

## Content Distribution
- **Technical Posts (40%)**: Architecture, APIs, advanced features
- **Tutorial Posts (30%)**: Getting started, how-to guides
- **Business Posts (20%)**: Use cases, comparisons, ROI
- **Community Posts (10%)**: Contributing, roadmap, vision

## Session Coordination
- **Session A**: Repository analysis, research, planning
- **Session B**: Technical deep-dive content creation
- **Session C**: User-focused tutorials and guides
- **Session D**: Advanced topics and strategic content
`

	default:
		// Generic project
		files["README.md"] = fmt.Sprintf("# %s\n\nA sample project.\n", sampleType)
		files["src/main.txt"] = "Main source file\n"
	}

	// Add common files
	files[".gitignore"] = `# Dependencies
node_modules/
__pycache__/
*.pyc

# Build outputs
build/
dist/

# Environment
.env
.env.local

# IDE
.vscode/
.idea/
`

	return files
}

// storeRepositoryMetadata stores repository metadata in the store (if supported)
func (s *GitRepositoryService) storeRepositoryMetadata(ctx context.Context, repo *GitRepository) error {
	// Use the store's git repository methods if available
	if postgresStore, ok := s.store.(interface {
		CreateGitRepository(ctx context.Context, repo *GitRepository) error
	}); ok {
		err := postgresStore.CreateGitRepository(ctx, repo)
		if err != nil {
			log.Warn().Err(err).Str("repo_id", repo.ID).Msg("Failed to store repository metadata in database")
			return err
		}
		log.Info().
			Str("repo_id", repo.ID).
			Str("repo_type", string(repo.RepoType)).
			Str("owner_id", repo.OwnerID).
			Msg("Repository metadata stored in database")
		return nil
	}

	log.Warn().Msg("Store does not support git repository persistence")
	return fmt.Errorf("store does not support git repository persistence")
}

// getRepositoryMetadata retrieves repository metadata from store
func (s *GitRepositoryService) getRepositoryMetadata(ctx context.Context, repoID string) (*GitRepository, error) {
	// Use the store's git repository methods if available
	if postgresStore, ok := s.store.(interface {
		GetGitRepository(ctx context.Context, id string) (*GitRepository, error)
	}); ok {
		repo, err := postgresStore.GetGitRepository(ctx, repoID)
		if err != nil {
			return nil, err
		}
		return repo, nil
	}

	return nil, fmt.Errorf("repository metadata not found in store")
}
