package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	modelpkg "github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pricing"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getUsage godoc
// @Summary Get daily usage
// @Description Get daily usage
// @Accept json
// @Produce json
// @Tags    usage
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Param   org_id query string false "Organization ID"
// @Param   project_id query string false "Project ID"
// @Param   spec_task_id query string false "Spec Task ID"
// @Param   aggregation_level query string false "Aggregation level"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)

	from := time.Now().Add(-time.Hour * 24 * 7) // Last 7 days
	to := time.Now()
	orgID := r.URL.Query().Get("org_id")
	projectID := r.URL.Query().Get("project_id")
	specTaskID := r.URL.Query().Get("spec_task_id")

	aggregationLevel := store.AggregationLevelDaily
	if r.URL.Query().Get("aggregation_level") == "hourly" {
		aggregationLevel = store.AggregationLevelHourly
	}

	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	if orgID != "" {
		// Lookup org
		org, err := s.lookupOrg(r.Context(), orgID)
		if err != nil {
			return nil, system.NewHTTPError404(err.Error())
		}

		orgID = org.ID

		_, err = s.authorizeOrgMember(r.Context(), user, orgID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
	}

	var err error

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	}

	usageUserID := user.ID
	if orgID != "" {
		usageUserID = ""
	}

	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		AggregationLevel: aggregationLevel,
		UserID:           usageUserID,
		OrganizationID:   orgID,
		ProjectID:        projectID,
		SpecTaskID:       specTaskID,
		From:             from,
		To:               to,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	if orgID != "" && projectID == "" && specTaskID == "" {
		sandboxMetrics, err := s.Store.GetSandboxUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
			AggregationLevel: aggregationLevel,
			OrganizationID:   orgID,
			From:             from,
			To:               to,
		})
		if err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
		mergeSandboxUsageCosts(metrics, sandboxMetrics)
	}

	return metrics, nil
}

