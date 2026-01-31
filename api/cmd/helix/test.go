// Package helix provides the Helix CLI commands.
//
// Multi-turn conversation tests can be defined in helix.yaml as follows:
//
// ```yaml
// assistants:
//   - name: MyAssistant
//     tests:
//   - name: Multi-turn Test
//     steps:
//   - is_multi_turn: true
//     turns:
//   - user_prompt: "Hello, how are you?"
//     expected_assistant_response: "I'm doing well, thank you for asking. How can I help you today?"
//   - user_prompt: "Tell me about yourself"
//     expected_assistant_response: "I am an AI assistant designed to help with various tasks."
//   - user_prompt: "What can you help me with?"
//     expected_assistant_response: "I can help with answering questions, providing information, and assisting with tasks."
//
// ```
//
// Each turn in a multi-turn test will be evaluated separately, and the overall test will pass only if all turns pass.
// The test results will show details for each turn in the conversation.
package helix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"html/template"

	"github.com/helixml/helix/api/pkg/apps"
	cliutil "github.com/helixml/helix/api/pkg/cli/util"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

type ChatRequest struct {
	Model     string    `json:"model"`
	SessionID string    `json:"session_id"`
	System    string    `json:"system"`
	Messages  []Message `json:"messages"`
	AppID     string    `json:"app_id"`
}

type Message struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

type Content struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}

type TurnResult struct {
	TurnNumber        int           `json:"turn_number"`
	UserPrompt        string        `json:"user_prompt"`
	AssistantResponse string        `json:"assistant_response"`
	ExpectedResponse  string        `json:"expected_response"`
	Result            string        `json:"result"`
	Reason            string        `json:"reason"`
	InferenceTime     time.Duration `json:"inference_time"`
	EvaluationTime    time.Duration `json:"evaluation_time"`
}

type RAGMetrics struct {
	FilesUploaded     int                      `json:"files_uploaded"`
	TotalUploadTime   time.Duration            `json:"total_upload_time"`
	TotalIndexingTime time.Duration            `json:"total_indexing_time"`
	KnowledgeSources  []KnowledgeSourceMetrics `json:"knowledge_sources"`
}

type KnowledgeSourceMetrics struct {
	Name         string        `json:"name"`
	FileCount    int           `json:"file_count"`
	UploadTime   time.Duration `json:"upload_time"`
	IndexingTime time.Duration `json:"indexing_time"`
	LocalDir     string        `json:"local_dir"`
	RemotePath   string        `json:"remote_path"`
}

type TestResult struct {
	TestName        string        `json:"test_name"`
	Prompt          string        `json:"prompt"`
	Response        string        `json:"response"`
	Expected        string        `json:"expected"`
	Result          string        `json:"result"`
	Reason          string        `json:"reason"`
	SessionID       string        `json:"session_id"`
	Model           string        `json:"model"`
	EvaluationModel string        `json:"evaluation_model"`
	InferenceTime   time.Duration `json:"inference_time"`
	EvaluationTime  time.Duration `json:"evaluation_time"`
	HelixURL        string        `json:"helix_url"`
	IsMultiTurn     bool          `json:"is_multi_turn"`
	TurnNumber      int           `json:"turn_number,omitempty"`
	TotalTurns      int           `json:"total_turns,omitempty"`
	TurnResults     []TurnResult  `json:"turn_results,omitempty"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type ModelResponse struct {
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"data"`
}

type TestResults struct {
	Tests             []TestResult  `json:"tests"`
	TotalTime         time.Duration `json:"total_time"`
	RAGMetrics        *RAGMetrics   `json:"rag_metrics,omitempty"`
	HelixYaml         string        `json:"helix_yaml"`
	TestID            string        `json:"test_id"`
	NamespacedAppName string        `json:"namespaced_app_name"`
}

// Template for HTML report
var htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Helix Test Results</title>
    <style>
        body, html {
            font-family: Arial, sans-serif;
            margin: 0;
            padding: 0;
            height: 100%;
            overflow: hidden;
        }
        .main-container {
            display: flex;
            flex-direction: column;
            height: 100vh;
        }
        .header {
            padding: 10px 20px;
            background-color: #f8f8f8;
            display: flex;
            align-items: center;
            justify-content: space-between;
            flex-wrap: wrap;
        }
        .header h1 {
            margin: 0;
            font-size: 1.2em;
        }
        .header-info {
            display: flex;
            align-items: center;
            gap: 20px;
            padding-left: 10px;
        }
        .header-info p {
            margin: 0;
            font-size: 0.9em;
        }
        .header-controls {
            display: flex;
            align-items: center;
            margin-left: auto;
        }
        .header-controls select {
            margin-right: 10px;
        }
        .results-container {
            flex: 1;
            overflow-y: auto;
            padding: 0 20px;
        }
        table {
            border-collapse: collapse;
            width: 100%;
            margin-bottom: 20px;
        }
        th, td {
            border: 1px solid #ddd;
            padding: 8px;
            text-align: left;
        }
        th {
            background-color: #f2f2f2;
            position: sticky;
            top: 0;
            z-index: 10;
        }
        tr.pass { background-color: #e6ffe6; }
        tr.fail { background-color: #ffe6e6; }
        .section {
            margin-bottom: 30px;
        }
        .section-title {
            font-size: 1.2em;
            margin: 20px 0 10px;
            padding-bottom: 5px;
            border-bottom: 2px solid #f2f2f2;
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .metric-card {
            background: #f8f8f8;
            padding: 15px;
            border-radius: 5px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .metric-title {
            font-weight: bold;
            margin-bottom: 10px;
        }
        .metric-value {
            font-size: 1.2em;
            color: #2a6b9c;
        }
        #iframe-container {
            position: fixed;
            bottom: 0;
            left: 0;
            right: 0;
            height: 50vh;
            background: #333;
            z-index: 100;
            display: none;
            box-shadow: 0 -2px 10px rgba(0,0,0,0.2);
        }
        #iframe-container iframe {
            width: 100%;
            height: calc(100% - 10px);
            border: none;
            margin-top: 10px;
        }
        #close-iframe {
            position: absolute;
            top: 10px;
            right: 10px;
            cursor: pointer;
        }
        #resize-handle {
            width: 100%;
            height: 10px;
            background: #f0f0f0;
            cursor: ns-resize;
            border-top: 1px solid #ccc;
        }
        #view-helix-yaml {
            padding: 5px 10px;
            font-size: 0.9em;
        }
        .truncate {
            max-width: 400px;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            position: relative;
            cursor: pointer;
        }
        .tooltip {
            display: none;
            position: absolute;
            background-color: #f9f9f9;
            border: 1px solid #ddd;
            padding: 5px;
            z-index: 1000;
            max-width: 300px;
            word-wrap: break-word;
            box-shadow: 0 2px 5px rgba(0,0,0,0.2);
        }
        /* Tab styling */
        .tabs {
            display: flex;
            margin-bottom: 20px;
            border-bottom: 1px solid #ddd;
        }
        .tab {
            padding: 10px 20px;
            cursor: pointer;
            margin-right: 5px;
            border: 1px solid #ddd;
            border-bottom: none;
            border-radius: 5px 5px 0 0;
            background-color: #f9f9f9;
        }
        .tab.active {
            background-color: #fff;
            border-bottom: 1px solid #fff;
            margin-bottom: -1px;
            font-weight: bold;
        }
        .tab-content {
            display: none;
        }
        .tab-content.active {
            display: block;
        }
        /* Overlay for resizing */
        #resize-overlay {
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            z-index: 9999;
            display: none;
        }
    </style>
