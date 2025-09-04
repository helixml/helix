# Revised Zed-Based Planning Architecture

## 🚨 **Critical Architecture Revision**

### **Problem Identified with Current Implementation**

The current architecture has a **fundamental disconnect**:

1. **Helix Planning Agents:** Generate specs in isolated local repos (`/helix-data/git-repos/`)
2. **Zed Implementation Agents:** Work in user's actual project repos (`/workspace/user-project/`)
3. **No Bridge:** How do specs get from Helix repos → actual project repos?

**Result:** Manual handoff required, specs isolated from implementation context.

### **Root Cause Analysis**

- **Helix agents:** Limited git access, can't easily `git clone`, `git push`
- **Zed agents:** Full shell access, can run any git commands
- **Architecture mismatch:** Planning in one system, implementation in another

## 💡 **Revised Architecture: Zed-Based Planning**

### **Core Principle**
> **Use Zed agents for BOTH planning and implementation phases**
> Work directly in user's actual project repositories

### **Revised Workflow**

```
User Request → Zed Planning Agent → Specs in Project Repo → Multi-Session Implementation

1. User creates SpecTask with project repository URL
2. Zed Planning Agent clones user's actual project repo
3. Planning Agent generates specs directly in project repo
4. Specs committed to feature branch for review
5. Human approval triggers implementation phase
6. Multiple Zed Implementation Agents work in same repo
7. All coordination happens within actual project context
```

## 🏗️ **Technical Implementation**

### **1. Zed Planning Agent Workflow**

```javascript
// Planning Phase - Zed Agent
const planningWorkflow = async (specTask, projectRepoUrl) => {
  // 1. Clone user's actual project repository
  await exec(`git clone ${projectRepoUrl} /workspace/${specTask.id}`);
  await exec(`cd /workspace/${specTask.id}`);
  
  // 2. Create feature branch for specs
  const specBranch = `specs/${specTask.id}`;
  await exec(`git checkout -b ${specBranch}`);
  
  // 3. Analyze existing codebase context
  const projectContext = await analyzeProject('/workspace/' + specTask.id);
  
  // 4. Generate specs with project awareness
  const requirements = await generateRequirements(specTask, projectContext);
  const design = await generateDesign(specTask, projectContext);
  const tasks = await generateImplementationPlan(specTask, projectContext);
  
  // 5. Write specs to project repo
  await writeFile('docs/specs/requirements.md', requirements);
  await writeFile('docs/specs/design.md', design);
  await writeFile('docs/specs/tasks.md', tasks);
  
  // 6. Commit and push for review
  await exec(`git add docs/specs/`);
  await exec(`git commit -m "Add specifications for: ${specTask.name}"`);
  await exec(`git push origin ${specBranch}`);
  
  // 7. Create pull request (if GitHub integration available)
  if (hasGitHubOAuth) {
    await createPullRequest(specBranch, 'main', 'Specifications for Review');
  }
  
  return { branch: specBranch, status: 'pending_review' };
};
```

### **2. Project Repository Structure**

```
user-project-repo/
├── .git/                           # User's actual git repository
├── docs/
│   ├── specs/                      # Generated specifications (Kiro-style)
│   │   ├── requirements.md         # EARS notation requirements  
│   │   ├── design.md              # Technical design with codebase context
│   │   ├── tasks.md               # Implementation plan
│   │   └── coordination.md        # Multi-session coordination plan
│   └── sessions/                  # Session history during implementation
│       ├── coordination-log.md    # Inter-session coordination
│       └── session-{id}/          # Individual session history
├── src/                           # Existing user code
├── tests/                         # Existing user tests
└── README.md                      # User's project README
```

### **3. Multi-Session Implementation in Same Repo**

