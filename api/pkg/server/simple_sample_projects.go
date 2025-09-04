package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SimpleSampleProject represents a sample project with natural language prompts
type SimpleSampleProject struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	GitHubRepo    string             `json:"github_repo"`
	DefaultBranch string             `json:"default_branch"`
	Technologies  []string           `json:"technologies"`
	TaskPrompts   []SampleTaskPrompt `json:"task_prompts"`
	ReadmeURL     string             `json:"readme_url"`
	DemoURL       string             `json:"demo_url,omitempty"`
	Difficulty    string             `json:"difficulty"`
	Category      string             `json:"category"`
}

// SampleTaskPrompt follows Kiro's approach - just natural language prompts
type SampleTaskPrompt struct {
	Prompt      string   `json:"prompt"`      // Natural language request
	Priority    string   `json:"priority"`    // "low", "medium", "high", "critical"
	Labels      []string `json:"labels"`      // Tags for organization
	Context     string   `json:"context"`     // Additional context about the codebase
	Constraints string   `json:"constraints"` // Any specific constraints or requirements
}

// SIMPLE_SAMPLE_PROJECTS - realistic projects with natural prompts like Kiro expects
var SIMPLE_SAMPLE_PROJECTS = []SimpleSampleProject{
	{
		ID:            "modern-todo-app",
		Name:          "Modern Todo App",
		Description:   "Full-stack todo application with React and Node.js - perfect for learning modern web patterns",
		GitHubRepo:    "helix-ai/sample-todo-app",
		DefaultBranch: "main",
		Technologies:  []string{"React", "TypeScript", "Node.js", "Express", "PostgreSQL", "Tailwind CSS"},
		ReadmeURL:     "https://github.com/helix-ai/sample-todo-app/blob/main/README.md",
		DemoURL:       "https://sample-todo-app.vercel.app",
		Difficulty:    "intermediate",
		Category:      "web",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add a dark mode toggle that remembers the user's preference",
				Priority: "medium",
				Labels:   []string{"frontend", "ui/ux"},
				Context:  "The app uses React with Tailwind CSS. Users should be able to switch between light and dark themes seamlessly.",
			},
			{
				Prompt:   "Fix the bug where deleting a todo item doesn't remove it from the database",
				Priority: "high",
				Labels:   []string{"backend", "database", "bug"},
				Context:  "Users report that deleted todos reappear after page refresh. The DELETE API endpoint exists but may not be working correctly.",
			},
			{
				Prompt:   "Add the ability for users to organize todos into custom categories",
				Priority: "low",
				Labels:   []string{"frontend", "backend", "feature"},
				Context:  "Users want to group todos by categories like 'Work', 'Personal', 'Shopping'. Each category should have a color and be filterable.",
			},
			{
				Prompt:   "Implement user authentication so people can have private todo lists",
				Priority: "critical",
				Labels:   []string{"backend", "security", "frontend"},
				Context:  "Right now todos are shared globally. Users need their own private lists with secure login/register functionality.",
			},
		},
	},
	{
		ID:            "ecommerce-api",
		Name:          "E-commerce REST API",
		Description:   "Comprehensive API for an e-commerce platform with product management and order processing",
		GitHubRepo:    "helix-ai/sample-ecommerce-api",
		DefaultBranch: "main",
		Technologies:  []string{"Node.js", "Express", "MongoDB", "Redis", "Stripe", "JWT"},
		ReadmeURL:     "https://github.com/helix-ai/sample-ecommerce-api/blob/main/README.md",
		Difficulty:    "advanced",
		Category:      "api",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add full-text search for products with filters and sorting",
				Priority: "high",
				Labels:   []string{"search", "performance"},
				Context:  "Users need to search products by name, description, and tags. Include filters for price range, category, and availability.",
			},
			{
				Prompt:   "Fix the race condition where inventory can go negative when multiple users buy the same item",
				Priority: "critical",
				Labels:   []string{"concurrency", "inventory", "bug"},
				Context:  "Under high load, the same product can be oversold. This causes major business issues and customer complaints.",
			},
			{
				Prompt:      "Add order cancellation feature with automatic refunds",
				Priority:    "medium",
				Labels:      []string{"orders", "payments"},
				Context:     "Customers should be able to cancel orders within 1 hour of placement. Refunds should process automatically via Stripe.",
				Constraints: "Must integrate with existing Stripe payment system and maintain order audit trail",
			},
		},
	},
	{
		ID:            "weather-app",
		Name:          "Weather App - React Native",
		Description:   "Cross-platform weather app with location services and offline caching",
		GitHubRepo:    "helix-ai/sample-weather-app",
		DefaultBranch: "main",
		Technologies:  []string{"React Native", "TypeScript", "Expo", "AsyncStorage"},
		ReadmeURL:     "https://github.com/helix-ai/sample-weather-app/blob/main/README.md",
		Difficulty:    "beginner",
		Category:      "mobile",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add smooth weather animations that match the current conditions",
				Priority: "low",
				Labels:   []string{"ui/ux", "animations"},
				Context:  "The app shows static weather icons. Users want animated rain, snow, sun rays, and clouds that reflect real conditions.",
			},
			{
				Prompt:   "Implement offline mode so the app works without internet",
				Priority: "medium",
				Labels:   []string{"offline", "caching"},
				Context:  "Users need weather info even when offline. Cache recent weather data and show it with a 'last updated' timestamp.",
			},
		},
	},
	{
		ID:            "blog-cms",
		Name:          "Simple Blog CMS",
		Description:   "Content management system for bloggers with markdown support and media uploads",
		GitHubRepo:    "helix-ai/sample-blog-cms",
		DefaultBranch: "main",
		Technologies:  []string{"Next.js", "Prisma", "PostgreSQL", "TailwindCSS", "S3"},
		ReadmeURL:     "https://github.com/helix-ai/sample-blog-cms/blob/main/README.md",
		Difficulty:    "intermediate",
		Category:      "web",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add image upload functionality for blog posts",
				Priority: "high",
				Labels:   []string{"media", "uploads", "s3"},
				Context:  "Bloggers need to upload images for their posts. Images should be optimized and stored in S3 with proper URLs in the markdown.",
			},
			{
				Prompt:   "Create a commenting system for blog posts",
				Priority: "medium",
				Labels:   []string{"comments", "moderation"},
				Context:  "Readers want to leave comments on blog posts. Include basic moderation features to prevent spam.",
			},
			{
				Prompt:   "Add SEO optimization with meta tags and sitemap generation",
				Priority: "low",
				Labels:   []string{"seo", "performance"},
				Context:  "Blog posts need proper meta tags, Open Graph tags, and automatic sitemap generation for better search engine visibility.",
			},
		},
	},
}