</head>
<body>
    <div class="main-container">
        <div class="header">
            <h1>Helix Test Results</h1>
            <div class="header-info">
                <p>Total Time: {{.TotalExecutionTime}}</p>
                <p>File: {{.LatestResultsFile}}</p>
            </div>
            <div class="header-controls">
                <form action="/" method="get" style="margin: 0;">
                    <select name="file" onchange="this.form.submit()" style="padding: 5px;">
                        {{range .AvailableResultFiles}}
                            <option value="{{.}}" {{if eq . $.LatestResultsFile}}selected{{end}}>{{.}}</option>
                        {{end}}
                    </select>
                </form>
                <button id="view-helix-yaml" onclick="viewHelixYaml()">View helix.yaml</button>
            </div>
        </div>
        <div class="results-container">
            <div class="tabs">
                <div class="tab active" data-tab="test-results">Test Results</div>
                {{if .RAGMetrics}}
                <div class="tab" data-tab="rag-benchmark">RAG Benchmark</div>
                {{end}}
            </div>
            
            <div id="test-results" class="tab-content active">
                <div class="section">
                    <table>
                        <thead>
                            <tr>
                                <th>Test Name</th>
                                <th>Result</th>
                                <th>Reason</th>
                                <th>Model</th>
                                <th>Inference Time</th>
                                <th>Evaluation Time</th>
                                <th>Session Link</th>
                                <th>Debug Link</th>
                            </tr>
                        </thead>
                        <tbody>
                            {{range .Tests}}
                            <tr class="{{if eq .Result "PASS"}}pass{{else}}fail{{end}}">
                                <td>{{.TestName}}</td>
                                <td>{{.Result}}</td>
                                <td class="truncate" data-full-text="{{.Reason}}">{{truncate .Reason 100}}</td>
                                <td>{{.Model}}</td>
                                <td>{{printf "%.2f" .InferenceTime.Seconds}}s</td>
                                <td>{{printf "%.2f" .EvaluationTime.Seconds}}s</td>
                                <td><a href="{{$.HelixURL}}/session/{{.SessionID}}" onclick="openLink('{{$.HelixURL}}/session/{{.SessionID}}'); return false;">Session</a></td>
                                <td><a href="{{$.HelixURL}}/dashboard?tab=llm_calls&filter_sessions={{.SessionID}}" onclick="openLink('{{$.HelixURL}}/dashboard?tab=llm_calls&filter_sessions={{.SessionID}}'); return false;">Debug</a></td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
            </div>
            
            {{if .RAGMetrics}}
            <div id="rag-benchmark" class="tab-content">
                <div class="section">
                    <div class="metrics-grid">
                        <div class="metric-card">
                            <div class="metric-title">Total Files Uploaded</div>
                            <div class="metric-value">{{.RAGMetrics.FilesUploaded}}</div>
                        </div>
                        <div class="metric-card">
                            <div class="metric-title">Total Upload Time</div>
                            <div class="metric-value">{{printf "%.2f" .RAGMetrics.TotalUploadTime.Seconds}}s</div>
                        </div>
                        <div class="metric-card">
                            <div class="metric-title">Total Indexing Time</div>
                            <div class="metric-value">{{printf "%.2f" .RAGMetrics.TotalIndexingTime.Seconds}}s</div>
                        </div>
                    </div>
                    {{if .RAGMetrics.KnowledgeSources}}
                    <table>
                        <thead>
                            <tr>
                                <th>Knowledge Source</th>
                                <th>Files</th>
                                <th>Upload Time</th>
                                <th>Indexing Time</th>
                                <th>Local Directory</th>
                                <th>Remote Path</th>
                            </tr>
                        </thead>
                        <tbody>
                            {{range .RAGMetrics.KnowledgeSources}}
                            <tr>
                                <td>{{.Name}}</td>
                                <td>{{.FileCount}}</td>
                                <td>{{printf "%.2f" .UploadTime.Seconds}}s</td>
                                <td>{{printf "%.2f" .IndexingTime.Seconds}}s</td>
                                <td>{{.LocalDir}}</td>
                                <td>{{.RemotePath}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>
    </div>
    <div id="iframe-container">
        <div id="resize-handle"></div>
        <div id="close-iframe" onclick="closeDashboard()" style="color: white;">Close</div>
        <iframe id="dashboard-iframe" src=""></iframe>
    </div>
    <div id="resize-overlay"></div>
    <div id="tooltip" class="tooltip"></div>
    <script>
        // Tab functionality
        document.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', () => {
                // Remove active class from all tabs and content
                document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
                document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                
                // Add active class to clicked tab
                tab.classList.add('active');
                
                // Show corresponding content
                const tabId = tab.getAttribute('data-tab');
                document.getElementById(tabId).classList.add('active');
            });
        });

        function openLink(url) {
            if (window.location.protocol === 'file:') {
                window.open(url, '_blank');
            } else {
                // typically ngrok URLs don't have keycloak properly configured
                // to use the ngrok url, so use localhost in that case
                if (url.includes('ngrok')) {
                    url = url.replace(/https?:\/\/[^\/]+/, 'http://localhost:8080');
                }
                openDashboard(url);
            }
        }

        function openDashboard(url) {
            document.getElementById('dashboard-iframe').src = url;
            document.getElementById('iframe-container').style.display = 'block';
            adjustContentHeight();
        }

        function closeDashboard() {
            document.getElementById('iframe-container').style.display = 'none';
            document.getElementById('dashboard-iframe').src = '';
            adjustContentHeight();
        }

        function adjustContentHeight() {
            const mainContainer = document.querySelector('.main-container');
            const iframeContainer = document.getElementById('iframe-container');
            if (iframeContainer.style.display === 'block') {
                mainContainer.style.height = 'calc(100vh - ' + iframeContainer.offsetHeight + 'px)';
            } else {
                mainContainer.style.height = '100vh';
            }
        }

        // Improved resize handling
        const resizeHandle = document.getElementById('resize-handle');
        const iframeContainer = document.getElementById('iframe-container');
        const resizeOverlay = document.getElementById('resize-overlay');
        let isResizing = false;
        let startY = 0;
        let startHeight = 0;

        resizeHandle.addEventListener('mousedown', function(e) {
            e.preventDefault();
            isResizing = true;
            startY = e.clientY;
            startHeight = parseInt(window.getComputedStyle(iframeContainer).height);
            
            // Show the overlay to capture mouse events
            resizeOverlay.style.display = 'block';
            
            // Add resizing class for visual feedback
            document.body.classList.add('resizing');
        });

        document.addEventListener('mousemove', function(e) {
            if (!isResizing) return;
            
            const deltaY = startY - e.clientY;
            let newHeight = startHeight + deltaY;
            
            // Set reasonable limits
            newHeight = Math.max(150, Math.min(newHeight, window.innerHeight - 100));
            
            iframeContainer.style.height = newHeight + 'px';
            adjustContentHeight();
        });

        document.addEventListener('mouseup', function() {
            if (isResizing) {
                isResizing = false;
                resizeOverlay.style.display = 'none';
                document.body.classList.remove('resizing');
            }
        });

        // Ensure resize stops if mouse leaves window
        document.addEventListener('mouseleave', function() {
            if (isResizing) {
                isResizing = false;
                resizeOverlay.style.display = 'none';
                document.body.classList.remove('resizing');
            }
        });

        // Add additional style for body when resizing
        const style = document.createElement('style');
        style.textContent = 
            'body.resizing {' +
            '    cursor: ns-resize !important;' +
            '    user-select: none;' +
            '}' +
            'body.resizing iframe,' +
            'body.resizing a,' +
            'body.resizing button {' +
            '    pointer-events: none;' +
            '}' +
            '#resize-overlay {' +
            '    cursor: ns-resize;' +
            '}';
        document.head.appendChild(style);

        function viewHelixYaml() {
            const helixYaml = {{.HelixYaml}};
            const blob = new Blob([helixYaml], { type: 'text/yaml' });
            const url = URL.createObjectURL(blob);
            openDashboard(url);
        }

        // Tooltip functionality with improved responsiveness
        const tooltip = document.getElementById('tooltip');
        let activeElement = null;
        let lastMouseX = 0;
        let lastMouseY = 0;
        let tooltipCheckInterval = null;

        // Track mouse position globally
        document.addEventListener('mousemove', function(e) {
            lastMouseX = e.clientX;
            lastMouseY = e.clientY;
            
            if (activeElement) {
                updateTooltipPosition(e);
            }
        });

        function startTooltipCheck() {
            if (tooltipCheckInterval) clearInterval(tooltipCheckInterval);
            tooltipCheckInterval = setInterval(checkTooltipVisibility, 100);
        }

        function stopTooltipCheck() {
            if (tooltipCheckInterval) {
                clearInterval(tooltipCheckInterval);
                tooltipCheckInterval = null;
            }
        }

        function checkTooltipVisibility() {
            if (!activeElement || !tooltip.style.display === 'block') {
                hideTooltip();
                return;
            }

            const rect = activeElement.getBoundingClientRect();
            if (!isPointNearRect(lastMouseX, lastMouseY, rect, 20)) {
                hideTooltip();
            }
        }

        function isPointNearRect(x, y, rect, threshold) {
            return (
                x >= rect.left - threshold &&
                x <= rect.right + threshold &&
                y >= rect.top - threshold &&
                y <= rect.bottom + threshold
            );
        }

        function hideTooltip() {
            tooltip.style.display = 'none';
            activeElement = null;
            stopTooltipCheck();
        }

        document.querySelectorAll('.truncate').forEach(el => {
            el.addEventListener('mouseover', function(e) {
                activeElement = this;
                tooltip.textContent = this.getAttribute('data-full-text');
                tooltip.style.display = 'block';
                updateTooltipPosition(e);
                startTooltipCheck();
            });

            el.addEventListener('mouseout', function(e) {
                // Let the interval handle cleanup to avoid race conditions
                if (!isPointNearRect(e.clientX, e.clientY, this.getBoundingClientRect(), 20)) {
                    hideTooltip();
                }
            });
        });

        function updateTooltipPosition(e) {
            const padding = 5;
            const tooltipRect = tooltip.getBoundingClientRect();
            const viewportWidth = window.innerWidth;
            const viewportHeight = window.innerHeight;

            // Calculate initial position
            let left = e.clientX + padding;
            let top = e.clientY + padding;

            // Adjust if tooltip would go off-screen
            if (left + tooltipRect.width > viewportWidth) {
                left = e.clientX - tooltipRect.width - padding;
            }
            if (top + tooltipRect.height > viewportHeight) {
                top = e.clientY - tooltipRect.height - padding;
            }

            tooltip.style.left = left + 'px';
            tooltip.style.top = top + 'px';
        }

        // Initial adjustment
        adjustContentHeight();
    </script>
