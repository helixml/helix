package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
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
	Prompt   string                 `json:"prompt"`   // Natural language request (include all context here)
	Priority types.SpecTaskPriority `json:"priority"` // "low", "medium", "high", "critical"
	Labels   []string               `json:"labels"`   // Tags for organization
}

// SIMPLE_SAMPLE_PROJECTS - realistic projects with natural prompts like Kiro expects
var SIMPLE_SAMPLE_PROJECTS = []SimpleSampleProject{
	{
		ID:            "modern-todo-app",
		Name:          "Modern Todo App (React)",
		Description:   "Full-stack todo application with React and Node.js - perfect for learning modern web patterns",
		GitHubRepo:    "helixml/sample-todo-app",
		DefaultBranch: "main",
		Technologies:  []string{"React", "TypeScript", "Node.js", "Express", "PostgreSQL", "Tailwind CSS"},
		ReadmeURL:     "https://github.com/helixml/sample-todo-app/blob/main/README.md",
		DemoURL:       "https://sample-todo-app.vercel.app",
		Difficulty:    "intermediate",
		Category:      "web",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add a dark mode toggle that remembers the user's preference. The app uses React with Tailwind CSS.",
				Priority: "medium",
				Labels:   []string{"frontend", "ui/ux"},
			},
			{
				Prompt:   "Fix the bug where deleting a todo item doesn't remove it from the database. Users report that deleted todos reappear after page refresh.",
				Priority: "high",
				Labels:   []string{"backend", "database", "bug"},
			},
			{
				Prompt:   "Add the ability for users to organize todos into custom categories like 'Work', 'Personal', 'Shopping'. Each category should have a color and be filterable.",
				Priority: "low",
				Labels:   []string{"frontend", "backend", "feature"},
			},
			{
				Prompt:   "Implement user authentication so people can have private todo lists with secure login/register functionality.",
				Priority: "critical",
				Labels:   []string{"backend", "security", "frontend"},
			},
		},
	},
	{
		ID:            "ecommerce-api",
		Name:          "E-commerce REST API (Node.js)",
		Description:   "Comprehensive API for an e-commerce platform with product management and order processing",
		GitHubRepo:    "helixml/sample-ecommerce-api",
		DefaultBranch: "main",
		Technologies:  []string{"Node.js", "Express", "MongoDB", "Redis", "Stripe", "JWT"},
		ReadmeURL:     "https://github.com/helixml/sample-ecommerce-api/blob/main/README.md",
		Difficulty:    "advanced",
		Category:      "api",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add full-text search for products with filters and sorting. Include filters for price range, category, and availability.",
				Priority: "high",
				Labels:   []string{"search", "performance"},
			},
			{
				Prompt:   "Fix the race condition where inventory can go negative when multiple users buy the same item. Under high load, products can be oversold.",
				Priority: "critical",
				Labels:   []string{"concurrency", "inventory", "bug"},
			},
			{
				Prompt:   "Add order cancellation feature with automatic refunds. Customers should be able to cancel orders within 1 hour. Must integrate with existing Stripe payment system.",
				Priority: "medium",
				Labels:   []string{"orders", "payments"},
			},
		},
	},
	{
		ID:            "weather-app",
		Name:          "Weather App - React Native",
		Description:   "Cross-platform weather app with location services and offline caching",
		GitHubRepo:    "helixml/sample-weather-app",
		DefaultBranch: "main",
		Technologies:  []string{"React Native", "TypeScript", "Expo", "AsyncStorage"},
		ReadmeURL:     "https://github.com/helixml/sample-weather-app/blob/main/README.md",
		Difficulty:    "beginner",
		Category:      "mobile",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add smooth weather animations that match the current conditions. Animate rain, snow, sun rays, and clouds that reflect real conditions.",
				Priority: "low",
				Labels:   []string{"ui/ux", "animations"},
			},
			{
				Prompt:   "Implement offline mode so the app works without internet. Cache recent weather data and show it with a 'last updated' timestamp.",
				Priority: "medium",
				Labels:   []string{"offline", "caching"},
			},
		},
	},
	{
		ID:            "blog-cms",
		Name:          "Simple Blog CMS (Next.js)",
		Description:   "Content management system for bloggers with markdown support and media uploads",
		GitHubRepo:    "helixml/sample-blog-cms",
		DefaultBranch: "main",
		Technologies:  []string{"Next.js", "Prisma", "PostgreSQL", "TailwindCSS", "S3"},
		ReadmeURL:     "https://github.com/helixml/sample-blog-cms/blob/main/README.md",
		Difficulty:    "intermediate",
		Category:      "web",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add image upload functionality for blog posts. Images should be optimized and stored in S3 with proper URLs in the markdown.",
				Priority: "high",
				Labels:   []string{"media", "uploads", "s3"},
			},
			{
				Prompt:   "Create a commenting system for blog posts with basic moderation features to prevent spam.",
				Priority: "medium",
				Labels:   []string{"comments", "moderation"},
			},
			{
				Prompt:   "Add SEO optimization with meta tags, Open Graph tags, and automatic sitemap generation for better search engine visibility.",
				Priority: "low",
				Labels:   []string{"seo", "performance"},
			},
		},
	},
	{
		ID:            "react-dashboard",
		Name:          "React Admin Dashboard",
		Description:   "Modern admin dashboard with Material-UI components and data visualization",
		GitHubRepo:    "helixml/sample-react-dashboard",
		DefaultBranch: "main",
		Technologies:  []string{"React", "TypeScript", "Material-UI", "Recharts", "React Router"},
		ReadmeURL:     "https://github.com/helixml/sample-react-dashboard/blob/main/README.md",
		DemoURL:       "https://sample-react-dashboard.vercel.app",
		Difficulty:    "intermediate",
		Category:      "web",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Add real-time data updates to the dashboard using WebSockets. Metrics should update without page refresh.",
				Priority: "high",
				Labels:   []string{"real-time", "websockets", "frontend"},
			},
			{
				Prompt:   "Implement user role-based access control for dashboard features. Different roles (admin, manager, viewer) should see different features.",
				Priority: "critical",
				Labels:   []string{"security", "rbac", "authorization"},
			},
			{
				Prompt:   "Add export functionality to download dashboard data as CSV and PDF for reports.",
				Priority: "medium",
				Labels:   []string{"export", "reports", "feature"},
			},
		},
	},
	{
		ID:            "linkedin-outreach",
		Name:          "LinkedIn Outreach Campaign",
		Description:   "Multi-session campaign to reach out to 100 prospects using LinkedIn automation",
		GitHubRepo:    "helixml/sample-linkedin-outreach",
		DefaultBranch: "main",
		Technologies:  []string{"LinkedIn", "Outreach", "Sales", "Marketing Automation"},
		ReadmeURL:     "https://github.com/helixml/sample-linkedin-outreach/blob/main/README.md",
		Difficulty:    "advanced",
		Category:      "business",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Build prospect list of 100 qualified leads in AI/ML industry. Target CTOs, VPs of Engineering, and AI Leads. Include LinkedIn profiles, company info, and recent activity.",
				Priority: "critical",
				Labels:   []string{"research", "prospecting", "lead-generation"},
			},
			{
				Prompt:   "Write personalized outreach messages based on prospect profiles. Analyze LinkedIn profiles and recent posts to craft connection requests.",
				Priority: "high",
				Labels:   []string{"messaging", "personalization", "outreach"},
			},
			{
				Prompt:   "Set up tracking system for campaign metrics: messages sent, response rate, meetings scheduled, and conversion metrics.",
				Priority: "medium",
				Labels:   []string{"tracking", "analytics", "reporting"},
			},
			{
				Prompt:   "Create 3-touch follow-up sequence for non-responders. Each message should add value and reference recent activity.",
				Priority: "medium",
				Labels:   []string{"follow-up", "automation", "engagement"},
			},
		},
	},
	{
		ID:            "helix-blog-posts",
		Name:          "Helix Technical Blog Series",
		Description:   "Write comprehensive technical blog posts about Helix for developers",
		GitHubRepo:    "helixml/helix",
		DefaultBranch: "main",
		Technologies:  []string{"Technical Writing", "Code Analysis", "Documentation", "Markdown"},
		ReadmeURL:     "https://github.com/helixml/helix/blob/main/README.md",
		Difficulty:    "advanced",
		Category:      "content",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Write 'Getting Started with Helix' tutorial blog post. Walk readers through installation, configuration, and first conversation with real code examples.",
				Priority: "high",
				Labels:   []string{"tutorial", "onboarding", "documentation"},
			},
			{
				Prompt:   "Write architecture deep-dive blog post explaining Helix's session management and multi-model routing. Include sequence diagrams and code snippets.",
				Priority: "high",
				Labels:   []string{"architecture", "technical", "deep-dive"},
			},
			{
				Prompt:   "Write tutorial blog post: Building a Custom Helix Skill. Include complete working example code for a practical skill like GitHub integration.",
				Priority: "medium",
				Labels:   []string{"skills", "integration", "tutorial"},
			},
		},
	},
	{
		ID:            "jupyter-financial-analysis",
		Name:          "Jupyter Financial Analysis",
		Description:   "Financial data analysis using Jupyter notebooks with S&P 500 data, trading signals, and portfolio optimization",
		GitHubRepo:    "", // Created from sample files
		DefaultBranch: "main",
		Technologies:  []string{"Python", "Jupyter", "Pandas", "NumPy", "Finance", "Data Analysis"},
		ReadmeURL:     "",
		Difficulty:    "intermediate",
		Category:      "data-science",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Build comprehensive technical analysis library with SMA/EMA, RSI, MACD, Bollinger Bands. Support multiple timeframes (20/50/200 day) with vectorized Pandas operations.",
				Priority: "high",
				Labels:   []string{"indicators", "technical-analysis", "quantitative"},
			},
			{
				Prompt:   "Implement quantitative risk analytics: VaR (95%/99%), CVaR, maximum drawdown, and Sortino ratio. Visualize risk metrics and stress test scenarios.",
				Priority: "high",
				Labels:   []string{"risk-management", "var", "quantitative"},
			},
			{
				Prompt:   "Build mean-variance portfolio optimization with efficient frontier visualization. Use scipy.optimize for Sharpe ratio maximization.",
				Priority: "critical",
				Labels:   []string{"portfolio-optimization", "modern-portfolio-theory", "quantitative"},
			},
			{
				Prompt:   "Create systematic backtesting engine with performance attribution and alpha/beta analysis. Calculate Sharpe ratio, max drawdown, win rate.",
				Priority: "high",
				Labels:   []string{"backtesting", "alpha-generation", "quantitative"},
			},
		},
	},
	{
		ID:            "data-platform-api-migration",
		Name:          "Data Platform API Migration Suite",
		Description:   "Migrate data pipeline APIs from legacy infrastructure to modern data platform with automated schema mapping and ETL generation",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"Python", "FastAPI", "Apache Airflow", "Pandas", "SQLAlchemy", "Pydantic"},
		ReadmeURL:     "",
		Difficulty:    "advanced",
		Category:      "data-engineering",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Set up FastAPI project structure with 3-5 example legacy API endpoints that return sample data for migration.",
				Priority: "high",
				Labels:   []string{"setup", "fastapi", "architecture"},
			},
			{
				Prompt:   "Create data schema mapping configuration (YAML/JSON) that maps legacy API response schemas to modern data warehouse schemas with type conversions.",
				Priority: "high",
				Labels:   []string{"schema", "data-modeling", "migration"},
			},
			{
				Prompt:   "Generate Airflow DAG that orchestrates migration of all API endpoints in dependency order with proper transformation.",
				Priority: "critical",
				Labels:   []string{"airflow", "orchestration", "etl"},
			},
			{
				Prompt:   "Implement data quality validation checks comparing source API data with migrated warehouse data: row counts, types, referential integrity.",
				Priority: "high",
				Labels:   []string{"data-quality", "validation", "testing"},
			},
			{
				Prompt:   "Add rollback capability that can revert a failed migration and restore previous state if validation fails.",
				Priority: "medium",
				Labels:   []string{"rollback", "recovery", "reliability"},
			},
			{
				Prompt:   "Generate data lineage documentation showing flow from source APIs through transformations to target tables.",
				Priority: "medium",
				Labels:   []string{"documentation", "lineage", "catalog"},
			},
			{
				Prompt:   "Build a migration dashboard showing progress, data volumes, and quality metrics for the API migration project.",
				Priority: "low",
				Labels:   []string{"monitoring", "dashboard", "metrics"},
			},
		},
	},
	{
		ID:            "portfolio-management-dotnet",
		Name:          "Portfolio Management System (.NET)",
		Description:   "Production-grade portfolio management and trade execution system with real-time P&L calculation and compliance",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"C#", ".NET 8", "Entity Framework Core", "In-Memory Queue", "xUnit", "SignalR"},
		ReadmeURL:     "",
		Difficulty:    "advanced",
		Category:      "backend",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Set up .NET 8 project structure with clean architecture: Domain, Application, Infrastructure, and API layers. Use Entity Framework Core.",
				Priority: "critical",
				Labels:   []string{"architecture", "dotnet", "setup"},
			},
			{
				Prompt:   "Implement Portfolio aggregate with position tracking (symbol, quantity, cost basis), cash balances, and real-time P&L calculation.",
				Priority: "critical",
				Labels:   []string{"domain-model", "portfolio", "business-logic"},
			},
			{
				Prompt:   "Build trade execution service with pre-trade compliance checks: sufficient cash, position limits, restricted securities, price limits.",
				Priority: "high",
				Labels:   []string{"trading", "compliance", "validation"},
			},
			{
				Prompt:   "Add in-memory message queue using System.Threading.Channels for async order processing. Publish OrderCreated, OrderFilled, OrderRejected events.",
				Priority: "high",
				Labels:   []string{"messaging", "async", "background-jobs"},
			},
			{
				Prompt:   "Implement real-time market data integration with SignalR to push simulated price updates to clients.",
				Priority: "medium",
				Labels:   []string{"signalr", "real-time", "market-data"},
			},
			{
				Prompt:   "Build comprehensive xUnit tests for portfolio calculations and trade validation. Aim for >80% coverage on business logic.",
				Priority: "high",
				Labels:   []string{"testing", "xunit", "tdd"},
			},
			{
				Prompt:   "Add audit logging tracking portfolio changes and trade executions for regulatory compliance. Include search and export.",
				Priority: "medium",
				Labels:   []string{"audit", "compliance", "logging"},
			},
			{
				Prompt:   "Create REST API with Swagger/OpenAPI documentation for portfolio and trading operations.",
				Priority: "medium",
				Labels:   []string{"api", "swagger", "documentation"},
			},
		},
	},
	{
		ID:            "research-analysis-toolkit",
		Name:          "Research Analysis Toolkit (PyForest)",
		Description:   "Financial research notebooks using PyForest library for backtesting strategies and portfolio optimization",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"Python", "Jupyter", "Pandas", "NumPy", "Matplotlib", "PyForest"},
		ReadmeURL:     "",
		Difficulty:    "intermediate",
		Category:      "data-science",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Create research notebook that uses PyForest to load portfolio data and calculate total value, sector allocation, top positions, daily P&L.",
				Priority: "high",
				Labels:   []string{"setup", "notebook", "pyforest"},
			},
			{
				Prompt:   "Implement SMA crossover strategy (50-day crosses 200-day) and backtest it. Calculate total return, Sharpe ratio, max drawdown, win rate.",
				Priority: "high",
				Labels:   []string{"backtesting", "strategy", "sma"},
			},
			{
				Prompt:   "Build portfolio optimization notebook using scipy.optimize for mean-variance optimization. Visualize efficient frontier and risk-return tradeoff.",
				Priority: "medium",
				Labels:   []string{"optimization", "portfolio", "markowitz"},
			},
			{
				Prompt:   "Add risk factor analysis decomposing portfolio returns into market beta, size factor, value factor, and alpha using Fama-French 3-factor model.",
				Priority: "medium",
				Labels:   []string{"risk", "factor-analysis", "attribution"},
			},
			{
				Prompt:   "Create parameter sweep notebook testing SMA strategy with different period combinations (20/50, 50/100, 50/200). Generate performance heatmap.",
				Priority: "low",
				Labels:   []string{"optimization", "parameter-tuning", "analysis"},
			},
		},
	},
	{
		ID:            "data-validation-toolkit",
		Name:          "Data Validation Toolkit (Jupyter)",
		Description:   "Compare data structures between systems, generate quality reports, and validate migrations",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"Python", "Jupyter", "Pandas", "Great Expectations", "Matplotlib"},
		ReadmeURL:     "",
		Difficulty:    "intermediate",
		Category:      "data-engineering",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Build data profiling notebook analyzing source and target datasets: column types, null counts, unique values, distribution charts.",
				Priority: "high",
				Labels:   []string{"profiling", "analysis", "data-quality"},
			},
			{
				Prompt:   "Implement automated comparison detecting schema mismatches: missing/extra columns, type mismatches, referential integrity. Generate severity-level report.",
				Priority: "critical",
				Labels:   []string{"validation", "schema", "comparison"},
			},
			{
				Prompt:   "Create Great Expectations test suite validating column existence, data types, value ranges, null constraints, uniqueness. Generate HTML report.",
				Priority: "high",
				Labels:   []string{"great-expectations", "testing", "validation"},
			},
			{
				Prompt:   "Build visual data quality dashboard with pass/fail counts, trend charts, top issues. Use matplotlib/seaborn for charts.",
				Priority: "medium",
				Labels:   []string{"visualization", "dashboard", "reporting"},
			},
			{
				Prompt:   "Add row-level data reconciliation identifying missing rows, extra rows, and value differences. Generate CSV report of discrepancies.",
				Priority: "medium",
				Labels:   []string{"reconciliation", "data-quality", "debugging"},
			},
		},
	},
	{
		ID:            "angular-analytics-dashboard",
		Name:          "Multi-Tenant Analytics Dashboard (Angular)",
		Description:   "Multi-tenant analytics dashboard with RBAC, real-time updates, and configurable visualizations",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"Angular", "TypeScript", "RxJS", "NgRx", "PrimeNG", "Chart.js"},
		ReadmeURL:     "",
		Difficulty:    "advanced",
		Category:      "frontend",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Set up Angular 18 project with NgRx state management, PrimeNG components, routing, and sample authentication guard.",
				Priority: "critical",
				Labels:   []string{"setup", "angular", "ngrx"},
			},
			{
				Prompt:   "Implement multi-tenant context service: stores tenant ID, fetches tenant config (colors, logo), filters API calls by tenant, prevents data leakage.",
				Priority: "critical",
				Labels:   []string{"multi-tenant", "context", "isolation"},
			},
			{
				Prompt:   "Create configurable dashboard layout with drag-and-drop widgets (KPI cards, charts, tables) using Angular CDK. Save layout per user.",
				Priority: "high",
				Labels:   []string{"dashboard", "drag-drop", "layout"},
			},
			{
				Prompt:   "Add real-time data updates using RxJS WebSocket integration with automatic reconnection and subscription lifecycle management.",
				Priority: "high",
				Labels:   []string{"websocket", "real-time", "rxjs"},
			},
			{
				Prompt:   "Implement RBAC permission service: check roles (Admin, Manager, Viewer), show/hide UI elements, prevent unauthorized API calls.",
				Priority: "critical",
				Labels:   []string{"rbac", "authorization", "security"},
			},
			{
				Prompt:   "Build data export: generate PDF reports with jsPDF, Excel files with SheetJS. Include filters and date ranges.",
				Priority: "medium",
				Labels:   []string{"export", "reporting", "pdf-excel"},
			},
			{
				Prompt:   "Add Chart.js integration with interactive charts: drill-down, tooltips, responsive mobile, dark/light themes.",
				Priority: "medium",
				Labels:   []string{"charts", "visualization", "chartjs"},
			},
			{
				Prompt:   "Implement global error handler: catch HTTP errors, display user-friendly messages, retry transient failures, prevent crashes.",
				Priority: "medium",
				Labels:   []string{"error-handling", "ux", "resilience"},
			},
		},
	},
	{
		ID:            "angular-version-migration",
		Name:          "Angular Version Migration (15 → 18)",
		Description:   "Migrate an Angular 15 application to Angular 18 with standalone components and modern patterns",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"Angular", "TypeScript", "Migration", "Refactoring"},
		ReadmeURL:     "",
		Difficulty:    "advanced",
		Category:      "frontend",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Update package.json to Angular 18: @angular/* packages to ^18.0.0, TypeScript ~5.4.0, RxJS ^7.8.0. Resolve peer dependency conflicts.",
				Priority: "high",
				Labels:   []string{"dependencies", "npm", "version-upgrade"},
			},
			{
				Prompt:   "Migrate NgModule components to standalone: add standalone: true, move imports to component, use bootstrapApplication.",
				Priority: "critical",
				Labels:   []string{"standalone-components", "refactoring", "architecture"},
			},
			{
				Prompt:   "Update routing to provideRouter() in main.ts, use loadComponent for lazy loading, migrate to functional route guards.",
				Priority: "high",
				Labels:   []string{"routing", "standalone", "migration"},
			},
			{
				Prompt:   "Fix deprecated RxJS operators (do→tap, switch→switchMap), use provideHttpClient(), update interceptors to functional pattern.",
				Priority: "medium",
				Labels:   []string{"rxjs", "http", "migration"},
			},
			{
				Prompt:   "Run 'ng update @angular/core@18 @angular/cli@18', fix DI errors, routing problems, and API changes. Ensure feature parity.",
				Priority: "critical",
				Labels:   []string{"ng-update", "testing", "validation"},
			},
		},
	},
	{
		ID:            "cobol-modernization",
		Name:          "Legacy COBOL Modernization",
		Description:   "Modernize COBOL mainframe code to Python with validation testing",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"COBOL", "Legacy Modernization", "Python"},
		ReadmeURL:     "",
		Difficulty:    "advanced",
		Category:      "modernization",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt:   "Modernize COBOL batch processor to Python using Pandas. Replicate all calculations exactly and maintain identical output format.",
				Priority: "critical",
				Labels:   []string{"implementation", "python", "migration"},
			},
			{
				Prompt:   "Build validation test suite proving Python output exactly matches COBOL output. Implement byte-by-byte comparison for 100% match.",
				Priority: "critical",
				Labels:   []string{"validation", "testing", "quality-assurance"},
			},
			{
				Prompt:   "Create production cutover runbook: pre-migration checklist, parallel run procedures, validation gates, rollback procedures, monitoring steps.",
				Priority: "high",
				Labels:   []string{"runbook", "operations", "deployment"},
			},
		},
	},
	// Clone Feature Demo - 5 shape projects (Brand Color Demo)
	{
		ID:            "clone-demo-shapes",
		Name:          "Brand Color Demo (Clone Feature)",
		Description:   "Demonstrates the clone feature: fill a shape with your brand color, then clone to apply the same color to 4 other shapes. Shows how learnings (the color choice) transfer across repositories.",
		GitHubRepo:    "",
		DefaultBranch: "main",
		Technologies:  []string{"SVG", "HTML", "CSS", "Design"},
		ReadmeURL:     "",
		Difficulty:    "beginner",
		Category:      "clone-demo",
		TaskPrompts: []SampleTaskPrompt{
			{
				Prompt: `Fill shape.svg with the company's brand color.

CRITICAL TIMING INSTRUCTIONS:
- The brand color is NOT specified anywhere - you will need to ask the user
- You are currently in the PLANNING/SPEC-WRITING phase. You will NOT be in the implementation phase until the system explicitly tells you "You are now in the implementation phase" or similar
- During PLANNING (now): Do NOT ask the user for the color. Just write a plan that includes "Ask user for brand color" as a step
- During IMPLEMENTATION (later, when told): THAT is when you actually ask "What is your brand color?" and wait for their response
- Do NOT guess a color or use a placeholder - you MUST wait until implementation to ask

The color discovery happens during implementation, and that learning gets captured in the spec for cloning to other shapes.`,
				Priority: "high",
				Labels:   []string{"design", "branding", "visual"},
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

// forkSimpleProject godoc
// @Summary Fork a simple sample project
// @Description Fork a sample project and create tasks from natural language prompts
// @Tags    sample-projects
// @Success 201 {object} types.ForkSimpleProjectResponse
// @Param request body types.ForkSimpleProjectRequest true "Fork request"
// @Router /api/v1/sample-projects/simple/fork [post]
// @Security BearerAuth
func (s *HelixAPIServer) forkSimpleProject(_ http.ResponseWriter, r *http.Request) (*types.ForkSimpleProjectResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	var req types.ForkSimpleProjectRequest
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

	// Create project name
	projectName := req.ProjectName
	if projectName == "" {
		projectName = sampleProject.Name
	}

	description := req.Description
	if description == "" {
		description = sampleProject.Description
	}

	// Use organization from request (if provided), otherwise it's a personal project
	// Don't auto-assign user's first org - respect their workspace context
	orgID := req.OrganizationID

	// Deduplicate project name within the workspace (org or personal)
	// Build a set of existing names and add (1), (2), etc. if needed
	var existingProjects []*types.Project
	var listErr error
	if orgID != "" {
		existingProjects, listErr = s.Store.ListProjects(ctx, &store.ListProjectsQuery{
			OrganizationID: orgID,
		})
	} else {
		existingProjects, listErr = s.Store.ListProjects(ctx, &store.ListProjectsQuery{
			UserID: user.ID,
		})
	}
	if listErr != nil {
		log.Warn().Err(listErr).Msg("failed to list projects for name deduplication (continuing)")
	}

	existingNames := make(map[string]bool)
	for _, p := range existingProjects {
		existingNames[p.Name] = true
	}

	// Auto-increment name if it already exists: MyProject -> MyProject (1) -> MyProject (2)
	baseName := projectName
	uniqueName := baseName
	suffix := 1
	for existingNames[uniqueName] {
		uniqueName = fmt.Sprintf("%s (%d)", baseName, suffix)
		suffix++
	}
	projectName = uniqueName

	log.Info().
		Str("user_id", user.ID).
		Str("sample_project_id", req.SampleProjectID).
		Str("project_name", projectName).
		Str("organization_id", orgID).
		Msg("Forking simple sample project")

	// Get sample project code to retrieve startup script
	sampleCodeService := services.NewSampleProjectCodeService()
	sampleCode, err := sampleCodeService.GetProjectCode(ctx, req.SampleProjectID)
	var startupScript string
	if err == nil && sampleCode.StartupScript != "" {
		startupScript = sampleCode.StartupScript
	}

	// Create actual Project in database
	project := &types.Project{
		ID:             system.GenerateProjectID(),
		Name:           projectName,
		Description:    description,
		UserID:         user.ID,
		OrganizationID: orgID,
		Technologies:   sampleProject.Technologies,
		StartupScript:  startupScript, // Use sample's startup script
		Status:         "active",
	}

	createdProject, err := s.Store.CreateProject(ctx, project)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_name", projectName).
			Msg("failed to create project for sample")
		return nil, system.NewHTTPError500("failed to create project")
	}

	// Create code repository with sample files
	// NOTE: Internal repos are no longer used - startup script lives in the primary code repo at .helix/startup.sh
	if req.SampleProjectID == "helix-blog-posts" {
		// Special case: Clone real HelixML/helix repo as code repo
		codeRepoPath, repoErr := s.projectInternalRepoService.CloneSampleProject(ctx, createdProject, "https://github.com/helixml/helix.git", user.FullName, user.Email)
		if repoErr != nil {
			log.Error().
				Err(repoErr).
				Str("project_id", createdProject.ID).
				Str("sample_id", req.SampleProjectID).
				Msg("❌ Failed to clone helix-blog-posts repository")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to clone repository: %v", repoErr))
		}

		// Create GitRepository entry for code repo
		codeRepoID := fmt.Sprintf("%s-code", createdProject.ID)
		codeRepo := &types.GitRepository{
			ID:             codeRepoID,
			Name:           data.SlugifyName(createdProject.Name),
			Description:    fmt.Sprintf("Helix codebase for %s", createdProject.Name),
			OwnerID:        user.ID,
			OrganizationID: createdProject.OrganizationID,
			ProjectID:      createdProject.ID,
			RepoType:       "code",
			Status:         "ready",
			LocalPath:      codeRepoPath,
			DefaultBranch:  "main",
			Metadata:       map[string]interface{}{},
			KoditIndexing:  true,
		}

		log.Info().
			Str("project_id", createdProject.ID).
			Str("repo_id", codeRepoID).
			Str("local_path", codeRepoPath).
			Str("organization_id", createdProject.OrganizationID).
			Msg("📝 Creating GitRepository entry for helix code repo")

		err = s.Store.CreateGitRepository(ctx, codeRepo)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", createdProject.ID).
				Str("repo_id", codeRepoID).
				Str("local_path", codeRepoPath).
				Str("organization_id", createdProject.OrganizationID).
				Interface("repo", codeRepo).
				Msg("❌ Failed to create git repository entry for helix code repo")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to create code repository entry: %v", err))
		}

		// Create junction table entry for project-repository relationship
		if err := s.Store.AttachRepositoryToProject(ctx, createdProject.ID, codeRepoID); err != nil {
			log.Warn().Err(err).Str("repo_id", codeRepoID).Str("project_id", createdProject.ID).
				Msg("Failed to attach repository to project via junction table")
		}

		log.Info().
			Str("project_id", createdProject.ID).
			Str("repo_id", codeRepoID).
			Msg("✅ Helix code repository created successfully")

		// Set as primary repo and initialize startup script
		createdProject.DefaultRepoID = codeRepoID
		err = s.Store.UpdateProject(ctx, createdProject)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to set default repository")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to set default repository: %v", err))
		}

		// Initialize startup script in code repo
		if err := s.projectInternalRepoService.InitializeStartupScriptInCodeRepo(codeRepoPath, createdProject.Name, startupScript, user.FullName, user.Email); err != nil {
			log.Warn().Err(err).Msg("Failed to initialize startup script in code repo (continuing)")
		}
	} else if req.SampleProjectID == "jupyter-financial-analysis" {
		// Special case: Create TWO repositories - one for notebooks, one for pyforest library

		// Repository 1: Jupyter notebooks
		notebooksRepoID, notebooksPath, repoErr := s.projectInternalRepoService.InitializeCodeRepoFromSample(ctx, createdProject, "jupyter-notebooks", user.FullName, user.Email)
		if repoErr != nil {
			log.Error().
				Err(repoErr).
				Str("project_id", createdProject.ID).
				Msg("❌ Failed to initialize notebooks repository")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to initialize notebooks repository: %v", repoErr))
		}

		notebooksRepo := &types.GitRepository{
			ID:             notebooksRepoID,
			Name:           fmt.Sprintf("%s-notebooks", data.SlugifyName(createdProject.Name)),
			Description:    "Jupyter notebooks for financial analysis",
			OwnerID:        user.ID,
			OrganizationID: createdProject.OrganizationID,
			ProjectID:      createdProject.ID,
			RepoType:       "code",
			Status:         "ready",
			LocalPath:      notebooksPath,
			DefaultBranch:  "main",
			Metadata:       map[string]interface{}{"repo_purpose": "notebooks"},
			KoditIndexing:  true,
		}

		err = s.Store.CreateGitRepository(ctx, notebooksRepo)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to create notebooks repository entry")
			return nil, system.NewHTTPError500("failed to create notebooks repository")
		}

		// Create junction table entry for project-repository relationship
		if err := s.Store.AttachRepositoryToProject(ctx, createdProject.ID, notebooksRepoID); err != nil {
			log.Warn().Err(err).Str("repo_id", notebooksRepoID).Str("project_id", createdProject.ID).
				Msg("Failed to attach repository to project via junction table")
		}

		log.Info().
			Str("project_id", createdProject.ID).
			Str("repo_id", notebooksRepoID).
			Msg("✅ Notebooks repository created")

		// Repository 2: pyforest library
		pyforestRepoID, pyforestPath, repoErr := s.projectInternalRepoService.InitializeCodeRepoFromSample(ctx, createdProject, "pyforest-library", user.FullName, user.Email)
		if repoErr != nil {
			log.Error().
				Err(repoErr).
				Str("project_id", createdProject.ID).
				Msg("❌ Failed to initialize pyforest library repository")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to initialize pyforest repository: %v", repoErr))
		}

		pyforestRepo := &types.GitRepository{
			ID:             pyforestRepoID,
			Name:           "pyforest",
			Description:    "Python library for financial analysis and portfolio management",
			OwnerID:        user.ID,
			OrganizationID: createdProject.OrganizationID,
			ProjectID:      createdProject.ID,
			RepoType:       "code",
			Status:         "ready",
			LocalPath:      pyforestPath,
			DefaultBranch:  "main",
			Metadata:       map[string]interface{}{"repo_purpose": "library"},
			KoditIndexing:  true,
		}

		err = s.Store.CreateGitRepository(ctx, pyforestRepo)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to create pyforest repository entry")
			return nil, system.NewHTTPError500("failed to create pyforest repository")
		}

		// Create junction table entry for project-repository relationship
		if err := s.Store.AttachRepositoryToProject(ctx, createdProject.ID, pyforestRepoID); err != nil {
			log.Warn().Err(err).Str("repo_id", pyforestRepoID).Str("project_id", createdProject.ID).
				Msg("Failed to attach repository to project via junction table")
		}

		log.Info().
			Str("project_id", createdProject.ID).
			Str("repo_id", pyforestRepoID).
			Msg("✅ PyForest library repository created")

		// Set notebooks repo as primary (default repo)
		createdProject.DefaultRepoID = notebooksRepoID
		err = s.Store.UpdateProject(ctx, createdProject)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to set default repository")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to set default repository: %v", err))
		}

		// Initialize startup script in primary code repo (notebooks)
		if err := s.projectInternalRepoService.InitializeStartupScriptInCodeRepo(notebooksPath, createdProject.Name, startupScript, user.FullName, user.Email); err != nil {
			log.Warn().Err(err).Msg("Failed to initialize startup script in code repo (continuing)")
		}
	} else if req.SampleProjectID == "clone-demo-shapes" {
		// Special case: Create FIVE separate projects for the clone demo
		// Each project has its own repository with a different shape.
		// Only the first (Circle) gets the task - others receive cloned tasks.
		// Projects are created in reverse order so "Start Here" appears first in the UI.

		// Define the 5 shapes - order matters: last created appears first in UI
		shapeConfigs := []struct {
			sampleCodeID string
			nameSuffix   string
			description  string
			hasTask      bool
		}{
			{"clone-demo-shape-star", " - Star", "Star shape - clone target", false},
			{"clone-demo-shape-hexagon", " - Hexagon", "Hexagon shape - clone target", false},
			{"clone-demo-shape-triangle", " - Triangle", "Triangle shape - clone target", false},
			{"clone-demo-shape-square", " - Square", "Square shape - clone target", false},
			{"clone-demo-shape-circle", " - Circle (Start Here)", "Circle shape - START HERE", true},
		}

		// We already created the first project above (with the sample project name)
		// We need to:
		// 1. Delete it (we'll recreate with proper naming)
		// 2. Create all 5 projects with proper names

		// Delete the auto-created project - we'll create 5 new ones
		if delErr := s.Store.DeleteProject(ctx, createdProject.ID); delErr != nil {
			log.Warn().Err(delErr).Str("project_id", createdProject.ID).Msg("Failed to delete placeholder project for clone demo")
		}

		// Track the "start here" project ID to return
		var startHereProjectID string
		var totalTasksCreated int

		// Get external agent app for spec tasks
		var demoAgentApp *types.App
		if req.HelixAppID != "" {
			demoAgentApp, _ = s.Store.GetApp(ctx, req.HelixAppID)
		}
		if demoAgentApp == nil {
			demoAgentApp, _ = s.getUserDefaultExternalAgentApp(ctx, user.ID)
		}

		for _, shapeCfg := range shapeConfigs {
			// Get sample code for this shape
			shapeSampleCode, codeErr := sampleCodeService.GetProjectCode(ctx, shapeCfg.sampleCodeID)
			if codeErr != nil {
				log.Error().Err(codeErr).Str("sample_id", shapeCfg.sampleCodeID).Msg("Failed to get shape sample code")
				continue
			}

			// Create unique project name
			shapeProjectName := baseName + shapeCfg.nameSuffix
			shapeUniqueName := shapeProjectName
			shapeSuffix := 1
			for existingNames[shapeUniqueName] {
				shapeUniqueName = fmt.Sprintf("%s (%d)", shapeProjectName, shapeSuffix)
				shapeSuffix++
			}
			existingNames[shapeUniqueName] = true

			// Create project
			shapeProject := &types.Project{
				ID:             system.GenerateProjectID(),
				Name:           shapeUniqueName,
				Description:    shapeCfg.description,
				UserID:         user.ID,
				OrganizationID: orgID,
				Technologies:   sampleProject.Technologies,
				StartupScript:  shapeSampleCode.StartupScript,
				Status:         "active",
			}

			createdShapeProject, createErr := s.Store.CreateProject(ctx, shapeProject)
			if createErr != nil {
				log.Error().Err(createErr).Str("project_name", shapeUniqueName).Msg("Failed to create shape project")
				continue
			}

			// Create code repository for this shape
			codeRepoID, codeRepoPath, repoErr := s.projectInternalRepoService.InitializeCodeRepoFromSample(ctx, createdShapeProject, shapeCfg.sampleCodeID, user.FullName, user.Email)
			if repoErr != nil {
				log.Error().Err(repoErr).Str("project_id", createdShapeProject.ID).Msg("Failed to initialize shape code repo")
				continue
			}

			codeRepo := &types.GitRepository{
				ID:             codeRepoID,
				Name:           data.SlugifyName(createdShapeProject.Name),
				Description:    fmt.Sprintf("Code repository for %s", createdShapeProject.Name),
				OwnerID:        user.ID,
				OrganizationID: createdShapeProject.OrganizationID,
				ProjectID:      createdShapeProject.ID,
				RepoType:       "code",
				Status:         "ready",
				LocalPath:      codeRepoPath,
				DefaultBranch:  "main",
				Metadata:       map[string]interface{}{"clone_demo": true},
				KoditIndexing:  true,
			}

			if repoCreateErr := s.Store.CreateGitRepository(ctx, codeRepo); repoCreateErr != nil {
				log.Error().Err(repoCreateErr).Str("project_id", createdShapeProject.ID).Msg("Failed to create shape git repo entry")
				continue
			}

			// Create junction table entry for project-repository relationship
			if err := s.Store.AttachRepositoryToProject(ctx, createdShapeProject.ID, codeRepo.ID); err != nil {
				log.Warn().Err(err).Str("repo_id", codeRepo.ID).Str("project_id", createdShapeProject.ID).
					Msg("Failed to attach repository to project via junction table")
			}

			// Set code repo as default
			createdShapeProject.DefaultRepoID = codeRepo.ID
			if demoAgentApp != nil {
				createdShapeProject.DefaultHelixAppID = demoAgentApp.ID
			}
			if updateErr := s.Store.UpdateProject(ctx, createdShapeProject); updateErr != nil {
				log.Warn().Err(updateErr).Msg("Failed to update shape project defaults")
			}

			// Initialize startup script
			if scriptErr := s.projectInternalRepoService.InitializeStartupScriptInCodeRepo(codeRepoPath, createdShapeProject.Name, shapeSampleCode.StartupScript, user.FullName, user.Email); scriptErr != nil {
				log.Warn().Err(scriptErr).Msg("Failed to initialize startup script in shape repo")
			}

			log.Info().
				Str("project_id", createdShapeProject.ID).
				Str("project_name", shapeUniqueName).
				Bool("has_task", shapeCfg.hasTask).
				Msg("✅ Created clone demo shape project")

			// Create task only for the "Start Here" project
			if shapeCfg.hasTask {
				startHereProjectID = createdShapeProject.ID

				// Create the brand color task
				for i := len(sampleProject.TaskPrompts) - 1; i >= 0; i-- {
					taskPrompt := sampleProject.TaskPrompts[i]
					task := &types.SpecTask{
						ID:             system.GenerateSpecTaskID(),
						ProjectID:      createdShapeProject.ID,
						Name:           generateTaskNameFromPrompt(taskPrompt.Prompt),
						Description:    taskPrompt.Prompt,
						Type:           inferTaskType(taskPrompt.Labels),
						Priority:       taskPrompt.Priority,
						Status:         "backlog",
						OriginalPrompt: taskPrompt.Prompt,
						CreatedBy:      user.ID,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}

					if demoAgentApp != nil {
						task.HelixAppID = demoAgentApp.ID
					}

					task.Labels = taskPrompt.Labels
					task.Metadata = map[string]interface{}{
						"sample_project": sampleProject.ID,
						"clone_demo":     true,
					}

					if taskErr := s.Store.CreateSpecTask(ctx, task); taskErr != nil {
						log.Warn().Err(taskErr).Msg("Failed to create clone demo task")
					} else {
						totalTasksCreated++
						// Log audit event for task creation
						if s.auditLogService != nil {
							s.auditLogService.LogTaskCreated(ctx, task, user.ID, user.Email)
						}
					}
				}
			}

			// Small delay between project creations to ensure proper ordering by created_at
			time.Sleep(50 * time.Millisecond)
		}

		log.Info().
			Str("start_here_project_id", startHereProjectID).
			Int("tasks_created", totalTasksCreated).
			Msg("✅ Clone demo: Created 5 shape projects")

		return &types.ForkSimpleProjectResponse{
			ProjectID:    startHereProjectID,
			TasksCreated: totalTasksCreated,
			Message: fmt.Sprintf("Created 5 shape projects for the clone demo. "+
				"Start with the 'Circle (Start Here)' project - the agent will ask you for your brand color. "+
				"Then clone the task to fill the other 4 shapes (Square, Triangle, Hexagon, Star) with the same color. "+
				"%d task(s) ready to work on.", totalTasksCreated),
		}, nil
	} else {
		// Use hardcoded sample code for all other samples
		codeRepoID, codeRepoPath, repoErr := s.projectInternalRepoService.InitializeCodeRepoFromSample(ctx, createdProject, req.SampleProjectID, user.FullName, user.Email)
		if repoErr != nil {
			log.Error().
				Err(repoErr).
				Str("project_id", createdProject.ID).
				Str("sample_id", req.SampleProjectID).
				Msg("❌ Failed to initialize code repository from sample")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to initialize code repository: %v", repoErr))
		}

		// Create GitRepository entry for code repo
		codeRepo := &types.GitRepository{
			ID:             codeRepoID,
			Name:           data.SlugifyName(createdProject.Name),
			Description:    fmt.Sprintf("Code repository for %s", createdProject.Name),
			OwnerID:        user.ID,
			OrganizationID: createdProject.OrganizationID,
			ProjectID:      createdProject.ID,
			RepoType:       "code", // Helix-hosted code repo (cloned from LocalPath)
			Status:         "ready",
			LocalPath:      codeRepoPath,
			DefaultBranch:  "main",
			Metadata:       map[string]interface{}{},
			KoditIndexing:  true,
		}

		log.Info().
			Str("project_id", createdProject.ID).
			Str("repo_id", codeRepoID).
			Str("local_path", codeRepoPath).
			Str("organization_id", createdProject.OrganizationID).
			Msg("📝 Creating GitRepository entry for sample code repo")

		err = s.Store.CreateGitRepository(ctx, codeRepo)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", createdProject.ID).
				Str("repo_id", codeRepoID).
				Str("local_path", codeRepoPath).
				Str("organization_id", createdProject.OrganizationID).
				Interface("repo", codeRepo).
				Msg("❌ Failed to create git repository entry for sample code repo")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to create code repository entry: %v", err))
		}

		// Create junction table entry for project-repository relationship
		if err := s.Store.AttachRepositoryToProject(ctx, createdProject.ID, codeRepoID); err != nil {
			log.Warn().Err(err).Str("repo_id", codeRepoID).Str("project_id", createdProject.ID).
				Msg("Failed to attach repository to project via junction table")
		}

		log.Info().
			Str("project_id", createdProject.ID).
			Str("repo_id", codeRepoID).
			Msg("✅ Sample code repository created successfully")

		// Set code repo as default
		createdProject.DefaultRepoID = codeRepo.ID
		err = s.Store.UpdateProject(ctx, createdProject)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", createdProject.ID).
				Str("default_repo_id", codeRepo.ID).
				Msg("❌ Failed to set default repository")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to set default repository: %v", err))
		}

		// Initialize startup script in code repo
		if err := s.projectInternalRepoService.InitializeStartupScriptInCodeRepo(codeRepoPath, createdProject.Name, startupScript, user.FullName, user.Email); err != nil {
			log.Warn().Err(err).Msg("Failed to initialize startup script in code repo (continuing)")
		}
	}

	// Get external agent app for spec tasks
	// Use the explicitly provided app ID, or fall back to user's default
	var agentApp *types.App
	if req.HelixAppID != "" {
		// User explicitly selected an agent
		agentApp, err = s.Store.GetApp(ctx, req.HelixAppID)
		if err != nil {
			log.Warn().Err(err).
				Str("user_id", user.ID).
				Str("helix_app_id", req.HelixAppID).
				Msg("Failed to get specified agent app, falling back to default")
			agentApp = nil
		}
	}
	// Fall back to user's default if no explicit app specified or if lookup failed
	if agentApp == nil {
		agentApp, err = s.getUserDefaultExternalAgentApp(ctx, user.ID)
		if err != nil {
			log.Warn().Err(err).
				Str("user_id", user.ID).
				Msg("Failed to get default external agent app, spec tasks may fail to start")
		}
	}

	// Set the project's default agent if we have one
	if agentApp != nil {
		createdProject.DefaultHelixAppID = agentApp.ID
		err = s.Store.UpdateProject(ctx, createdProject)
		if err != nil {
			log.Warn().Err(err).
				Str("project_id", createdProject.ID).
				Str("default_helix_app_id", agentApp.ID).
				Msg("Failed to set project's default agent (continuing)")
		} else {
			log.Info().
				Str("project_id", createdProject.ID).
				Str("default_helix_app_id", agentApp.ID).
				Msg("Set project's default agent")
		}
	}

	// Create spec-driven tasks from the natural language prompts
	// Iterate in reverse order so first task appears at top of backlog (newest CreatedAt)
	tasksCreated := 0
	for i := len(sampleProject.TaskPrompts) - 1; i >= 0; i-- {
		taskPrompt := sampleProject.TaskPrompts[i]
		task := &types.SpecTask{
			ID:          system.GenerateSpecTaskID(),
			ProjectID:   createdProject.ID,
			Name:        generateTaskNameFromPrompt(taskPrompt.Prompt),
			Description: taskPrompt.Prompt, // The description IS the prompt
			Type:        inferTaskType(taskPrompt.Labels),
			Priority:    taskPrompt.Priority,
			Status:      "backlog",

			// Store the original prompt, specs will be generated later
			OriginalPrompt:     taskPrompt.Prompt,
			RequirementsSpec:   "", // Will be generated when agent picks up task
			TechnicalDesign:    "", // Will be generated when agent picks up task
			ImplementationPlan: "", // Will be generated when agent picks up task

			CreatedBy: user.ID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Set HelixAppID if we found an agent app
		if agentApp != nil {
			task.HelixAppID = agentApp.ID
		}

		// Store labels directly (GORM serializer handles JSON conversion)
		task.Labels = taskPrompt.Labels

		// Store sample project ID in metadata
		task.Metadata = map[string]interface{}{
			"sample_project": sampleProject.ID,
		}

		err := s.Store.CreateSpecTask(ctx, task)
		if err != nil {
			log.Warn().Err(err).
				Str("project_id", createdProject.ID).
				Str("task_prompt", taskPrompt.Prompt).
				Msg("Failed to create spec task")
		} else {
			tasksCreated++
			// Log audit event for task creation
			if s.auditLogService != nil {
				s.auditLogService.LogTaskCreated(ctx, task, user.ID, user.Email)
			}
		}
	}

	log.Info().
		Str("project_id", createdProject.ID).
		Str("user_id", user.ID).
		Int("tasks_created", tasksCreated).
		Msg("Successfully forked simple sample project")

	return &types.ForkSimpleProjectResponse{
		ProjectID:     createdProject.ID,
		GitHubRepoURL: sampleProject.GitHubRepo,
		TasksCreated:  tasksCreated,
		Message: fmt.Sprintf("Created %s with %d tasks from natural language prompts. "+
			"Assign agents to these tasks to automatically generate requirements, "+
			"technical design, and implementation plans.",
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

