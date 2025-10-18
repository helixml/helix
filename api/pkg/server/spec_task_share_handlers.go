package server

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/russross/blackfriday/v2"
)

// DesignDocsShareTokenClaims for JWT-based sharing
type DesignDocsShareTokenClaims struct {
	jwt.RegisteredClaims
	TaskID string `json:"task_id"`
	UserID string `json:"user_id"`
}

// DesignDocsShareLinkResponse contains the shareable URL
type DesignDocsShareLinkResponse struct {
	ShareURL  string    `json:"share_url"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// @Summary Generate shareable design docs link
// @Description Generate a token-based shareable link for viewing design documents on any device
// @Tags SpecTasks
// @Produce json
// @Param id path string true "SpecTask ID"
// @Success 200 {object} DesignDocsShareLinkResponse
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/{id}/design-docs/share [post]
func (apiServer *HelixAPIServer) generateDesignDocsShareLink(_ http.ResponseWriter, req *http.Request) (*DesignDocsShareLinkResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	taskID := vars["id"]
	user := getRequestUser(req)

	// Get task
	task, err := apiServer.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		return nil, system.NewHTTPError404("task not found")
	}

	// Verify access
	if task.CreatedBy != user.ID && !isAdmin(user) {
		return nil, system.NewHTTPError403("access denied")
	}

	// Generate JWT token
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // Valid for 7 days
	claims := DesignDocsShareTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "helix",
			Subject:   taskID,
		},
		TaskID: taskID,
		UserID: user.ID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(apiServer.Cfg.WebServer.RunnerToken))
	if err != nil {
		log.Error().Err(err).Msg("Failed to sign share token")
		return nil, system.NewHTTPError500("failed to generate share token")
	}

	// Build shareable URL
	baseURL := apiServer.Cfg.WebServer.URL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	shareURL := fmt.Sprintf("%s/spec-tasks/%s/view?token=%s", baseURL, taskID, tokenString)

	log.Info().
		Str("task_id", taskID).
		Str("user_id", user.ID).
		Msg("Generated shareable design docs link")

	return &DesignDocsShareLinkResponse{
		ShareURL:  shareURL,
		Token:     tokenString,
		ExpiresAt: expiresAt,
	}, nil
}

// viewDesignDocsPublic renders design documents as mobile-optimized HTML (no login required)
func (apiServer *HelixAPIServer) viewDesignDocsPublic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]
	tokenString := r.URL.Query().Get("token")

	if tokenString == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}

	// Validate JWT token
	claims := &DesignDocsShareTokenClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(apiServer.Cfg.WebServer.RunnerToken), nil
	})

	if err != nil || !token.Valid {
		log.Warn().Err(err).Str("task_id", taskID).Msg("Invalid share token")
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	// Verify task ID matches token
	if claims.TaskID != taskID {
		http.Error(w, "token does not match task", http.StatusUnauthorized)
		return
	}

	// Get task (no user auth needed, token validates access)
	task, err := apiServer.Store.GetSpecTask(r.Context(), taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	// Verify task has specs generated
	if task.Status == types.TaskStatusBacklog || task.Status == types.TaskStatusSpecGeneration {
		http.Error(w, "specifications not yet generated", http.StatusNotFound)
		return
	}

	// Convert markdown to HTML
	requirementsHTML := template.HTML(blackfriday.Run([]byte(task.RequirementsSpec)))
	designHTML := template.HTML(blackfriday.Run([]byte(task.TechnicalDesign)))
	implementationHTML := template.HTML(blackfriday.Run([]byte(task.ImplementationPlan)))

	// Render HTML template
	data := struct {
		TaskID             string
		TaskName           string
		Status             string
		OriginalPrompt     string
		RequirementsHTML   template.HTML
		TechnicalDesignHTML template.HTML
		ImplementationPlanHTML template.HTML
		UpdatedAt          string
	}{
		TaskID:             task.ID,
		TaskName:           task.Name,
		Status:             task.Status,
		OriginalPrompt:     task.OriginalPrompt,
		RequirementsHTML:   requirementsHTML,
		TechnicalDesignHTML: designHTML,
		ImplementationPlanHTML: implementationHTML,
		UpdatedAt:          task.UpdatedAt.Format("2006-01-02 15:04:05"),
	}

	tmpl, err := template.New("design_docs").Parse(designDocsViewerTemplate)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse template")
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to render template")
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("task_id", taskID).
		Str("user_id", claims.UserID).
		Msg("Rendered public design docs view")
}

// Mobile-optimized HTML template for design docs
const designDocsViewerTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <title>{{.TaskName}} - Design Documents</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f7fa;
            padding: 0;
            margin: 0;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
            background: white;
            min-height: 100vh;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .header h1 {
            font-size: 24px;
            font-weight: 600;
            margin-bottom: 8px;
        }
        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
            background: rgba(255,255,255,0.2);
            color: white;
            text-transform: uppercase;
        }
        .content {
            padding: 20px;
        }
        .section {
            margin-bottom: 40px;
            animation: fadeIn 0.5s ease-in;
        }
        .section h2 {
            color: #667eea;
            font-size: 20px;
            font-weight: 600;
            margin-bottom: 16px;
            padding-bottom: 8px;
            border-bottom: 2px solid #e5e7eb;
        }
        .section h3 {
            color: #4b5563;
            font-size: 18px;
            font-weight: 600;
            margin-top: 24px;
            margin-bottom: 12px;
        }
        .section p {
            margin-bottom: 16px;
            color: #4b5563;
        }
        .section ul, .section ol {
            margin-left: 24px;
            margin-bottom: 16px;
            color: #4b5563;
        }
        .section li {
            margin-bottom: 8px;
        }
        code {
            background: #f3f4f6;
            padding: 2px 6px;
            border-radius: 4px;
            font-family: 'Monaco', 'Courier New', monospace;
            font-size: 14px;
            color: #e53e3e;
        }
        pre {
            background: #1e293b;
            color: #e2e8f0;
            padding: 16px;
            border-radius: 8px;
            overflow-x: auto;
            margin-bottom: 16px;
            font-family: 'Monaco', 'Courier New', monospace;
            font-size: 14px;
            line-height: 1.5;
        }
        pre code {
            background: transparent;
            padding: 0;
            color: #e2e8f0;
        }
        .footer {
            padding: 20px;
            text-align: center;
            color: #9ca3af;
            font-size: 14px;
            border-top: 1px solid #e5e7eb;
        }
        .prompt-box {
            background: #fef3c7;
            border-left: 4px solid #f59e0b;
            padding: 16px;
            margin-bottom: 24px;
            border-radius: 4px;
        }
        .prompt-box strong {
            color: #92400e;
        }
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }
        @media (max-width: 768px) {
            .header { padding: 16px; }
            .header h1 { font-size: 20px; }
            .content { padding: 16px; }
            .section h2 { font-size: 18px; }
            .section h3 { font-size: 16px; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>{{.TaskName}}</h1>
            <span class="badge">{{.Status}}</span>
        </div>

        <div class="content">
            <div class="prompt-box">
                <strong>Original Request:</strong> {{.OriginalPrompt}}
            </div>

            <div class="section">
                <h2>📋 Requirements Specification</h2>
                {{.RequirementsHTML}}
            </div>

            <div class="section">
                <h2>🏗️ Technical Design</h2>
                {{.TechnicalDesignHTML}}
            </div>

            <div class="section">
                <h2>✅ Implementation Plan</h2>
                {{.ImplementationPlanHTML}}
            </div>
        </div>

        <div class="footer">
            Generated by Helix AI<br>
            Last updated: {{.UpdatedAt}}<br>
            Task ID: {{.TaskID}}
        </div>
    </div>
</body>
</html>
`