</body>
</html>
`

func NewTestCmd() *cobra.Command {
	var yamlFile string
	var evaluationModel string
	var syncFiles []string
	var deleteExtraFiles bool
	var knowledgeTimeout time.Duration
	var skipCleanup bool

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run tests for Helix agent",
		Long:  `This command runs tests defined in helix.yaml or a specified YAML file and evaluates the results.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTest(cmd, yamlFile, evaluationModel, syncFiles, deleteExtraFiles, knowledgeTimeout, skipCleanup)
		},
	}

	cmd.Flags().StringVarP(&yamlFile, "file", "f", "helix.yaml", "Path to the YAML file containing test definitions")
	cmd.Flags().StringVar(&evaluationModel, "evaluation-model", "", "Model to use for evaluating test results")
	cmd.Flags().StringSliceVar(&syncFiles, "rsync", []string{}, "Sync local files to the filestore for knowledge sources. Format: ./local/path[:knowledge_name]. If knowledge_name is omitted, uses the first knowledge source. Can be specified multiple times.")
	cmd.Flags().BoolVar(&deleteExtraFiles, "delete", false, "When used with --rsync, delete files in filestore that don't exist locally (similar to rsync --delete)")
	cmd.Flags().DurationVar(&knowledgeTimeout, "knowledge-timeout", 5*time.Minute, "Timeout when waiting for knowledge indexing")
	cmd.Flags().BoolVar(&skipCleanup, "skip-cleanup", false, "Skip cleaning up the test app after tests complete")

	return cmd
}

