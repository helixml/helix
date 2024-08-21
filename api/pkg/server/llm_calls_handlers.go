package server

import (
	"net/http"
	"strconv"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listLLMCalls godoc
// @Summary List LLM calls
// @Description List LLM calls with pagination
// @Tags    llm_calls
// @Produce json
// @Param   page     query    int     false  "Page number"
// @Param   pageSize query    int     false  "Page size"
// @Success 200 {object} types.PaginatedLLMCalls
// @Router /api/v1/llm_calls [get]
// @Security BearerAuth
func (s *HelixAPIServer) listLLMCalls(w http.ResponseWriter, r *http.Request) (*types.PaginatedLLMCalls, *system.HTTPError) {
	// Parse query parameters
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if err != nil || pageSize < 1 {
		pageSize = 10 // Default page size
	}

	// Call the ListLLMCalls function from the store
	calls, totalCount, err := s.Store.ListLLMCalls(r.Context(), page, pageSize)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Calculate total pages
	totalPages := (int(totalCount) + pageSize - 1) / pageSize

	// Prepare the response
	response := &types.PaginatedLLMCalls{
		Calls:      calls,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	return response, nil
}