// getOrgUsageSummary godoc
// @Summary Get organization usage summary
// @Description Get organization usage summary with breakdowns by user, project, app, session, task/model, and model/provider
// @Accept json
// @Produce json
// @Tags    usage
// @Param   org_id query string true "Organization ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Param   user_id query string false "User ID"
// @Param   project_id query string false "Project ID"
// @Param   task_id query string false "Task ID"
// @Param   app_id query string false "App ID"
// @Param   session_id query string false "Session ID"
// @Param   provider query string false "Provider"
// @Param   model query string false "Model"
// @Param   user_search query string false "User search"
// @Param   user_limit query int false "User page size"
// @Param   user_offset query int false "User page offset"
// @Param   project_limit query int false "Project page size"
// @Param   project_offset query int false "Project page offset"
// @Param   task_limit query int false "Task page size"
// @Param   task_offset query int false "Task page offset"
// @Param   session_limit query int false "Session page size"
// @Param   session_offset query int false "Session page offset"
// @Success 200 {object} types.OrgUsageSummaryResponse
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/usage/org-summary [get]
// @Security BearerAuth
func (s *HelixAPIServer) getOrgUsageSummary(_ http.ResponseWriter, r *http.Request) (*types.OrgUsageSummaryResponse, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		return nil, system.NewHTTPError400("org_id is required")
	}
	org, err := s.lookupOrg(r.Context(), orgID)
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}
	orgID = org.ID

	_, err = s.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	from := time.Now().Add(-time.Hour * 24 * 7)
	to := time.Now()
	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	}
	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	}

	userLimit := 10
	if rawLimit := r.URL.Query().Get("user_limit"); rawLimit != "" {
		userLimit, err = strconv.Atoi(rawLimit)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse user_limit: %s", err))
		}
	}
	userOffset := 0
	if rawOffset := r.URL.Query().Get("user_offset"); rawOffset != "" {
		userOffset, err = strconv.Atoi(rawOffset)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse user_offset: %s", err))
		}
	}
	projectLimit := 10
	if rawLimit := r.URL.Query().Get("project_limit"); rawLimit != "" {
		projectLimit, err = strconv.Atoi(rawLimit)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse project_limit: %s", err))
		}
	}
	projectOffset := 0
	if rawOffset := r.URL.Query().Get("project_offset"); rawOffset != "" {
		projectOffset, err = strconv.Atoi(rawOffset)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse project_offset: %s", err))
		}
	}
	taskLimit := 10
	if rawLimit := r.URL.Query().Get("task_limit"); rawLimit != "" {
		taskLimit, err = strconv.Atoi(rawLimit)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse task_limit: %s", err))
		}
	}
	taskOffset := 0
	if rawOffset := r.URL.Query().Get("task_offset"); rawOffset != "" {
		taskOffset, err = strconv.Atoi(rawOffset)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse task_offset: %s", err))
		}
	}
	sessionLimit := 10
	if rawLimit := r.URL.Query().Get("session_limit"); rawLimit != "" {
		sessionLimit, err = strconv.Atoi(rawLimit)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse session_limit: %s", err))
		}
	}
	sessionOffset := 0
	if rawOffset := r.URL.Query().Get("session_offset"); rawOffset != "" {
		sessionOffset, err = strconv.Atoi(rawOffset)
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse session_offset: %s", err))
		}
	}

	summary, err := s.Store.GetOrgUsageSummary(r.Context(), &store.GetOrgUsageSummaryQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
		UserID:         r.URL.Query().Get("user_id"),
		ProjectID:      r.URL.Query().Get("project_id"),
		TaskID:         r.URL.Query().Get("task_id"),
		AppID:          r.URL.Query().Get("app_id"),
		SessionID:      r.URL.Query().Get("session_id"),
		Provider:       r.URL.Query().Get("provider"),
		Model:          r.URL.Query().Get("model"),
		UserSearch:     r.URL.Query().Get("user_search"),
		UserLimit:      userLimit,
		UserOffset:     userOffset,
		ProjectLimit:   projectLimit,
		ProjectOffset:  projectOffset,
		TaskLimit:      taskLimit,
		TaskOffset:     taskOffset,
		SessionLimit:   sessionLimit,
		SessionOffset:  sessionOffset,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	s.enrichOrgUsageCosts(r.Context(), summary)

	// Sandbox runtime is the other half of the bill. It comes from the wallet
	// ledger rather than usage_metrics, so it is a separate query rather than
	// part of GetOrgUsageSummary. A failure here must not blank the token
	// numbers the page is mainly about.
	compute, err := s.Store.GetOrgComputeUsage(r.Context(), &store.GetOrgComputeUsageQuery{
		OrganizationID: orgID,
		ProjectID:      r.URL.Query().Get("project_id"),
		TaskID:         r.URL.Query().Get("task_id"),
		From:           from,
		To:             to,
	})
	if err != nil {
		log.Warn().Err(err).Str("org_id", orgID).Msg("failed to load organization compute usage")
	} else {
		summary.Compute = compute
	}

	return summary, nil
}