func runTest(cmd *cobra.Command, yamlFile string, evaluationModel string, syncFiles []string, deleteExtraFiles bool, knowledgeTimeout time.Duration, skipCleanup bool) error {
	appConfig, helixYamlContent, err := readHelixYaml(yamlFile)
	if err != nil {
		return err
	}

	testID := system.GenerateTestRunID()
	namespacedAppName := fmt.Sprintf("%s/%s", testID, appConfig.Name)

	helixURL := getHelixURL()

	apiKey, err := getAPIKey()
	if err != nil {
		return err
	}

	// Get available models if evaluation model is not specified
	if evaluationModel == "" {
		models, err := getAvailableModels(apiKey, helixURL)
		if err != nil {
			return fmt.Errorf("error getting available models: %v", err)
		}
		evaluationModel = models[0]
	}
	fmt.Printf("Using evaluation model: %s\n", evaluationModel)

	// Deploy the app with the namespaced name and appConfig
	appID, err := deployApp(namespacedAppName, yamlFile)
	if err != nil {
		return fmt.Errorf("error deploying app: %v", err)
	}

	fmt.Printf("Deployed app with ID: %s\n", appID)

	// Setup cleanup function
	cleanup := func() {
		if !skipCleanup {
			if err := deleteApp(namespacedAppName); err != nil {
				fmt.Printf("Error deleting app: %v\n", err)
			}
		}
	}
	// Ensure cleanup runs at the end unless skipped
	defer cleanup()

	// Initialize RAG metrics
	var ragMetrics *RAGMetrics
	if len(syncFiles) > 0 {
		ragMetrics = &RAGMetrics{
			KnowledgeSources: make([]KnowledgeSourceMetrics, 0),
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		mappings, err := cliutil.ParseSyncMappings(syncFiles, &appConfig)
		if err != nil {
			return err
		}

		totalFilesUploaded := 0
		totalUploadStart := time.Now()

		for _, mapping := range mappings {
			fmt.Printf("Syncing local directory '%s' to knowledge source '%s' (path: %s)\n",
				mapping.LocalDir, mapping.KnowledgeName, mapping.RemotePath)

			uploadStart := time.Now()
			fileCount, err := cliutil.SyncLocalDirToFilestore(cmd.Context(), apiClient, mapping.LocalDir, mapping.RemotePath, deleteExtraFiles, appID)
			if err != nil {
				return fmt.Errorf("failed to sync files for knowledge '%s': %w", mapping.KnowledgeName, err)
			}
			uploadTime := time.Since(uploadStart)

			totalFilesUploaded += fileCount
			ragMetrics.KnowledgeSources = append(ragMetrics.KnowledgeSources, KnowledgeSourceMetrics{
				Name:       mapping.KnowledgeName,
				FileCount:  fileCount,
				UploadTime: uploadTime,
				LocalDir:   mapping.LocalDir,
				RemotePath: mapping.RemotePath,
			})
		}

		ragMetrics.FilesUploaded = totalFilesUploaded
		ragMetrics.TotalUploadTime = time.Since(totalUploadStart)

		// After syncing all files, complete preparation and trigger indexing for all knowledge sources
		knowledgeFilter := &client.KnowledgeFilter{
			AppID: appID,
		}

		knowledge, err := apiClient.ListKnowledge(cmd.Context(), knowledgeFilter)
		if err != nil {
			return fmt.Errorf("failed to list knowledge sources: %w", err)
		}

		// Keep track of which knowledge sources need re-triggering
		needsRetrigger := make(map[string]*types.Knowledge)
		anyIndexing := false

		for _, k := range knowledge {
			// If knowledge is in "preparing" state, complete preparation
			if k.State == types.KnowledgeStatePreparing {
				fmt.Printf("Completing preparation for knowledge source %s (%s)\n", k.ID, k.Name)
				err = apiClient.CompleteKnowledgePreparation(cmd.Context(), k.ID)
				if err != nil {
					return fmt.Errorf("failed to complete preparation for knowledge %s (%s): %w", k.ID, k.Name, err)
				}
			} else if k.State == types.KnowledgeStateReady {
				// If knowledge is already ready, refresh it
				fmt.Printf("Refreshing knowledge source %s (%s)\n", k.ID, k.Name)
				err = apiClient.RefreshKnowledge(cmd.Context(), k.ID)
				if err != nil {
					// If knowledge is already queued for indexing or already being indexed, that's fine, we'll just wait
					if strings.Contains(err.Error(), "knowledge is queued for indexing") ||
						strings.Contains(err.Error(), "knowledge is already being indexed") {
						fmt.Printf("Knowledge %s (%s) is already being processed for indexing\n", k.ID, k.Name)
						needsRetrigger[k.ID] = k
						anyIndexing = true
						continue
					}
					return fmt.Errorf("failed to refresh knowledge %s (%s): %w", k.ID, k.Name, err)
				}
			} else if k.State == types.KnowledgeStateIndexing {
				// If knowledge is already indexing, add it to the re-trigger list
				fmt.Printf("Knowledge %s (%s) is already being processed for indexing\n", k.ID, k.Name)
				needsRetrigger[k.ID] = k
				anyIndexing = true
			}
		}

		if anyIndexing {
			fmt.Println("Some knowledge sources are already being indexed. Proceeding to wait for indexing to complete...")
		}

		// Wait for knowledge to be fully indexed before running tests
		fmt.Println("Waiting for knowledge to be indexed before running tests...")
		indexingStartTime := time.Now()
		err = cliutil.WaitForKnowledgeReady(cmd.Context(), apiClient, appID, knowledgeTimeout)
		if err != nil {
			return fmt.Errorf("error waiting for knowledge to be ready: %w", err)
		}
		indexingTime := time.Since(indexingStartTime)
		ragMetrics.TotalIndexingTime = indexingTime

		// If we detected any knowledge sources that were already indexing, re-trigger just those
		if len(needsRetrigger) > 0 {
			fmt.Printf("Previous indexing finished. Re-triggering indexing for %d knowledge source(s) that were already being processed...\n", len(needsRetrigger))

			for id, k := range needsRetrigger {
				fmt.Printf("Re-triggering indexing for knowledge source %s (%s)\n", id, k.Name)
				err = apiClient.RefreshKnowledge(cmd.Context(), id)
				if err != nil {
					// If knowledge is somehow still indexing, that's odd but we'll continue
					if strings.Contains(err.Error(), "knowledge is queued for indexing") ||
						strings.Contains(err.Error(), "knowledge is already being indexed") {
						fmt.Printf("Knowledge %s (%s) is somehow still being processed for indexing\n", id, k.Name)
						continue
					}
					return fmt.Errorf("failed to re-refresh knowledge %s (%s): %w", id, k.Name, err)
				}
			}

			// Wait for the second indexing to complete
			fmt.Println("Waiting for re-triggered indexing to complete...")
			reindexingStartTime := time.Now()
			err = cliutil.WaitForKnowledgeReady(cmd.Context(), apiClient, appID, knowledgeTimeout)
			if err != nil {
				return fmt.Errorf("error waiting for re-triggered indexing to complete: %w", err)
			}
			reindexingTime := time.Since(reindexingStartTime)

			// Update the total indexing time to include both waits
			ragMetrics.TotalIndexingTime = indexingTime + reindexingTime
			fmt.Printf("Re-triggered indexing completed in %s\n", reindexingTime)
		}

		// Update individual knowledge source indexing times
		for _, k := range knowledge {
			for j, ks := range ragMetrics.KnowledgeSources {
				if k.Name == ks.Name {
					ragMetrics.KnowledgeSources[j].IndexingTime = ragMetrics.TotalIndexingTime
					break
				}
			}
		}
	}

	fmt.Printf("Running tests...\n")

	results, totalTime, err := runTests(appConfig, appID, apiKey, helixURL, evaluationModel)
	if err != nil {
		return err
	}

	testResults := &TestResults{
		Tests:             results,
		TotalTime:         totalTime,
		RAGMetrics:        ragMetrics,
		HelixYaml:         helixYamlContent,
		TestID:            testID,
		NamespacedAppName: namespacedAppName,
	}

	displayResults(cmd, testResults, helixURL)

	err = writeResultsToFile(testResults)
	if err != nil {
		return err
	}

	// Check if any test failed
	for _, result := range results {
		if result.Result != "PASS" {
			cleanup() // Ensure cleanup runs before exit
			os.Exit(1)
		}
	}

	return nil
}

func readHelixYaml(yamlFile string) (types.AppHelixConfig, string, error) {
	// Read the raw YAML content first
	yamlContent, err := os.ReadFile(yamlFile)
	if err != nil {
		return types.AppHelixConfig{}, "", fmt.Errorf("error reading YAML file %s: %v", yamlFile, err)
	}

	helixYamlContent := string(yamlContent)

	// Use NewLocalApp to handle both regular config and CRD formats
	localApp, err := apps.NewLocalApp(yamlFile)
	if err != nil {
		return types.AppHelixConfig{}, "", fmt.Errorf("error parsing YAML file %s: %v", yamlFile, err)
	}

	appConfig := localApp.GetAppConfig()
	if appConfig == nil {
		return types.AppHelixConfig{}, "", fmt.Errorf("error: app config is nil")
	}

	return *appConfig, helixYamlContent, nil
}

func getAPIKey() (string, error) {
	apiKey := os.Getenv("HELIX_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("HELIX_API_KEY environment variable not set")
	}
	return apiKey, nil
}

func getHelixURL() string {
	helixURL := os.Getenv("HELIX_URL")
	if helixURL == "" {
		return "https://app.helix.ml"
	}
	return helixURL
}

func runTests(appConfig types.AppHelixConfig, appID, apiKey, helixURL, evaluationModel string) ([]TestResult, time.Duration, error) {
	totalStartTime := time.Now()

	resultsChan := make(chan TestResult)
	errorChan := make(chan error, 1) // Buffer of 1 to avoid blocking
	var wg sync.WaitGroup

	// Map to store session IDs for multi-turn conversations
	sessionCache := make(map[string]string) // key: assistantName+testName, value: sessionID

	// Use a mutex to protect concurrent access to the session cache
	var sessionMutex sync.Mutex

	for _, assistant := range appConfig.Assistants {
		for _, test := range assistant.Tests {
			// Process each test's steps sequentially for multi-turn conversation support
			wg.Add(1)
			go func(assistant types.AssistantConfig, test struct {
				Name  string           `json:"name,omitempty" yaml:"name,omitempty"`
				Steps []types.TestStep `json:"steps,omitempty" yaml:"steps,omitempty"`
			}) {
				// Add panic recovery for the outer goroutine
				defer func() {
					if r := recover(); r != nil {
						errorChan <- fmt.Errorf("panic in test execution: %v", r)
					}
					wg.Done()
				}()

				testKey := assistant.Name + "-" + test.Name

				for i, step := range test.Steps {
					wg.Add(1)

					// Create a new goroutine for each step but execute them in order
					func(assistantName, testName string, step types.TestStep, stepIndex int) {
						// Add panic recovery for the inner goroutine
						defer func() {
							if r := recover(); r != nil {
								errorChan <- fmt.Errorf("panic in test step execution: %v", r)
							}
							wg.Done()
						}()

						// Get the session ID from previous steps if this is a multi-turn conversation
						var sessionID string
						if stepIndex > 0 {
							sessionMutex.Lock()
							sessionID = sessionCache[testKey]
							sessionMutex.Unlock()
						}

						// Include step number in test name for multi-turn conversations
						stepTestName := testName
						if len(test.Steps) > 1 {
							stepTestName = fmt.Sprintf("%s (Turn %d/%d)", testName, stepIndex+1, len(test.Steps))
						}

						result, err := runSingleTest(assistantName, stepTestName, step, appID, apiKey, helixURL, assistant.Model, evaluationModel)
						if err != nil {
							result.Reason = err.Error()
							result.Result = "ERROR"
							fmt.Printf("Error running test %s: %v\n", stepTestName, err)
						}

						// Store the session ID for subsequent steps in this test
						if sessionID == "" && result.SessionID != "" {
							sessionMutex.Lock()
							sessionCache[testKey] = result.SessionID
							sessionMutex.Unlock()
						}

						// Mark result as part of a multi-turn conversation if needed
						if len(test.Steps) > 1 {
							result.IsMultiTurn = true
							result.TurnNumber = stepIndex + 1
							result.TotalTurns = len(test.Steps)
						}

						// Before sending, check if there's an error
						select {
						case err := <-errorChan:
							// Put the error back and return
							errorChan <- err
							return
						default:
							// Continue if no error
						}

						resultsChan <- result

						// Output . for pass, F for fail
						if result.Result == "PASS" {
							fmt.Print(".")
						} else {
							fmt.Print("F")
						}
					}(assistant.Name, test.Name, step, i)
				}
			}(assistant, test)
		}
	}

	// Wait for all tests to complete or first error
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results or handle errors
	var results []TestResult
	for {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				// Channel closed, we're done
				fmt.Println() // Add a newline after all tests have completed
				totalTime := time.Since(totalStartTime)
				sort.Slice(results, func(i, j int) bool {
					return results[i].TestName < results[j].TestName
				})
				return results, totalTime, nil
			}
			results = append(results, result)
		case err := <-errorChan:
			// Return early on error
			return results, time.Since(totalStartTime), err
		}
	}
}

