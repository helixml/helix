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
	Prompt      string                 `json:"prompt"`      // Natural language request
	Priority    types.SpecTaskPriority `json:"priority"`    // "low", "medium", "high", "critical"
	Labels      []string               `json:"labels"`      // Tags for organization
	Context     string                 `json:"context"`     // Additional context about the codebase
	Constraints string                 `json:"constraints"` // Any specific constraints or requirements
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
		GitHubRepo:    "helixml/sample-weather-app",
		DefaultBranch: "main",
		Technologies:  []string{"React Native", "TypeScript", "Expo", "AsyncStorage"},
		ReadmeURL:     "https://github.com/helixml/sample-weather-app/blob/main/README.md",
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
				Prompt:   "Add real-time data updates to the dashboard using WebSockets",
				Priority: "high",
				Labels:   []string{"real-time", "websockets", "frontend"},
				Context:  "Dashboard metrics should update in real-time without page refresh. Use WebSocket connection to backend for live data.",
			},
			{
				Prompt:   "Implement user role-based access control for dashboard features",
				Priority: "critical",
				Labels:   []string{"security", "rbac", "authorization"},
				Context:  "Different user roles (admin, manager, viewer) should see different dashboard features and data based on permissions.",
			},
			{
				Prompt:   "Add export functionality to download dashboard data as CSV and PDF",
				Priority: "medium",
				Labels:   []string{"export", "reports", "feature"},
				Context:  "Users need to export dashboard data and charts for reports. Support both CSV data export and PDF report generation.",
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
				Prompt:   "Build prospect list of 100 qualified leads in AI/ML industry",
				Priority: "critical",
				Labels:   []string{"research", "prospecting", "lead-generation"},
				Context:  "Create a list of 100 qualified prospects matching our ICP (CTOs, VPs of Engineering, AI Leads). Include their LinkedIn profiles, company info, and recent activity.",
			},
			{
				Prompt:   "Write personalized outreach messages based on prospect profiles",
				Priority: "high",
				Labels:   []string{"messaging", "personalization", "outreach"},
				Context:  "For each prospect, analyze their LinkedIn profile and recent posts to craft personalized connection requests and follow-up messages.",
			},
			{
				Prompt:   "Set up tracking system for campaign metrics and responses",
				Priority: "medium",
				Labels:   []string{"tracking", "analytics", "reporting"},
				Context:  "Build a tracking sheet to monitor: messages sent, response rate, positive responses, meetings scheduled, and conversion metrics.",
			},
			{
				Prompt:   "Create follow-up sequence for non-responders",
				Priority: "medium",
				Labels:   []string{"follow-up", "automation", "engagement"},
				Context:  "Design a 3-touch follow-up sequence for prospects who don't respond. Each message should add value and reference recent activity.",
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
				Prompt:   "Write 'Getting Started with Helix' tutorial blog post with step-by-step examples",
				Priority: "high",
				Labels:   []string{"tutorial", "onboarding", "documentation"},
				Context:  "Deliverable: One complete blog post. Planning phase: Agent will analyze Helix codebase for installation/setup process, identify key first-time-user workflows, outline tutorial sections. Implementation phase: Write beginner-friendly tutorial post walking readers through their first Helix AI assistant - installation, configuration, first conversation - with real code examples from the repository.",
			},
			{
				Prompt:   "Write architecture deep-dive blog post explaining Helix's session management and multi-model routing",
				Priority: "high",
				Labels:   []string{"architecture", "technical", "deep-dive"},
				Context:  "Deliverable: One technical blog post. Planning phase: Agent will study api/pkg/ structure, trace session lifecycle, understand model orchestration, plan article structure with specific code references. Implementation phase: Write comprehensive post with sequence diagrams showing how requests flow through Helix, how sessions are managed, and how multi-model routing works. Include real code snippets.",
			},
			{
				Prompt:   "Write tutorial blog post: Building a Custom Helix Skill (with complete working example)",
				Priority: "medium",
				Labels:   []string{"skills", "integration", "tutorial"},
				Context:  "Deliverable: One tutorial blog post + working skill code. Planning phase: Agent will analyze existing skills implementation, design a practical example skill (e.g., GitHub integration), outline tutorial steps. Implementation phase: Write hands-on tutorial showing how to build a custom skill from scratch, including complete working code for the example skill.",
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
				Prompt:   "Build comprehensive technical analysis library with momentum and trend indicators",
				Priority: "high",
				Labels:   []string{"indicators", "technical-analysis", "quantitative"},
				Context:  "Deliverable: pyforest indicators module. Planning phase: Agent will design indicator library architecture, plan calculations for SMA/EMA, RSI, MACD, Bollinger Bands. Implementation phase: Implement all indicators in pyforest/indicators.py with vectorized Pandas operations, create visualizations overlaying price charts, identify buy/sell signal crossovers. Support multiple timeframes (20/50/200 day).",
			},
			{
				Prompt:   "Implement quantitative risk analytics: VaR, CVaR, maximum drawdown, and Sortino ratio",
				Priority: "high",
				Labels:   []string{"risk-management", "var", "quantitative"},
				Context:  "Deliverable: Risk metrics module. Planning phase: Agent will design risk calculation methodology, plan VaR/CVaR estimation approach (historical simulation), identify drawdown calculation algorithm. Implementation phase: Implement Value-at-Risk (95%/99% confidence), Conditional VaR, maximum drawdown (peak-to-trough), Sortino ratio (downside deviation). Visualize risk metrics and stress test scenarios.",
			},
			{
				Prompt:   "Build mean-variance portfolio optimization with efficient frontier visualization",
				Priority: "critical",
				Labels:   []string{"portfolio-optimization", "modern-portfolio-theory", "quantitative"},
				Context:  "Deliverable: Portfolio optimizer. Planning phase: Agent will design optimization framework, plan efficient frontier calculation, identify constraints (long-only, max allocation). Implementation phase: Use scipy.optimize for mean-variance optimization, implement Sharpe ratio maximization, calculate efficient frontier, visualize optimal portfolios, compare against equal-weight and market-cap-weighted benchmarks.",
			},
			{
				Prompt:   "Create systematic backtesting engine with performance attribution and alpha/beta analysis",
				Priority: "high",
				Labels:   []string{"backtesting", "alpha-generation", "quantitative"},
				Context:  "Deliverable: Backtesting framework. Planning phase: Agent will design backtest simulator, plan transaction cost modeling, identify performance metrics (alpha, beta, information ratio). Implementation phase: Build event-driven backtester simulating technical indicator strategies, calculate alpha/beta vs S&P 500 benchmark, compute Sharpe ratio, max drawdown, win rate. Generate tearsheets with performance attribution.",
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
				Prompt:   "Set up the FastAPI project structure with example legacy API endpoints to migrate",
				Priority: "high",
				Labels:   []string{"setup", "fastapi", "architecture"},
				Context:  "Create a working FastAPI application with 3-5 example legacy API endpoints that return sample data. These represent the APIs we need to migrate to a modern data platform.",
			},
			{
				Prompt:   "Create data schema mapping configuration that maps legacy API response schemas to modern data warehouse schemas",
				Priority: "high",
				Labels:   []string{"schema", "data-modeling", "migration"},
				Context:  "Build a schema mapping system (YAML or JSON) that defines how legacy API fields map to target data warehouse columns. Include data type conversions and transformations.",
			},
			{
				Prompt:   "Generate Airflow DAG that orchestrates the migration of all API endpoints in dependency order",
				Priority: "critical",
				Labels:   []string{"airflow", "orchestration", "etl"},
				Context:  "Create an Airflow DAG that calls each legacy API, transforms the data using the schema mappings, and loads it into the target data warehouse. Handle dependencies between APIs.",
			},
			{
				Prompt:   "Implement data quality validation checks that compare source API data with migrated warehouse data",
				Priority: "high",
				Labels:   []string{"data-quality", "validation", "testing"},
				Context:  "Build validation framework that checks: row counts match, data types are correct, key fields are populated, referential integrity is maintained, no data loss occurred.",
			},
			{
				Prompt:   "Add rollback capability that can revert a failed migration and restore previous state",
				Priority: "medium",
				Labels:   []string{"rollback", "recovery", "reliability"},
				Context:  "Implement migration versioning and rollback mechanism. Should be able to undo a migration and return to the previous data state if validation fails.",
			},
			{
				Prompt:   "Generate data lineage documentation showing the flow from source APIs through transformations to target tables",
				Priority: "medium",
				Labels:   []string{"documentation", "lineage", "catalog"},
				Context:  "Create automated documentation that shows: which APIs feed which tables, what transformations are applied, data freshness/update schedules, and dependencies between datasets.",
			},
			{
				Prompt:   "Build a migration dashboard that shows progress, data volumes, and quality metrics for the API migration project",
				Priority: "low",
				Labels:   []string{"monitoring", "dashboard", "metrics"},
				Context:  "Create a simple web dashboard (can be Flask or FastAPI + HTML) showing: APIs migrated vs remaining, data volume processed, validation results, error rates.",
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
				Prompt:   "Set up the .NET 8 project structure with clean architecture (Domain, Application, Infrastructure layers)",
				Priority: "critical",
				Labels:   []string{"architecture", "dotnet", "setup"},
				Context:  "Create a .NET solution with separate projects for Domain (entities, interfaces), Application (business logic, DTOs), Infrastructure (database, external services), and API (controllers). Use Entity Framework Core for data access.",
			},
			{
				Prompt:   "Implement the Portfolio aggregate with position tracking, cash balances, and P&L calculation",
				Priority: "critical",
				Labels:   []string{"domain-model", "portfolio", "business-logic"},
				Context:  "Create Portfolio entity that tracks positions (symbol, quantity, cost basis), cash balance, and calculates real-time P&L based on current market prices. Include methods for adding/removing positions.",
			},
			{
				Prompt:   "Build the trade execution service with pre-trade compliance checks and order validation",
				Priority: "high",
				Labels:   []string{"trading", "compliance", "validation"},
				Context:  "Implement trade execution that validates: sufficient cash/positions, position limits not exceeded, no restricted securities, valid price limits. Return detailed validation errors.",
			},
			{
				Prompt:   "Add in-memory message queue for asynchronous order processing and event publishing",
				Priority: "high",
				Labels:   []string{"messaging", "async", "background-jobs"},
				Context:  "Implement an in-memory message queue using System.Threading.Channels to process trade orders asynchronously. Publish events for: OrderCreated, OrderFilled, OrderRejected, PositionUpdated. This allows local development without external dependencies.",
			},
			{
				Prompt:   "Implement real-time market data integration with SignalR to push price updates to clients",
				Priority: "medium",
				Labels:   []string{"signalr", "real-time", "market-data"},
				Context:  "Create SignalR hub that pushes simulated real-time market price updates. Clients can subscribe to specific symbols and receive price updates every few seconds.",
			},
			{
				Prompt:   "Build comprehensive unit tests for portfolio calculations and trade validation logic",
				Priority: "high",
				Labels:   []string{"testing", "xunit", "tdd"},
				Context:  "Write xUnit tests covering: P&L calculations, position updates, compliance rule validation, edge cases (fractional shares, short positions, margin). Aim for >80% code coverage on business logic.",
			},
			{
				Prompt:   "Add audit logging that tracks all portfolio changes and trade executions for regulatory compliance",
				Priority: "medium",
				Labels:   []string{"audit", "compliance", "logging"},
				Context:  "Implement audit trail that logs: who made changes, what changed, when, why (if provided). Store in separate audit table. Include search and export capabilities for regulatory reviews.",
			},
			{
				Prompt:   "Create a REST API with Swagger/OpenAPI documentation for all portfolio and trading operations",
				Priority: "medium",
				Labels:   []string{"api", "swagger", "documentation"},
				Context:  "Build RESTful API controllers with full Swagger annotations. Endpoints for: get portfolio, get positions, place order, get order status, get P&L summary, get transaction history.",
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
				Prompt:   "Create initial research notebook that uses PyForest to load sample portfolio data and calculate basic metrics",
				Priority: "high",
				Labels:   []string{"setup", "notebook", "pyforest"},
				Context:  "Build a Jupyter notebook that imports PyForest, loads sample portfolio holdings data (CSV or mock data), and calculates: total value, sector allocation, top positions, daily P&L.",
			},
			{
				Prompt:   "Implement a simple moving average crossover strategy and backtest it against historical data",
				Priority: "high",
				Labels:   []string{"backtesting", "strategy", "sma"},
				Context:  "Create strategy that buys when 50-day SMA crosses above 200-day SMA, sells on opposite cross. Backtest on sample tickers, calculate: total return, Sharpe ratio, max drawdown, win rate.",
			},
			{
				Prompt:   "Build portfolio optimization notebook that finds optimal asset weights using mean-variance optimization",
				Priority: "medium",
				Labels:   []string{"optimization", "portfolio", "markowitz"},
				Context:  "Use scipy.optimize to find portfolio weights that maximize Sharpe ratio for a basket of 5-10 stocks. Compare efficient frontier of different weight combinations. Visualize risk-return tradeoff.",
			},
			{
				Prompt:   "Add risk factor analysis that decomposes portfolio returns into systematic and idiosyncratic components",
				Priority: "medium",
				Labels:   []string{"risk", "factor-analysis", "attribution"},
				Context:  "Implement factor model (Fama-French 3-factor or simple market model). Decompose portfolio returns into: market beta, size factor, value factor, alpha. Show which factors drive performance.",
			},
			{
				Prompt:   "Create parameter sweep notebook that tests strategy performance across different parameter combinations",
				Priority: "low",
				Labels:   []string{"optimization", "parameter-tuning", "analysis"},
				Context:  "Test SMA crossover strategy with different period combinations (20/50, 50/100, 50/200, 100/200). Generate heatmap showing performance across parameter space to identify optimal parameters.",
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
				Prompt:   "Build a data profiling notebook that analyzes structure and quality of source and target datasets",
				Priority: "high",
				Labels:   []string{"profiling", "analysis", "data-quality"},
				Context:  "Create notebook that profiles both datasets: column names/types, null counts, unique values, min/max/mean, distribution charts. Identify structural differences between source and target.",
			},
			{
				Prompt:   "Implement automated comparison logic that detects schema mismatches and data type inconsistencies",
				Priority: "critical",
				Labels:   []string{"validation", "schema", "comparison"},
				Context:  "Compare source vs target: missing/extra columns, data type mismatches, constraint violations, referential integrity issues. Generate detailed mismatch report with severity levels.",
			},
			{
				Prompt:   "Create Great Expectations test suite that validates data quality rules for the migration",
				Priority: "high",
				Labels:   []string{"great-expectations", "testing", "validation"},
				Context:  "Define Great Expectations suite with expectations for: column existence, data types, value ranges, null constraints, uniqueness, referential integrity. Generate HTML validation report.",
			},
			{
				Prompt:   "Build visual data quality dashboard showing validation results with charts and summary statistics",
				Priority: "medium",
				Labels:   []string{"visualization", "dashboard", "reporting"},
				Context:  "Create comprehensive visual report with: pass/fail counts by category, trend charts over time, top data quality issues, affected row counts. Use matplotlib/seaborn for charts.",
			},
			{
				Prompt:   "Add row-level data reconciliation that identifies specific records with discrepancies",
				Priority: "medium",
				Labels:   []string{"reconciliation", "data-quality", "debugging"},
				Context:  "Compare source and target datasets row-by-row. Identify: missing rows, extra rows, rows with different values. Generate CSV report of discrepancies for investigation.",
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
				Prompt:   "Set up Angular 18 project with NgRx state management and PrimeNG component library",
				Priority: "critical",
				Labels:   []string{"setup", "angular", "ngrx"},
				Context:  "Create new Angular application with: NgRx for state management, PrimeNG for UI components, routing configured, environment setup for dev/prod. Include sample authentication guard.",
			},
			{
				Prompt:   "Implement multi-tenant context management that switches data and branding based on selected tenant",
				Priority: "critical",
				Labels:   []string{"multi-tenant", "context", "isolation"},
				Context:  "Build tenant context service that: stores current tenant ID, fetches tenant-specific configuration (colors, logo, name), filters all API calls by tenant, prevents cross-tenant data leakage.",
			},
			{
				Prompt:   "Create configurable dashboard layout with drag-and-drop widget positioning using Angular CDK",
				Priority: "high",
				Labels:   []string{"dashboard", "drag-drop", "layout"},
				Context:  "Build dashboard grid where users can add/remove/resize widgets. Widgets include: KPI cards, line charts, bar charts, data tables. Save layout preferences per user. Use Angular CDK drag-drop.",
			},
			{
				Prompt:   "Add real-time data updates using RxJS WebSocket integration with automatic reconnection",
				Priority: "high",
				Labels:   []string{"websocket", "real-time", "rxjs"},
				Context:  "Implement WebSocket service with RxJS that: connects to backend WebSocket, handles reconnection on disconnect, broadcasts updates to dashboard widgets, manages subscription lifecycle.",
			},
			{
				Prompt:   "Implement role-based access control (RBAC) that shows/hides features based on user permissions",
				Priority: "critical",
				Labels:   []string{"rbac", "authorization", "security"},
				Context:  "Create permission service that: checks user roles (Admin, Manager, Viewer), shows/hides UI elements based on permissions, prevents unauthorized API calls, handles permission changes without page reload.",
			},
			{
				Prompt:   "Build data export functionality that generates PDF reports and Excel downloads from dashboard data",
				Priority: "medium",
				Labels:   []string{"export", "reporting", "pdf-excel"},
				Context:  "Add export buttons to dashboard that: generate PDF reports with charts using jsPDF, create Excel files with data tables using SheetJS, include filters and date ranges in exports.",
			},
			{
				Prompt:   "Add Chart.js integration with interactive charts that respond to user filters and selections",
				Priority: "medium",
				Labels:   []string{"charts", "visualization", "chartjs"},
				Context:  "Integrate Chart.js for data visualization. Charts should: update when filters change, support drill-down on click, show tooltips with details, be responsive on mobile, support dark/light themes.",
			},
			{
				Prompt:   "Implement comprehensive error handling with user-friendly error messages and retry logic",
				Priority: "medium",
				Labels:   []string{"error-handling", "ux", "resilience"},
				Context:  "Build global error handler that: catches HTTP errors, displays user-friendly messages, offers retry for transient failures, logs errors for debugging, prevents app crashes from unhandled exceptions.",
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
				Prompt:   "Update package.json dependencies from Angular 15 to Angular 18 and resolve peer dependency conflicts",
				Priority: "high",
				Labels:   []string{"dependencies", "npm", "version-upgrade"},
				Context:  "Update all @angular/* packages to ^18.0.0, update TypeScript to ~5.4.0, update RxJS to ^7.8.0. Run npm install and resolve any peer dependency warnings. Document any packages that need major version updates.",
			},
			{
				Prompt:   "Migrate NgModule-based components to standalone components throughout the application",
				Priority: "critical",
				Labels:   []string{"standalone-components", "refactoring", "architecture"},
				Context:  "Convert all components from NgModule declarations to standalone components. Add standalone: true, move imports from NgModule to component imports array. Update app bootstrapping to use bootstrapApplication.",
			},
			{
				Prompt:   "Update routing configuration to use new provideRouter and standalone routing patterns",
				Priority: "high",
				Labels:   []string{"routing", "standalone", "migration"},
				Context:  "Replace RouterModule.forRoot() with provideRouter() in main.ts. Update lazy loading to use loadComponent instead of loadChildren. Migrate route guards to functional guards.",
			},
			{
				Prompt:   "Fix deprecated RxJS operators and update HttpClient to use new functional providers",
				Priority: "medium",
				Labels:   []string{"rxjs", "http", "migration"},
				Context:  "Replace deprecated RxJS operators (do() → tap(), switch() → switchMap()). Update HttpClient to use provideHttpClient() in main.ts. Update interceptor registration to functional pattern.",
			},
			{
				Prompt:   "Run ng update schematics and fix any runtime errors from the automated migration",
				Priority: "critical",
				Labels:   []string{"ng-update", "testing", "validation"},
				Context:  "Use 'ng update @angular/core@18 @angular/cli@18' to run automated migrations. Test all routes and features, fix dependency injection errors, routing problems, and API changes. Ensure feature parity with Angular 15 version.",
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
				Prompt:   "Modernize COBOL batch processor to Python maintaining identical business logic",
				Priority: "critical",
				Labels:   []string{"implementation", "python", "migration"},
				Context:  "Deliverable: Python code replacing COBOL. Planning phase: Agent will analyze COBOL source code, reverse-engineer business logic and data flows, document all calculations and edge cases, create Python architecture design. Implementation phase: Agent will code Python version using Pandas for data processing, replicate COBOL calculations exactly, maintain identical output format.",
			},
			{
				Prompt:   "Build validation test suite proving Python output exactly matches COBOL output",
				Priority: "critical",
				Labels:   []string{"validation", "testing", "quality-assurance"},
				Context:  "Deliverable: Automated test suite. Planning phase: Agent will design test strategy, identify edge cases, plan test data generation approach. Implementation phase: Generate test input files, automate running both COBOL and Python versions, implement byte-by-byte output comparison, create reports showing 100% match for all test cases.",
			},
			{
				Prompt:   "Create production cutover runbook with step-by-step migration procedures",
				Priority: "high",
				Labels:   []string{"runbook", "operations", "deployment"},
				Context:  "Deliverable: Detailed migration runbook document. Planning phase: Agent will identify cutover requirements, design parallel-run approach, plan rollback strategy. Implementation phase: Write comprehensive runbook including pre-migration checklist, parallel run procedures, validation gates, rollback procedures, post-migration monitoring steps, and escalation paths.",
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
				Prompt:   "Fill the shape with the company's brand color. You MUST ask the user what color they want - do not guess or make up a color.",
				Priority: "high",
				Labels:   []string{"design", "branding", "visual"},
				Context:  "The shape.svg file contains a shape with a white fill and black outline. You need to change the fill color to match the company's brand color. CRITICAL: The brand color is NOT specified anywhere - not in code, not in config, not in comments. You MUST stop and ask the user 'What is your brand color?' before making any changes. Do NOT guess, do NOT use a placeholder, do NOT pick a color yourself. Wait for the user to tell you the exact color.",
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
						EstimatedHours: estimateHoursFromPrompt(taskPrompt.Prompt, taskPrompt.Context),
						CreatedBy:      user.ID,
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					}

					if demoAgentApp != nil {
						task.HelixAppID = demoAgentApp.ID
					}

					task.Labels = taskPrompt.Labels
					task.Metadata = map[string]interface{}{
						"context":        taskPrompt.Context,
						"constraints":    taskPrompt.Constraints,
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

			EstimatedHours: estimateHoursFromPrompt(taskPrompt.Prompt, taskPrompt.Context),
			CreatedBy:      user.ID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Set HelixAppID if we found an agent app
		if agentApp != nil {
			task.HelixAppID = agentApp.ID
		}

		// Store labels directly (GORM serializer handles JSON conversion)
		task.Labels = taskPrompt.Labels

		// Store context and constraints in metadata (GORM serializer handles JSON conversion)
		task.Metadata = map[string]interface{}{
			"context":        taskPrompt.Context,
			"constraints":    taskPrompt.Constraints,
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