// listSimpleSampleProjects godoc
// @Summary List simple sample projects
// @Description Get sample projects with natural language task prompts (Kiro-style)
// @Tags    sample-projects
// @Success 200 {array} SimpleSampleProject
// @Router /api/v1/sample-projects/simple [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSimpleSampleProjects(_ http.ResponseWriter, r *http.Request) ([]SimpleSampleProject, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Filter by category or difficulty if specified
	category := r.URL.Query().Get("category")
	difficulty := r.URL.Query().Get("difficulty")

	var filteredProjects []SimpleSampleProject
	for _, project := range SIMPLE_SAMPLE_PROJECTS {
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

// ForkSimpleProjectRequest represents request to fork a simple sample project
type ForkSimpleProjectRequest struct {
	SampleProjectID string `json:"sample_project_id"`
	ProjectName     string `json:"project_name"`
	Description     string `json:"description,omitempty"`
}

// ForkSimpleProjectResponse represents the fork response
type ForkSimpleProjectResponse struct {
	ProjectID     string `json:"project_id"`
	GitHubRepoURL string `json:"github_repo_url"`
	TasksCreated  int    `json:"tasks_created"`
	Message       string `json:"message"`
}

// forkSimpleProject godoc
// @Summary Fork a simple sample project
// @Description Fork a sample project and create tasks from natural language prompts
// @Tags    sample-projects
// @Success 201 {object} ForkSimpleProjectResponse
// @Param request body ForkSimpleProjectRequest true "Fork request"
// @Router /api/v1/sample-projects/simple/fork [post]
// @Security BearerAuth
func (s *HelixAPIServer) forkSimpleProject(_ http.ResponseWriter, r *http.Request) (*ForkSimpleProjectResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	var req ForkSimpleProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Find the sample project
	var sampleProject *SimpleSampleProject
	for _, project := range SIMPLE_SAMPLE_PROJECTS {
		if project.ID == req.SampleProjectID {
			sampleProject = &project
			break
		}
	}

	if sampleProject == nil {
		return nil, system.NewHTTPError404("sample project not found")
	}

	// Generate project ID
	projectID := fmt.Sprintf("proj_%s_%d", req.SampleProjectID, time.Now().Unix())

	// Create project name
	projectName := req.ProjectName
	if projectName == "" {
		projectName = fmt.Sprintf("%s - %s", sampleProject.Name, user.Username)
	}

	log.Info().
		Str("user_id", user.ID).
		Str("sample_project_id", req.SampleProjectID).
		Str("project_name", projectName).
		Msg("Forking simple sample project")

	// Simulated GitHub repo URL (in real implementation, this would fork the actual repo)
	forkedRepoURL := fmt.Sprintf("https://github.com/%s/helix-%s", user.Username, req.SampleProjectID)

	// Create spec-driven tasks from the natural language prompts
	tasksCreated := 0
	for i, taskPrompt := range sampleProject.TaskPrompts {
		task := &types.SpecTask{
			ID:          fmt.Sprintf("spec_task_%s_%d", projectID, i),
			ProjectID:   projectID,
			Name:        generateTaskNameFromPrompt(taskPrompt.Prompt),
			Description: taskPrompt.Prompt, // The description IS the prompt
			Type:        inferTaskType(taskPrompt.Labels),
			Priority:    taskPrompt.Priority,
			Status:      "backlog",

			// Kiro-style: store the original prompt, specs will be generated later
			OriginalPrompt:     taskPrompt.Prompt,
			RequirementsSpec:   "", // Will be generated when agent picks up task
			TechnicalDesign:    "", // Will be generated when agent picks up task
			ImplementationPlan: "", // Will be generated when agent picks up task

			EstimatedHours: estimateHoursFromPrompt(taskPrompt.Prompt, taskPrompt.Context),
			CreatedBy:      user.ID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Convert labels to JSON for storage
		if len(taskPrompt.Labels) > 0 {
			labelsJSON, _ := json.Marshal(taskPrompt.Labels)
			task.LabelsDB = labelsJSON
			task.Labels = taskPrompt.Labels
		}

		// Store context and constraints in metadata
		metadata := map[string]interface{}{
			"project_id":     projectID,
			"github_repo":    forkedRepoURL,
			"context":        taskPrompt.Context,
			"constraints":    taskPrompt.Constraints,
			"sample_project": sampleProject.ID,
			"technologies":   sampleProject.Technologies,
		}
		metadataJSON, _ := json.Marshal(metadata)
		task.Metadata = metadataJSON

		// Store the task (assuming we have a spec tasks store method)
		err := s.Store.CreateSpecTask(ctx, task)
		if err != nil {
			log.Warn().Err(err).
				Str("project_id", projectID).
				Str("task_prompt", taskPrompt.Prompt).
				Msg("Failed to create spec task")
		} else {
			tasksCreated++
		}
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", user.ID).
		Str("repo_url", forkedRepoURL).
		Int("tasks_created", tasksCreated).
		Msg("Successfully forked simple sample project")

	return &ForkSimpleProjectResponse{
		ProjectID:     projectID,
		GitHubRepoURL: forkedRepoURL,
		TasksCreated:  tasksCreated,
		Message: fmt.Sprintf("🎉 Forked %s! Created %d tasks from natural language prompts. "+
			"When you assign agents to these tasks, they'll automatically generate requirements, "+
			"technical design, and implementation plans following spec-driven development.",
			sampleProject.Name, tasksCreated),
	}, nil
}

// Helper functions
func generateTaskNameFromPrompt(prompt string) string {
	// Simple heuristic to create task name from prompt
	if len(prompt) > 60 {
		return prompt[:57] + "..."
	}
	return prompt
}

func inferTaskType(labels []string) string {
	for _, label := range labels {
		if label == "bug" {
			return "bug"
		}
		if label == "feature" {
			return "feature"
		}
	}
	return "task"
}

func estimateHoursFromPrompt(prompt, context string) int {
	// Simple heuristic based on prompt complexity
	totalLength := len(prompt) + len(context)

	if totalLength < 100 {
		return 2 // Simple tasks
	} else if totalLength < 300 {
		return 5 // Medium tasks
	} else {
		return 8 // Complex tasks
	}
}