// stripThinkTags removes <think>...</think> tags and any space after them from the evaluation content
func stripThinkTags(content string) string {
	// Use regex to remove <think>...</think> tags ((?s) makes . match newlines)
	re := regexp.MustCompile(`(?s)<think>.*?</think>\s*`)
	return re.ReplaceAllString(content, "")
}

// Example usage and test cases for stripThinkTags:
// "<think>reasoning</think> PASS" -> "PASS"
// "<think>some reasoning here</think> FAIL because xyz" -> "FAIL because xyz"
// "PASS without think tags" -> "PASS without think tags"
// "<think>nested <tags> here</think> PASS" -> "PASS"

func runSingleTest(assistantName, testName string, step types.TestStep, appID, apiKey, helixURL, model, evaluationModel string) (TestResult, error) {
	inferenceStartTime := time.Now()

	// partial result in case of error
	result := TestResult{
		TestName:        fmt.Sprintf("%s - %s", assistantName, testName),
		Prompt:          step.Prompt,
		Expected:        step.ExpectedOutput,
		Model:           model,
		EvaluationModel: evaluationModel,
		HelixURL:        helixURL,
	}

	chatReq := ChatRequest{
		Messages: []Message{
			{
				Role: "user",
				Content: Content{
					ContentType: "text",
					Parts:       []string{step.Prompt},
				},
			},
		},
		AppID: appID,
	}

	responseContent, chatResp, err := sendChatRequest(chatReq, apiKey, helixURL)
	if err != nil {
		return result, err
	}

	inferenceTime := time.Since(inferenceStartTime)

	evaluationStartTime := time.Now()

	evalReq := ChatRequest{
		Model:  evaluationModel,
		System: "You are an AI assistant tasked with evaluating test results. Output only PASS or FAIL followed by a brief explanation on the next line. Be fairly liberal about what you consider to be a PASS, as long as everything specifically requested is present. However, if the response is not as expected, you should output FAIL.",
		Messages: []Message{
			{
				Role: "user",
				Content: Content{
					ContentType: "text",
					Parts:       []string{fmt.Sprintf("Does this response:\n\n%s\n\nsatisfy the expected output:\n\n%s", responseContent, step.ExpectedOutput)},
				},
			},
		},
	}

	evalContent, _, err := sendChatRequest(evalReq, apiKey, helixURL)
	if err != nil {
		return result, err
	}

	evaluationTime := time.Since(evaluationStartTime)

	// Strip think tags from evaluation content before parsing
	cleanedEvalContent := stripThinkTags(evalContent)

	result.Response = responseContent

	// Parse result and reason safely
	if len(cleanedEvalContent) >= 4 {
		result.Result = cleanedEvalContent[:4]
		if len(cleanedEvalContent) > 5 {
			result.Reason = cleanedEvalContent[5:]
		} else {
			result.Reason = ""
		}
	} else {
		result.Result = "FAIL"
		result.Reason = "Invalid evaluation response format"
	}

	result.SessionID = chatResp.ID
	result.InferenceTime = inferenceTime
	result.EvaluationTime = evaluationTime

	return result, nil
}

