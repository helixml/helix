package server

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/russross/blackfriday/v2"
)

// viewDesignDocsPublic renders design documents as mobile-optimized HTML (no login required if public)
func (apiServer *HelixAPIServer) viewDesignDocsPublic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["id"]

	// Get task
	task, err := apiServer.Store.GetSpecTask(r.Context(), taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	// Check if task is public
	if !task.PublicDesignDocs {
		// Render private task page
		apiServer.renderPrivateTaskPage(w, task)
		return
	}

	// Verify task has specs generated
	if task.Status == types.TaskStatusBacklog || task.Status == types.TaskStatusSpecGeneration {
		http.Error(w, "specifications not yet generated", http.StatusNotFound)
		return
	}

	// Render the public design docs
	apiServer.renderDesignDocsPage(w, task)
}

// renderPrivateTaskPage shows a user-friendly message for non-public tasks
func (apiServer *HelixAPIServer) renderPrivateTaskPage(w http.ResponseWriter, task *types.SpecTask) {
	data := struct {
		TaskID   string
		TaskName string
		LoginURL string
	}{
		TaskID:   task.ID,
		TaskName: task.Name,
		LoginURL: apiServer.Cfg.WebServer.URL + "/login",
	}

	tmpl, err := template.New("private_task").Parse(privateTaskTemplate)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse private task template")
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to render private task template")
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
}

// renderDesignDocsPage renders the design documents for a public task
func (apiServer *HelixAPIServer) renderDesignDocsPage(w http.ResponseWriter, task *types.SpecTask) {
	// Convert markdown to HTML
	requirementsHTML := template.HTML(blackfriday.Run([]byte(task.RequirementsSpec)))
	designHTML := template.HTML(blackfriday.Run([]byte(task.TechnicalDesign)))
	implementationHTML := template.HTML(blackfriday.Run([]byte(task.ImplementationPlan)))

	data := struct {
		TaskID                 string
		TaskName               string
		Status                 string
		OriginalPrompt         string
		RequirementsHTML       template.HTML
		TechnicalDesignHTML    template.HTML
		ImplementationPlanHTML template.HTML
		UpdatedAt              string
	}{
		TaskID:                 task.ID,
		TaskName:               task.Name,
		Status:                 task.Status.String(),
		OriginalPrompt:         task.OriginalPrompt,
		RequirementsHTML:       requirementsHTML,
		TechnicalDesignHTML:    designHTML,
		ImplementationPlanHTML: implementationHTML,
		UpdatedAt:              task.UpdatedAt.Format("2006-01-02 15:04:05"),
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
		Str("task_id", task.ID).
		Msg("Rendered public design docs view")
}

// Private task template - shown when task is not public
const privateTaskTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <title>Private Task - Helix</title>
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
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            max-width: 500px;
            margin: 20px;
            background: white;
            border-radius: 12px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.1);
            padding: 40px;
            text-align: center;
        }
        .icon {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            color: #4b5563;
            font-size: 24px;
            font-weight: 600;
            margin-bottom: 16px;
        }
        p {
            color: #6b7280;
            margin-bottom: 24px;
        }
        .task-name {
            background: #f3f4f6;
            padding: 8px 16px;
            border-radius: 8px;
            font-family: 'Monaco', 'Courier New', monospace;
            font-size: 14px;
            color: #374151;
            margin-bottom: 24px;
            display: inline-block;
        }
        .login-btn {
            display: inline-block;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 12px 32px;
            border-radius: 8px;
            text-decoration: none;
            font-weight: 600;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .login-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }
        .footer {
            margin-top: 32px;
            color: #9ca3af;
            font-size: 12px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">🔒</div>
        <h1>This spec task is private</h1>
        <p>The owner has not made this task's design documents publicly viewable.</p>
        <div class="task-name">{{.TaskName}}</div>
        <p>If you have access to this project, you can log in to view the design documents.</p>
        <a href="{{.LoginURL}}" class="login-btn">Log in to Helix</a>
        <div class="footer">
            Task ID: {{.TaskID}}
        </div>
    </div>
</body>
</html>
`

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