func (s *HelixAPIServer) enrichOrgUsageCosts(ctx context.Context, summary *types.OrgUsageSummaryResponse) {
	if summary == nil {
		return
	}

	type providerAggregate struct {
		row    types.UsageBreakdownRow
		byDate map[time.Time]*types.AggregatedUsageMetric
	}
	providers := make(map[string]*providerAggregate)
	modelCosts := make(map[string]float64)
	modelProviders := make(map[string]string)
	modelInfo := make(map[string]*types.ModelInfo)
	missingModelInfo := make(map[string]bool)
	endpointProviders := make(map[string]string)
	checkedEndpoints := make(map[string]bool)

	for _, row := range summary.CostBreakdown {
		key := row.Provider + ":" + row.Model
		info, known := modelInfo[key]
		if !known && !missingModelInfo[key] && s.modelInfoProvider != nil {
			resolved, err := s.modelInfoProvider.GetModelInfo(ctx, &modelpkg.ModelInfoRequest{
				Provider: row.Provider,
				Model:    row.Model,
			})
			if err != nil {
				missingModelInfo[key] = true
				log.Debug().Err(err).Str("provider", row.Provider).Str("model", row.Model).
					Msg("model cannot be priced for organization usage")
			} else {
				info = resolved
				modelInfo[key] = resolved
			}
		}

		estimatedCost := row.TotalCost
		if info != nil {
			pricingPromptTokens := row.PromptTokens
			if row.Source == types.UsageMetricSourceACP {
				pricingPromptTokens += row.CacheReadTokens + row.CacheWriteTokens
			}
			usage := pricing.TokenUsage{
				PromptTokens:     int64(pricingPromptTokens),
				CompletionTokens: int64(row.CompletionTokens),
				CacheReadTokens:  int64(row.CacheReadTokens),
				CacheWriteTokens: int64(row.CacheWriteTokens),
			}
			priced, err := pricing.CalculateTokenPrice(info, usage)
			if err == nil {
				if estimatedCost == 0 {
					estimatedCost = priced.Total()
				}
				fullRate, fullRateErr := pricing.CalculateTokenPrice(info, pricing.TokenUsage{
					PromptTokens:     int64(pricingPromptTokens),
					CompletionTokens: int64(row.CompletionTokens),
				})
				if fullRateErr == nil {
					summary.CacheSavings += max(fullRate.Total()-priced.Total(), 0)
				}
			}
		}

		summary.RawTokenCost += estimatedCost
		if row.Source == types.UsageMetricSourceACP {
			summary.SubscriptionSavings += estimatedCost
		}
		modelCosts[key] += estimatedCost
		displayProvider := ""
		if strings.HasPrefix(row.Provider, system.ProviderEndpointPrefix) && s.Store != nil {
			if !checkedEndpoints[row.Provider] {
				checkedEndpoints[row.Provider] = true
				endpoint, endpointErr := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: row.Provider})
				if endpointErr != nil {
					log.Debug().Err(endpointErr).Str("provider", row.Provider).
						Msg("failed to resolve provider endpoint for organization usage")
				} else if endpoint != nil && endpoint.EndpointType == types.ProviderEndpointTypeGlobal && endpoint.OwnerType == types.OwnerTypeSystem {
					// Database-backed system globals are Helix-managed inference routes
					// (for example vLLM or SGLang), not the catalog provider for the model ID.
					endpointProviders[row.Provider] = "helix/" + endpoint.Name
				}
			}
			displayProvider = endpointProviders[row.Provider]
		}
		if displayProvider == "" && info != nil && info.ProviderSlug != "" {
			displayProvider = info.ProviderSlug
		}
		if displayProvider == "" {
			displayProvider = row.Provider
		}
		modelProviders[key] = displayProvider

		providerKey := displayProvider
		if providerKey == "" {
			providerKey = "unknown"
		}
		aggregate := providers[providerKey]
		if aggregate == nil {
			aggregate = &providerAggregate{
				row: types.UsageBreakdownRow{
					ID:       providerKey,
					Name:     usageProviderName(providerKey),
					Provider: providerKey,
				},
				byDate: make(map[time.Time]*types.AggregatedUsageMetric),
			}
			providers[providerKey] = aggregate
		}
		aggregate.row.PromptTokens += row.PromptTokens
		aggregate.row.CompletionTokens += row.CompletionTokens
		aggregate.row.TotalTokens += row.TotalTokens
		aggregate.row.CacheReadTokens += row.CacheReadTokens
		aggregate.row.CacheWriteTokens += row.CacheWriteTokens
		aggregate.row.TotalCost += estimatedCost

		metric := aggregate.byDate[row.Date]
		if metric == nil {
			metric = &types.AggregatedUsageMetric{Date: row.Date}
			aggregate.byDate[row.Date] = metric
		}
		metric.PromptTokens += row.PromptTokens
		metric.CompletionTokens += row.CompletionTokens
		metric.TotalTokens += row.TotalTokens
		metric.CacheReadTokens += row.CacheReadTokens
		metric.CacheWriteTokens += row.CacheWriteTokens
		metric.TotalCost += estimatedCost
		// Accumulate total duration here and average it once all rows for the
		// provider-day are in — averaging per row would weight a day with one
		// slow call the same as a day with a thousand fast ones.
		metric.LatencyMs += row.DurationMs
		metric.TotalRequests += row.TotalRequests
	}

	for index := range summary.Models {
		row := &summary.Models[index]
		key := row.Provider + ":" + row.Model
		row.TotalCost = modelCosts[key]
		row.Provider = modelProviders[key]
	}
	for index := range summary.ExportModels {
		row := &summary.ExportModels[index]
		key := row.Provider + ":" + row.Model
		row.TotalCost = modelCosts[key]
		row.Provider = modelProviders[key]
	}

	providerKeys := make([]string, 0, len(providers))
	for key := range providers {
		providerKeys = append(providerKeys, key)
	}
	sort.Slice(providerKeys, func(i, j int) bool {
		return providers[providerKeys[i]].row.TotalCost > providers[providerKeys[j]].row.TotalCost
	})
	for _, key := range providerKeys {
		aggregate := providers[key]
		summary.Providers = append(summary.Providers, aggregate.row)
		series := types.UsageProviderTimeSeries{
			Provider: key,
			Name:     aggregate.row.Name,
		}
		for _, totalMetric := range summary.Metrics {
			metric := aggregate.byDate[totalMetric.Date]
			if metric == nil {
				metric = &types.AggregatedUsageMetric{Date: totalMetric.Date}
			}
			point := *metric
			// LatencyMs accumulated total duration above; publish the mean per
			// request, which is what the latency chart plots.
			if point.TotalRequests > 0 {
				point.LatencyMs = point.LatencyMs / float64(point.TotalRequests)
			} else {
				point.LatencyMs = 0
			}
			series.Metrics = append(series.Metrics, point)
		}
		summary.ProviderTimeSeries = append(summary.ProviderTimeSeries, series)
	}
	if summary.HelixCredits < 0 {
		summary.HelixCredits = 0
	}
}