```javascript
// Implementation Phase - Multiple Zed Agents
const implementationWorkflow = async (specTask, approvedSpecs) => {
  // Each implementation session works in the same repo
  const workDir = `/workspace/${specTask.id}`;
  
  // 1. Checkout approved specs branch
  await exec(`cd ${workDir} && git checkout ${approvedSpecs.branch}`);
  await exec(`cd ${workDir} && git pull origin main`); // Merge latest changes
  
  // 2. Create implementation branch
  const implBranch = `implementation/${specTask.id}`;
  await exec(`cd ${workDir} && git checkout -b ${implBranch}`);
  
  // 3. Parse implementation tasks
  const tasks = await parseImplementationTasks(`${workDir}/docs/specs/tasks.md`);
  
  // 4. Coordinate multiple sessions
  const workSessions = await createMultipleWorkSessions(tasks);
  
  // Each work session:
  workSessions.forEach(async (session) => {
    // - Has access to full project context
    // - Can see all existing code
    // - Can run tests and validate changes
    // - Records activity in docs/sessions/
    await runWorkSession(session, workDir);
  });
  
  return { branch: implBranch, sessions: workSessions };
};
```

## 🔄 **Revised Service Architecture**

### **1. Enhanced Zed Integration Service**

```go
// ZedPlanningService - New service for Zed-based planning
type ZedPlanningService struct {
    store              store.Store
    zedIntegration     *ZedIntegrationService
    gitService         *GitService // New service for git operations
    projectAnalyzer    *ProjectAnalyzer // New service for codebase analysis
}

func (s *ZedPlanningService) StartPlanningPhase(
    ctx context.Context, 
    specTask *types.SpecTask,
    projectRepoUrl string,
) (*types.ZedPlanningSession, error) {
    // 1. Create Zed planning agent
    planningAgent := &types.ZedAgent{
        SessionID:   fmt.Sprintf("planning_%s", specTask.ID),
        ProjectPath: fmt.Sprintf("/workspace/%s", specTask.ID),
        Role:        "planning_agent",
        Capabilities: []string{"git", "file_analysis", "documentation"},
        Environment: map[string]string{
            "PROJECT_REPO_URL": projectRepoUrl,
            "SPEC_TASK_ID":     specTask.ID,
            "PHASE":            "planning",
        },
    }
    
    // 2. Launch planning session
    return s.zedIntegration.LaunchPlanningSession(ctx, planningAgent)
}
```

### **2. Git Service for Repository Management**

```go
// GitService - Manages git operations for project repositories
type GitService struct {
    githubClient   *github.Client // Optional GitHub integration
    defaultBranch  string
    enablePRs      bool
}

func (s *GitService) CloneProjectRepo(repoUrl, workDir string) error {
    // Handle different repo types:
    // - GitHub URLs (requires OAuth)
    // - GitLab URLs  
    // - Local file paths
    // - Git URLs
}

func (s *GitService) CreateSpecBranch(workDir, specTaskID string) (string, error) {
    branchName := fmt.Sprintf("specs/%s", specTaskID)
    // Create and checkout branch
    return branchName, nil
}

func (s *GitService) CommitSpecs(workDir, message string) error {
    // Add, commit, and optionally push specs
}
```

## 🔗 **GitHub OAuth Integration**

### **Current Status & Next Steps**

**For Sample Sessions (Immediate):**
1. **Manual Git Repo Setup:** Create sample project repos manually
2. **Local Git Operations:** Use file:// URLs for git repos
3. **Zed Agent Access:** Agents work directly in project directories

**For Production (Phase 2):**
1. **GitHub OAuth Integration:** Enable `repo` scope for private repos
2. **Automatic Clone/Push:** Zed agents can access user's GitHub repos  
3. **Pull Request Workflow:** Automatic PR creation for spec review

### **Sample Project Setup (Immediate Solution)**

```bash
# Create sample project repositories
mkdir -p /helix-data/sample-projects

# Sample 1: Simple Node.js app
cd /helix-data/sample-projects
git init nodejs-todo-app
cd nodejs-todo-app
echo "# Node.js Todo App" > README.md
mkdir src tests docs
git add . && git commit -m "Initial project structure"

# Sample 2: Python microservice  
cd /helix-data/sample-projects
git init python-api-service
cd python-api-service
echo "# Python API Service" > README.md
mkdir app tests docs requirements
git add . && git commit -m "Initial project structure"

# Sample 3: React frontend
cd /helix-data/sample-projects
git init react-dashboard
cd react-dashboard
echo "# React Dashboard" > README.md
mkdir src public docs
git add . && git commit -m "Initial project structure"
```