func sendChatRequest(req ChatRequest, apiKey, helixURL string) (string, ChatResponse, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", ChatResponse{}, fmt.Errorf("error marshaling JSON: %v", err)
	}

	httpReq, err := http.NewRequest("POST", helixURL+"/api/v1/sessions/chat", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", ChatResponse{}, fmt.Errorf("error creating request: %v", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", ChatResponse{}, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", ChatResponse{}, fmt.Errorf("error reading response: %v", err)
	}

	var chatResp ChatResponse
	err = json.Unmarshal(body, &chatResp)
	if err != nil {
		return "", ChatResponse{}, fmt.Errorf("error parsing response JSON: %v (response body: %s)", err, string(body))
	}

	if len(chatResp.Choices) == 0 {
		return "", ChatResponse{}, fmt.Errorf("no choices in the response")
	}

	return chatResp.Choices[0].Message.Content, chatResp, nil
}

func displayResults(cmd *cobra.Command, results *TestResults, helixURL string) {
	cmd.Println(generateResultsSummary(results, helixURL))
}

func generateResultsSummary(results *TestResults, helixURL string) string {
	var builder strings.Builder

	// Add RAG metrics if available
	if results.RAGMetrics != nil {
		builder.WriteString("\nRAG Benchmark:\n")
		builder.WriteString(fmt.Sprintf("Total Files Uploaded: %d\n", results.RAGMetrics.FilesUploaded))
		builder.WriteString(fmt.Sprintf("Total Upload Time: %.2fs\n", results.RAGMetrics.TotalUploadTime.Seconds()))
		builder.WriteString(fmt.Sprintf("Total Indexing Time: %.2fs\n\n", results.RAGMetrics.TotalIndexingTime.Seconds()))

		if len(results.RAGMetrics.KnowledgeSources) > 0 {
			builder.WriteString("Knowledge Sources:\n")
			for _, ks := range results.RAGMetrics.KnowledgeSources {
				builder.WriteString(fmt.Sprintf("- %s: %d files, Upload: %.2fs, Index: %.2fs\n",
					ks.Name, ks.FileCount, ks.UploadTime.Seconds(), ks.IndexingTime.Seconds()))
			}
			builder.WriteString("\n")
		}
	}

	// Add test results table
	builder.WriteString("| Test Name | Result | Reason | Model | Inference Time | Evaluation Time | Session Link | Debug Link |\n")
	builder.WriteString("|-----------|--------|--------|-------|----------------|-----------------|--------------|------------|\n")

	// If helixURL contains ngrok, use localhost instead
	reportURL := helixURL
	if strings.Contains(reportURL, "ngrok") {
		reportURL = "http://localhost:8080"
	}

	overallResult := "PASS"
	for _, result := range results.Tests {
		sessionLink := fmt.Sprintf("%s/session/%s", reportURL, result.SessionID)
		debugLink := fmt.Sprintf("%s/dashboard?tab=llm_calls&filter_sessions=%s", reportURL, result.SessionID)

		testName := result.TestName
		if result.IsMultiTurn {
			testName = fmt.Sprintf("%s (Multi-turn: %d turns)", result.TestName, len(result.TurnResults))
		}

		builder.WriteString(fmt.Sprintf("| %-20s | %-6s | %-50s | %-25s | %-15s | %-15s | [Session](%s) | [Debug](%s) |\n",
			testName,
			result.Result,
			truncate(result.Reason, 50),
			result.Model,
			result.InferenceTime.Round(time.Millisecond),
			result.EvaluationTime.Round(time.Millisecond),
			sessionLink,
			debugLink))

		if result.Result != "PASS" {
			overallResult = "FAIL"
		}
	}

	builder.WriteString(fmt.Sprintf("\nTotal execution time: %s\n", results.TotalTime.Round(time.Millisecond)))
	builder.WriteString(fmt.Sprintf("Overall result: %s\n", overallResult))

	// Add report link at the bottom
	builder.WriteString(fmt.Sprintf("\n* [View full test report 🚀](%s/files?path=/test-runs/%s)\n",
		reportURL,
		results.TestID))

	return builder.String()
}