func usageProviderName(provider string) string {
	switch strings.ToLower(provider) {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "google", "google-ai", "gemini":
		return "Google"
	case "xai":
		return "xAI"
	case "azure":
		return "Azure OpenAI"
	case "amazon-bedrock", "aws":
		return "Amazon Bedrock"
	case "deepseek":
		return "DeepSeek"
	case "unknown":
		return "Unknown"
	default:
		return provider
	}
}

func mergeSandboxUsageCosts(metrics []*types.AggregatedUsageMetric, sandboxMetrics []*types.AggregatedUsageMetric) {
	byDate := make(map[time.Time]*types.AggregatedUsageMetric, len(metrics))
	for _, metric := range metrics {
		if metric == nil {
			continue
		}
		byDate[metric.Date] = metric
	}
	for _, sandboxMetric := range sandboxMetrics {
		if sandboxMetric == nil || sandboxMetric.SandboxCost == 0 {
			continue
		}
		metric, ok := byDate[sandboxMetric.Date]
		if !ok {
			continue
		}
		metric.SandboxCost += sandboxMetric.SandboxCost
		metric.TotalCost += sandboxMetric.SandboxCost
		metric.TotalRequests += sandboxMetric.TotalRequests
	}
}

// getSpecTaskUsage godoc
// @Summary Get spec task usage
// @Description Get spec task usage
// @Accept json
// @Produce json
// @Tags    spec-tasks
// @Param   taskId path string true "Spec Task ID"
// @Param   from query string false "Start date"
// @Param   to query string false "End date"
// @Param   aggregation_level query string false "Aggregation level"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{taskId}/usage [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecTaskUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)

	specTaskID := getID(r)

	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	specTask, err := s.Store.GetSpecTask(r.Context(), specTaskID)
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}

	err = s.authorizeUserToProjectByID(r.Context(), user, specTask.ProjectID, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	var from time.Time
	var to time.Time

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	} else {
		switch {
		case specTask.PlanningStartedAt != nil:
			from = *specTask.PlanningStartedAt
		case specTask.StartedAt != nil:
			from = *specTask.StartedAt
		default:
			from = specTask.CreatedAt
		}

		minFrom := time.Now().Add(-30 * time.Minute)
		if from.After(minFrom) {
			from = minFrom
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	} else {
		// Default to now
		to = time.Now()
	}

	// Auto-select aggregation level based on time range to keep data points between 20-50
	// This prevents huge payloads for long-running tasks (e.g., 7-day task at 5min = 2016 points)
	aggregationLevel := selectAggregationLevel(from, to)

	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		AggregationLevel: aggregationLevel,
		UserID:           user.ID,
		ProjectID:        specTask.ProjectID,
		SpecTaskID:       specTaskID,
		From:             from,
		To:               to,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return metrics, nil
}