### **Zed Agent Configuration for Samples**

```go
// Sample project configuration
sampleProjects := []ProjectConfig{
    {
        Name: "Node.js Todo App",
        RepoPath: "file:///helix-data/sample-projects/nodejs-todo-app",
        Description: "Simple todo application with REST API",
        TechStack: []string{"node", "express", "mongodb"},
    },
    {
        Name: "Python API Service", 
        RepoPath: "file:///helix-data/sample-projects/python-api-service",
        Description: "Microservice with FastAPI and PostgreSQL",
        TechStack: []string{"python", "fastapi", "postgresql"},
    },
    {
        Name: "React Dashboard",
        RepoPath: "file:///helix-data/sample-projects/react-dashboard", 
        Description: "Admin dashboard with real-time updates",
        TechStack: []string{"react", "typescript", "mui"},
    },
}
```

## 🎯 **Benefits of Revised Architecture**

### **1. Unified Context**
- Planning and implementation in same repository
- Specs generated with full project awareness
- No manual handoff between systems

### **2. Real Project Integration**
- Works with user's actual repositories
- Maintains existing project structure
- Preserves git history and branches

### **3. Enhanced Zed Capabilities**
- Full git access for planning agents
- Codebase analysis capabilities
- Direct project manipulation

### **4. Simplified Workflow**
- Single repository for entire workflow
- Standard git branching for review
- Natural progression from specs → implementation

## 🚀 **Implementation Plan**

### **Phase 1: Immediate (Week 1)**
1. **Create ZedPlanningService** - Zed agents for planning phase
2. **Setup Sample Projects** - Manual git repos for demos
3. **Modify SpecTask Creation** - Accept project repository URLs
4. **Update Frontend** - Repository input in SpecTask creation

### **Phase 2: Git Integration (Week 2-3)**  
1. **GitService Implementation** - Repository management
2. **Branch Management** - Spec branches, implementation branches
3. **Commit Workflow** - Automatic commits during planning/implementation
4. **Local Git Testing** - File:// URLs for development

### **Phase 3: GitHub Integration (Week 4+)**
1. **GitHub OAuth Setup** - Repository access permissions
2. **Pull Request Workflow** - Automatic PR creation
3. **Webhook Integration** - Status updates from GitHub
4. **Private Repository Support** - Enterprise GitHub integration

## 🔧 **Migration from Current Implementation**

### **What Changes**
- **Planning Phase:** Helix agents → Zed agents
- **Repository Strategy:** Local isolated repos → User's actual repos
- **Handoff Process:** File copying → Git branching workflow

### **What Stays the Same**
- **Multi-session coordination** - All existing logic
- **Frontend UI** - Minor updates to repo input
- **Database schema** - Fully compatible
- **API endpoints** - Mostly unchanged

### **Compatibility**
- Existing SpecTasks can be migrated to new workflow
- Current multi-session management fully preserved
- Frontend components require minimal updates

## 🎉 **Conclusion**

This revised architecture **solves the fundamental git integration problem** while **preserving all the sophisticated multi-session coordination** we've built.

**Key Improvements:**
✅ **Unified Repository Context** - Planning and implementation in same repo
✅ **Enhanced Zed Integration** - Agents with full git capabilities  
✅ **Real Project Workflow** - Works with actual user repositories
✅ **Simplified Handoff** - Git branching instead of file copying
✅ **Future GitHub Ready** - Architecture supports OAuth integration

**Next Steps:**
1. Implement ZedPlanningService
2. Create sample project repositories  
3. Update SpecTask creation workflow
4. Test with local git repositories
5. Plan GitHub OAuth integration

This approach delivers **immediate value** with local git repos while building toward **full GitHub integration**! 🚀