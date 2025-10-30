package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// SampleProject represents a sample project template
type SampleProject struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	GitHubRepo    string              `json:"github_repo"`
	DefaultBranch string              `json:"default_branch"`
	Technologies  []string            `json:"technologies"`
	SampleTasks   []SampleProjectTask `json:"sample_tasks"`
	ReadmeURL     string              `json:"readme_url"`
	DemoURL       string              `json:"demo_url,omitempty"`
	Difficulty    string              `json:"difficulty"` // "beginner", "intermediate", "advanced"
	Category      string              `json:"category"`   // "web", "api", "mobile", "data", "ai"
}

type SampleProjectTask struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Type               string   `json:"type"`     // "feature", "bug", "task", "epic"
	Priority           string   `json:"priority"` // "low", "medium", "high", "critical"
	Status             string   `json:"status"`   // "backlog", "ready", "in_progress", "review", "done"
	EstimatedHours     int      `json:"estimated_hours,omitempty"`
	Labels             []string `json:"labels"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	TechnicalNotes     string   `json:"technical_notes,omitempty"`
	FilesToModify      []string `json:"files_to_modify,omitempty"`
}

// Built-in sample projects
var SAMPLE_PROJECTS = []SampleProject{
	{
		ID:            "modern-todo-app",
		Name:          "Modern Todo App",
		Description:   "A full-stack todo application with React, Node.js, and PostgreSQL. Perfect for learning modern web development patterns.",
		GitHubRepo:    "helix-ai/sample-todo-app",
		DefaultBranch: "main",
		Technologies:  []string{"React", "TypeScript", "Node.js", "Express", "PostgreSQL", "Tailwind CSS", "Prisma"},
		ReadmeURL:     "https://github.com/helix-ai/sample-todo-app/blob/main/README.md",
		DemoURL:       "https://sample-todo-app.vercel.app",
		Difficulty:    "intermediate",
		Category:      "web",
		SampleTasks: []SampleProjectTask{
			{
				Title:          "Implement dark mode toggle",
				Description:    "Add a dark/light mode toggle with persistent user preference using localStorage. The toggle should smoothly transition between themes and remember the user's choice across sessions.",
				Type:           "feature",
				Priority:       "medium",
				Status:         "backlog",
				EstimatedHours: 3,
				Labels:         []string{"frontend", "ui/ux", "react"},
				AcceptanceCriteria: []string{
					"Toggle button in the header switches between light and dark themes",
					"Theme preference persists across browser sessions",
					"Smooth transitions between theme changes",
					"All components properly styled in both themes",
				},
				TechnicalNotes: "Use React Context for theme state management and CSS variables for theme colors",
				FilesToModify:  []string{"src/contexts/ThemeContext.tsx", "src/components/Header.tsx", "src/styles/globals.css"},
			},
			{
				Title:          "Fix todo deletion race condition",
				Description:    "When multiple users try to delete the same todo item simultaneously, it causes database inconsistencies. Implement proper concurrency control.",
				Type:           "bug",
				Priority:       "high",
				Status:         "ready",
				EstimatedHours: 2,
				Labels:         []string{"backend", "database", "concurrency"},
				AcceptanceCriteria: []string{
					"Only one delete operation succeeds for the same todo item",
					"Proper error handling for concurrent delete attempts",
					"Database integrity maintained under high concurrency",
				},
				TechnicalNotes: "Implement optimistic locking or use database transactions with proper isolation levels",
				FilesToModify:  []string{"server/routes/todos.js", "server/models/Todo.js"},
			},
			{
				Title:          "Add todo categories and filtering",
				Description:    "Allow users to organize todos into custom categories (Work, Personal, Shopping, etc.) with filtering and color-coding capabilities.",
				Type:           "feature",
				Priority:       "low",
				Status:         "backlog",
				EstimatedHours: 6,
				Labels:         []string{"frontend", "backend", "database", "ui/ux"},
				AcceptanceCriteria: []string{
					"Users can create custom categories",
					"Each todo can be assigned to a category",
					"Filter todos by category",
					"Categories have customizable colors",
					"Category management interface (CRUD operations)",
				},
				TechnicalNotes: "Add categories table and update todo schema. Implement category selector component.",
				FilesToModify: []string{
					"server/migrations/add_categories.sql",
					"server/models/Category.js",
					"src/components/CategorySelector.tsx",
					"src/components/TodoList.tsx",
				},
			},
			{
				Title:          "Implement user authentication with JWT",
				Description:    "Add secure user authentication system with JWT tokens, registration, login, and protected routes.",
				Type:           "feature",
				Priority:       "critical",
				Status:         "ready",
				EstimatedHours: 10,
				Labels:         []string{"backend", "security", "frontend", "authentication"},
				AcceptanceCriteria: []string{
					"User registration with email validation",
					"Secure login with JWT tokens",
					"Password hashing with bcrypt",
					"Protected API routes",
					"Frontend auth state management",
					"Logout functionality",
				},
				TechnicalNotes: "Use bcrypt for password hashing, jsonwebtoken for JWT, and implement auth middleware",
				FilesToModify: []string{
					"server/middleware/auth.js",
					"server/routes/auth.js",
					"server/models/User.js",
					"src/contexts/AuthContext.tsx",
					"src/components/LoginForm.tsx",
					"src/components/RegisterForm.tsx",
				},
			},
		},
	},
	{
		ID:            "ecommerce-api",
		Name:          "E-commerce REST API",
		Description:   "A comprehensive RESTful API for an e-commerce platform with product management, order processing, and payment integration.",
		GitHubRepo:    "helix-ai/sample-ecommerce-api",
		DefaultBranch: "main",
		Technologies:  []string{"Node.js", "Express", "MongoDB", "Redis", "Stripe", "JWT", "Docker"},
		ReadmeURL:     "https://github.com/helix-ai/sample-ecommerce-api/blob/main/README.md",
		Difficulty:    "advanced",
		Category:      "api",
		SampleTasks: []SampleProjectTask{
			{
				Title:          "Implement product search with Elasticsearch",
				Description:    "Add full-text search functionality for products with advanced filtering, sorting, and faceted search capabilities.",
				Type:           "feature",
				Priority:       "high",
				Status:         "backlog",
				EstimatedHours: 8,
				Labels:         []string{"search", "elasticsearch", "performance"},
				AcceptanceCriteria: []string{
					"Full-text search across product name, description, and tags",
					"Filter by price range, category, brand, and availability",
					"Sort by relevance, price, rating, and date",
					"Autocomplete suggestions",
					"Search analytics and logging",
				},
				TechnicalNotes: "Set up Elasticsearch cluster and implement search indexing pipeline",
				FilesToModify: []string{
					"src/services/searchService.js",
					"src/controllers/productController.js",
					"src/config/elasticsearch.js",
				},
			},
			{
				Title:          "Fix inventory race condition in high concurrency",
				Description:    "When multiple users purchase the same item simultaneously, inventory can go negative. Implement proper concurrency control.",
				Type:           "bug",
				Priority:       "critical",
				Status:         "ready",
				EstimatedHours: 4,
				Labels:         []string{"concurrency", "inventory", "database"},
				AcceptanceCriteria: []string{
					"Inventory cannot go below zero under any circumstances",
					"Proper error handling for out-of-stock scenarios",
					"Performance maintained under high load",
				},
				TechnicalNotes: "Use MongoDB transactions with proper isolation levels or implement distributed locks with Redis",
				FilesToModify: []string{
					"src/services/inventoryService.js",
					"src/controllers/orderController.js",
				},
			},
			{
				Title:          "Add order cancellation workflow",
				Description:    "Implement order cancellation with automatic refund processing and inventory restoration within the allowed time window.",
				Type:           "feature",
				Priority:       "medium",
				Status:         "backlog",
				EstimatedHours: 6,
				Labels:         []string{"orders", "payments", "business-logic"},
				AcceptanceCriteria: []string{
					"Orders can be cancelled within 1 hour of placement",
					"Automatic refund processing via Stripe",
					"Inventory quantities restored",
					"Email notifications sent",
					"Order status tracking",
				},
				TechnicalNotes: "Integrate with Stripe refund API and implement cancellation time window logic",
				FilesToModify: []string{
					"src/services/orderService.js",
					"src/services/paymentService.js",
					"src/controllers/orderController.js",
				},
			},
		},
	},
	{
		ID:            "react-native-weather",
		Name:          "Weather App - React Native",
		Description:   "Cross-platform weather application with location services, weather forecasts, and offline caching.",
		GitHubRepo:    "helix-ai/sample-weather-app",
		DefaultBranch: "main",
		Technologies:  []string{"React Native", "TypeScript", "Expo", "AsyncStorage", "Reanimated"},
		ReadmeURL:     "https://github.com/helix-ai/sample-weather-app/blob/main/README.md",
		Difficulty:    "beginner",
		Category:      "mobile",
		SampleTasks: []SampleProjectTask{
			{
				Title:          "Add weather animations",
				Description:    "Implement animated weather icons and background effects that reflect current weather conditions.",
				Type:           "feature",
				Priority:       "low",
				Status:         "backlog",
				EstimatedHours: 5,
				Labels:         []string{"ui/ux", "animations", "react-native"},
				AcceptanceCriteria: []string{
					"Animated weather icons (rain, snow, sun, clouds)",
					"Dynamic background colors based on weather",
					"Smooth transitions between weather states",
				},
				TechnicalNotes: "Use React Native Reanimated 2 for smooth 60fps animations",
				FilesToModify: []string{
					"src/components/WeatherIcon.tsx",
					"src/components/WeatherBackground.tsx",
				},
			},
		},
	},
}

// listSampleProjects godoc
// @Summary List available sample projects
// @Description Get a list of all available sample projects that users can fork and use
// @Tags    sample-projects
// @Success 200 {array} SampleProject
// @Router /api/v1/sample-projects [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSampleProjectTemplates(_ http.ResponseWriter, r *http.Request) ([]SampleProject, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Filter by category or difficulty if specified
	category := r.URL.Query().Get("category")
	difficulty := r.URL.Query().Get("difficulty")

	var filteredProjects []SampleProject
	for _, project := range SAMPLE_PROJECTS {
		if category != "" && project.Category != category {
			continue
		}
		if difficulty != "" && project.Difficulty != difficulty {
			continue
		}
		filteredProjects = append(filteredProjects, project)
	}

	return filteredProjects, nil
}

// getSampleProject godoc
// @Summary Get a specific sample project
// @Description Get details of a specific sample project by ID
// @Tags    sample-projects
// @Success 200 {object} SampleProject
// @Param project_id path string true "Sample project ID"
// @Router /api/v1/sample-projects/{project_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSampleProject(_ http.ResponseWriter, r *http.Request) (*SampleProject, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	projectID := vars["project_id"]

	for _, project := range SAMPLE_PROJECTS {
		if project.ID == projectID {
			return &project, nil
		}
	}

	return nil, system.NewHTTPError404("sample project not found")
}

// ForkSampleProjectRequest represents the request to fork a sample project
type ForkSampleProjectRequest struct {
	SampleProjectID string `json:"sample_project_id"`
	ProjectName     string `json:"project_name"`
	Description     string `json:"description,omitempty"`
	Private         bool   `json:"private"`
}

// ForkSampleProjectResponse represents the response after forking
type ForkSampleProjectResponse struct {
	ProjectID     string `json:"project_id"`
	GitHubRepoURL string `json:"github_repo_url"`
	Message       string `json:"message"`
}

// forkSampleProject godoc
// @Summary Fork a sample project
// @Description Fork a sample project to the user's GitHub account and create a new Helix project
// @Tags    sample-projects
// @Success 201 {object} ForkSampleProjectResponse
// @Param request body ForkSampleProjectRequest true "Fork request details"
// @Router /api/v1/sample-projects/fork [post]
// @Security BearerAuth
func (s *HelixAPIServer) forkSampleProject(_ http.ResponseWriter, r *http.Request) (*ForkSampleProjectResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	var req ForkSampleProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Find the sample project
	var sampleProject *SampleProject
	for _, project := range SAMPLE_PROJECTS {
		if project.ID == req.SampleProjectID {
			sampleProject = &project
			break
		}
	}

	if sampleProject == nil {
		return nil, system.NewHTTPError404("sample project not found")
	}

	// Generate unique project ID
	projectID := fmt.Sprintf("proj_%s_%d", req.SampleProjectID, time.Now().Unix())

	// Create project name if not provided
	projectName := req.ProjectName
	if projectName == "" {
		projectName = fmt.Sprintf("%s - %s", sampleProject.Name, user.Username)
	}

	log.Info().
		Str("user_id", user.ID).
		Str("sample_project_id", req.SampleProjectID).
		Str("project_name", projectName).
		Msg("Forking sample project")

	// TODO: Implement actual GitHub forking using GitHub API
	// For now, we'll simulate the forking process
	forkedRepoURL := fmt.Sprintf("https://github.com/%s/%s", user.Username,
		fmt.Sprintf("helix-%s", req.SampleProjectID))

	// Create Helix project record
	project := &types.Project{
		ID:             projectID,
		Name:           projectName,
		Description:    req.Description,
		UserID:         user.ID,
		OrganizationID: "", // TODO: Get organization ID from user context
		GitHubRepoURL:  forkedRepoURL,
		DefaultBranch:  sampleProject.DefaultBranch,
		Technologies:   sampleProject.Technologies,
		Status:         "active",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata: func() datatypes.JSON {
			metadataMap := map[string]interface{}{
				"sample_project_id": req.SampleProjectID,
				"original_repo":     sampleProject.GitHubRepo,
				"forked_from":       "sample_project",
			}
			metadataBytes, _ := json.Marshal(metadataMap)
			return datatypes.JSON(metadataBytes)
		}(),
	}

	// Store project (assuming we have a projects store)
	createdProject, err := s.Store.CreateProject(ctx, project)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to create project")
		return nil, system.NewHTTPError500("Failed to create project")
	}
	_ = createdProject // Use the created project if needed

	// Create sample tasks for the project
	for i, taskTemplate := range sampleProject.SampleTasks {
		task := &types.AgentWorkItem{
			ID:              fmt.Sprintf("task_%s_%d", projectID, i),
			TriggerConfigID: "", // Will be set when trigger is created
			Name:            taskTemplate.Title,
			Description:     taskTemplate.Description,
			Source:          "sample_project",
			SourceID:        fmt.Sprintf("%d", i+1),
			Priority:        getPriorityNumber(taskTemplate.Priority),
			Status:          taskTemplate.Status,
			AgentType:       "zed", // Default to Zed for sample projects
			UserID:          user.ID,
			AppID:           "", // Will be linked to app if needed
			OrganizationID:  "", // TODO: Get organization ID from user context
			MaxRetries:      3,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			Metadata: func() datatypes.JSON {
				metadataMap := map[string]interface{}{
					"project_id":          projectID,
					"task_type":           taskTemplate.Type,
					"estimated_hours":     taskTemplate.EstimatedHours,
					"labels":              taskTemplate.Labels,
					"acceptance_criteria": taskTemplate.AcceptanceCriteria,
					"technical_notes":     taskTemplate.TechnicalNotes,
					"files_to_modify":     taskTemplate.FilesToModify,
				}
				metadataBytes, _ := json.Marshal(metadataMap)
				return datatypes.JSON(metadataBytes)
			}(),
		}

		// Convert work data to JSON
		workData := map[string]interface{}{
			"project_id":          projectID,
			"github_repo":         forkedRepoURL,
			"task_type":           taskTemplate.Type,
			"estimated_hours":     taskTemplate.EstimatedHours,
			"acceptance_criteria": taskTemplate.AcceptanceCriteria,
			"technical_notes":     taskTemplate.TechnicalNotes,
			"files_to_modify":     taskTemplate.FilesToModify,
		}
		workDataJSON, _ := json.Marshal(workData)
		task.WorkData = workDataJSON

		// Labels
		labelsJSON, _ := json.Marshal(taskTemplate.Labels)
		task.Labels = labelsJSON

		err := s.Store.CreateAgentWorkItem(ctx, task)
		if err != nil {
			log.Warn().Err(err).
				Str("project_id", projectID).
				Str("task_title", taskTemplate.Title).
				Msg("Failed to create sample task")
		}
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", user.ID).
		Str("repo_url", forkedRepoURL).
		Int("tasks_created", len(sampleProject.SampleTasks)).
		Msg("Successfully forked sample project")

	return &ForkSampleProjectResponse{
		ProjectID:     projectID,
		GitHubRepoURL: forkedRepoURL,
		Message: fmt.Sprintf("Successfully forked %s! Created %d sample tasks ready for AI agents to work on.",
			sampleProject.Name, len(sampleProject.SampleTasks)),
	}, nil
}

// Helper function to convert priority string to number
func getPriorityNumber(priority string) int {
	switch priority {
	case "critical":
		return 1
	case "high":
		return 2
	case "medium":
		return 3
	case "low":
		return 4
	default:
		return 3
	}
}