// selectAggregationLevel picks the finest granularity that keeps data points between minPoints and maxPoints
func selectAggregationLevel(from, to time.Time) store.AggregationLevel {
	const minPoints = 20
	const maxPoints = 50

	duration := to.Sub(from)
	pointsAt5Min := int(duration.Minutes() / 5)
	pointsAtHourly := int(duration.Hours())

	// Start with finest granularity and coarsen if too many points
	if pointsAt5Min <= maxPoints {
		return store.AggregationLevel5Min
	}
	if pointsAtHourly >= minPoints && pointsAtHourly <= maxPoints {
		return store.AggregationLevelHourly
	}
	if pointsAtHourly > maxPoints {
		return store.AggregationLevelDaily
	}
	// pointsAtHourly < minPoints but pointsAt5Min > maxPoints
	// Use hourly anyway (slightly sparse is better than 1000+ points)
	return store.AggregationLevelHourly
}

// BatchTaskUsageResponse contains usage metrics for all tasks in a project
// BatchTaskUsageMetric is a slim version of AggregatedUsageMetric for the batch
// endpoint — the Kanban sparkline charts only need date + total_tokens.
type BatchTaskUsageMetric struct {
	Date        time.Time `json:"date"`
	TotalTokens int       `json:"total_tokens"`
}

type BatchTaskUsageResponse struct {
	ProjectID string                            `json:"project_id"`
	Tasks     map[string][]BatchTaskUsageMetric `json:"tasks"` // keyed by task_id
}

// getBatchTaskUsage godoc
// @Summary Get usage for all tasks in a project
// @Description Get usage metrics for all spec-driven tasks in a project in a single request. This is more efficient than calling the individual usage endpoint for each task.
// @Tags    spec-driven-tasks
// @Produce json
// @Param   id path string true "Project ID"
// @Success 200 {object} BatchTaskUsageResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/projects/{id}/tasks-usage [get]
func (s *HelixAPIServer) getBatchTaskUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["id"]

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to access the project
	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionGet); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Get all non-archived tasks for this project
	tasks, err := s.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:       projectID,
		IncludeArchived: false,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to list tasks for batch usage")
		http.Error(w, "failed to list tasks", http.StatusInternalServerError)
		return
	}

	// Build response with usage for each task.
	// Use daily aggregation and return only date + total_tokens — the Kanban
	// sparkline charts don't need finer granularity or other metric fields.
	response := BatchTaskUsageResponse{
		ProjectID: projectID,
		Tasks:     make(map[string][]BatchTaskUsageMetric, len(tasks)),
	}

	// Fetch usage in parallel with concurrency limit
	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent DB queries

	for _, task := range tasks {
		wg.Add(1)
		go func(t *types.SpecTask) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Calculate time range for this task
			var from time.Time
			switch {
			case t.PlanningStartedAt != nil:
				from = *t.PlanningStartedAt
			case t.StartedAt != nil:
				from = *t.StartedAt
			default:
				from = t.CreatedAt
			}

			// Ensure minimum 30 min range
			minFrom := time.Now().Add(-30 * time.Minute)
			if from.After(minFrom) {
				from = minFrom
			}

			to := time.Now()

			metrics, err := s.Store.GetAggregatedUsageMetrics(ctx, &store.GetAggregatedUsageMetricsQuery{
				AggregationLevel: store.AggregationLevelHourly,
				UserID:           user.ID,
				ProjectID:        projectID,
				SpecTaskID:       t.ID,
				From:             from,
				To:               to,
			})
			if err != nil {
				log.Error().Err(err).Str("task_id", t.ID).Msg("Failed to get usage for task")
				return
			}

			// Only include the last 7 days of daily data — sparkline charts
			// don't need months of history. This also naturally limits the
			// payload since most tasks have zero usage outside their active period.
			sevenDaysAgo := time.Now().AddDate(0, 0, -7)
			slim := make([]BatchTaskUsageMetric, 0, 7)
			for _, m := range metrics {
				if m.Date.Before(sevenDaysAgo) {
					continue
				}
				slim = append(slim, BatchTaskUsageMetric{Date: m.Date, TotalTokens: m.TotalTokens})
			}

			// Only include tasks that have non-zero usage in the window
			hasUsage := false
			for _, m := range slim {
				if m.TotalTokens > 0 {
					hasUsage = true
					break
				}
			}
			if hasUsage {
				mu.Lock()
				response.Tasks[t.ID] = slim
				mu.Unlock()
			}
		}(task)
	}

	wg.Wait()

	writeResponseWithETag(w, r, response)
}