func writeResultsToFile(results *TestResults) error {
	timestamp := time.Now().Format("20060102150405")

	// Create test_results directory if it doesn't exist
	resultsDir := "test_results"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("error creating test_results directory: %v", err)
	}

	// Define filenames with appropriate paths
	jsonFilename := fmt.Sprintf("%s/results_%s_%s.json", resultsDir, results.TestID, timestamp)
	htmlFilename := fmt.Sprintf("%s/report_%s_%s.html", resultsDir, results.TestID, timestamp)
	summaryFilename := fmt.Sprintf("%s/summary_%s_%s.md", resultsDir, results.TestID, timestamp)

	// Define latest report in current directory
	reportLatestFilename := "report_latest.html"

	// Write JSON results
	jsonResults, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling results to JSON: %v", err)
	}
	err = os.WriteFile(jsonFilename, jsonResults, 0644)
	if err != nil {
		return fmt.Errorf("error writing results to JSON file: %v", err)
	}

	// Generate and write HTML report
	tmpl, err := template.New("results").Funcs(template.FuncMap{
		"truncate": truncate,
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("error parsing HTML template: %v", err)
	}

	htmlFile, err := os.Create(htmlFilename)
	if err != nil {
		return fmt.Errorf("error creating HTML file: %v", err)
	}
	defer htmlFile.Close()

	data := struct {
		Tests                []TestResult
		TotalExecutionTime   string
		LatestResultsFile    string
		AvailableResultFiles []string
		HelixYaml            string
		HelixURL             string
		RAGMetrics           *RAGMetrics
	}{
		Tests:                results.Tests,
		TotalExecutionTime:   results.TotalTime.String(),
		LatestResultsFile:    jsonFilename,
		AvailableResultFiles: []string{jsonFilename},
		HelixYaml:            results.HelixYaml,
		HelixURL:             getHelixURL(),
		RAGMetrics:           results.RAGMetrics,
	}

	err = tmpl.Execute(htmlFile, data)
	if err != nil {
		return fmt.Errorf("error executing HTML template: %v", err)
	}

	// Also create report_latest.html in current directory
	reportLatestFile, err := os.Create(reportLatestFilename)
	if err != nil {
		return fmt.Errorf("error creating latest HTML report file: %v", err)
	}
	defer reportLatestFile.Close()

	err = tmpl.Execute(reportLatestFile, data)
	if err != nil {
		return fmt.Errorf("error executing HTML template for latest report: %v", err)
	}

	// Write summary markdown file to test_results directory
	summaryContent := "# Helix Test Summary\n\n" + generateResultsSummary(results, getHelixURL())
	err = os.WriteFile(summaryFilename, []byte(summaryContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing summary to markdown file: %v", err)
	}

	// Keep summary_latest.md in current directory as requested
	err = os.WriteFile("summary_latest.md", []byte(summaryContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing latest summary to markdown file: %v", err)
	}

	// Create a client for uploading
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	ctx := context.Background()

	// Upload JSON results - extract filename without directory for remote path
	jsonBasename := filepath.Base(jsonFilename)
	jsonPath := fmt.Sprintf("/test-runs/%s/%s", results.TestID, jsonBasename)
	err = cliutil.UploadFile(ctx, apiClient, jsonFilename, jsonPath)
	if err != nil {
		return fmt.Errorf("error uploading JSON results: %v", err)
	}

	// Upload HTML report - extract filename without directory for remote path
	htmlBasename := filepath.Base(htmlFilename)
	htmlPath := fmt.Sprintf("/test-runs/%s/%s", results.TestID, htmlBasename)
	err = cliutil.UploadFile(ctx, apiClient, htmlFilename, htmlPath)
	if err != nil {
		return fmt.Errorf("error uploading HTML report: %v", err)
	}

	// Upload summary markdown - extract filename without directory for remote path
	summaryBasename := filepath.Base(summaryFilename)
	summaryPath := fmt.Sprintf("/test-runs/%s/%s", results.TestID, summaryBasename)
	err = cliutil.UploadFile(ctx, apiClient, summaryFilename, summaryPath)
	if err != nil {
		return fmt.Errorf("error uploading summary markdown: %v", err)
	}

	fmt.Printf("\nResults written to %s\n", jsonFilename)
	fmt.Printf("HTML report written to %s\n", htmlFilename)
	fmt.Printf("Latest HTML report written to %s\n", reportLatestFilename)
	fmt.Printf("Summary written to %s\n", summaryFilename)
	fmt.Printf("Latest summary written to summary_latest.md\n")
	helixURL := getHelixURL()
	if strings.Contains(helixURL, "ngrok") {
		helixURL = "http://localhost:8080"
	}
	fmt.Printf("View results at: %s/files?path=/test-runs/%s\n", helixURL, results.TestID)

	// Attempt to open the HTML report in the default browser
	if isGraphicalEnvironment() {
		openBrowser(getHelixURL() + "/files?path=/test-runs/" + results.TestID)
	}

	return nil
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func deployApp(namespacedAppName string, yamlFile string) (string, error) {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Use NewLocalApp to create the app from the original config
	localApp, err := apps.NewLocalApp(yamlFile)
	if err != nil {
		return "", fmt.Errorf("failed to create local app: %w", err)
	}

	// Get the parsed app config and override the Name field
	parsedAppConfig := localApp.GetAppConfig()
	parsedAppConfig.Name = namespacedAppName

	// Create the app using the same logic as in applyCmd
	app := &types.App{
		Global: false,
		Config: types.AppConfig{
			AllowedDomains: []string{},
			Helix:          *parsedAppConfig,
		},
	}

	createdApp, err := apiClient.CreateApp(context.Background(), app)
	if err != nil {
		return "", fmt.Errorf("failed to create app: %w", err)
	}

	return createdApp.ID, nil
}

func deleteApp(namespacedAppName string) error {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	ctx := context.Background()

	// First, we need to look up the app by name
	existingApps, err := apiClient.ListApps(ctx, &client.AppFilter{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	var appID string
	for _, existingApp := range existingApps {
		if existingApp.Config.Helix.Name == namespacedAppName {
			appID = existingApp.ID
			break
		}
	}

	if appID == "" {
		return fmt.Errorf("app with name %s not found", namespacedAppName)
	}

	// Delete the app
	if err := apiClient.DeleteApp(ctx, appID, true); err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}

	return nil
}

// isGraphicalEnvironment checks if the user is in a graphical environment
func isGraphicalEnvironment() bool {
	switch runtime.GOOS {
	case "linux":
		// Check for common Linux graphical environment variables
		display := os.Getenv("DISPLAY")
		wayland := os.Getenv("WAYLAND_DISPLAY")
		return display != "" || wayland != ""
	case "darwin":
		// On macOS, we assume a graphical environment is always present
		return true
	case "windows":
		// On Windows, check if the process is interactive
		_, err := exec.LookPath("cmd.exe")
		return err == nil
	default:
		// For other operating systems, assume no graphical environment
		return false
	}
}

// openBrowser attempts to open the given URL in the default browser
func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		fmt.Printf("Error opening browser: %v\n", err)
	}
}

func getAvailableModels(apiKey, helixURL string) ([]string, error) {
	req, err := http.NewRequest("GET", helixURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching models: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var modelResp ModelResponse
	err = json.Unmarshal(body, &modelResp)
	if err != nil {
		return nil, fmt.Errorf("error parsing response JSON: %v (response body: %s) calling URL %s with apiKey '%s'", err, string(body), helixURL+"/v1/models", apiKey)
	}

	if len(modelResp.Data) == 0 {
		return nil, fmt.Errorf("no models available")
	}

	// Extract model IDs from the response
	models := make([]string, 0, len(modelResp.Data))
	for _, model := range modelResp.Data {
		models = append(models, model.ID)
	}

	return models, nil
}
