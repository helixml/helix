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
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"html/template"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/cli/fs"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
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

type TestResult struct {
	TestName       string        `json:"test_name"`
	Prompt         string        `json:"prompt"`
	Response       string        `json:"response"`
	Expected       string        `json:"expected"`
	Result         string        `json:"result"`
	Reason         string        `json:"reason"`
	SessionID      string        `json:"session_id"`
	Model          string        `json:"model"`
	InferenceTime  time.Duration `json:"inference_time"`
	EvaluationTime time.Duration `json:"evaluation_time"`
	HelixURL       string        `json:"helix_url"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

const htmlTemplate = `
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
            border-bottom: 1px solid #ddd;
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
        }
        .header-info p {
            margin: 0;
            font-size: 0.9em;
        }
        .header-controls {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .results-container { 
            flex: 1;
            overflow-y: auto;
            padding: 0 20px;
        }
        table { 
            border-collapse: collapse; 
            width: 100%; 
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
        #iframe-container { 
            display: none; 
            position: fixed; 
            bottom: 0; 
            left: 0; 
            width: 100%; 
            height: 70%; 
            border: none; 
        }
        #iframe-container iframe { 
            width: 100%; 
            height: calc(100% - 10px); 
            border: none; 
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
                        <td><a href="#" onclick="openLink('{{.HelixURL}}/session/{{.SessionID}}'); return false;">Session</a></td>
                        <td><a href="#" onclick="openLink('{{.HelixURL}}/dashboard?tab=llm_calls&filter_sessions={{.SessionID}}'); return false;">Debug</a></td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
    <div id="iframe-container">
        <div id="resize-handle"></div>
        <div id="close-iframe" onclick="closeDashboard()" style="color: white;">Close</div>
        <iframe id="dashboard-iframe" src=""></iframe>
    </div>
    <div id="tooltip" class="tooltip"></div>
    <script>
        function openLink(url) {
            if (window.location.protocol === 'file:') {
                window.open(url, '_blank');
            } else {
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

        // Resizing functionality
        const resizeHandle = document.getElementById('resize-handle');
        const iframeContainer = document.getElementById('iframe-container');
        let isResizing = false;

        resizeHandle.addEventListener('mousedown', function(e) {
            isResizing = true;
            document.addEventListener('mousemove', resize);
            document.addEventListener('mouseup', stopResize);
        });

        function resize(e) {
            if (!isResizing) return;
            const newHeight = window.innerHeight - e.clientY;
            iframeContainer.style.height = newHeight + 'px';
            adjustContentHeight();
        }

        function stopResize() {
            isResizing = false;
            document.removeEventListener('mousemove', resize);
        }

        function viewHelixYaml() {
            const helixYaml = {{.HelixYaml}};
            const blob = new Blob([helixYaml], { type: 'text/yaml' });
            const url = URL.createObjectURL(blob);
            openDashboard(url);
        }

        // Tooltip functionality
        const tooltip = document.getElementById('tooltip');
        document.querySelectorAll('.truncate').forEach(el => {
            el.addEventListener('mouseover', function(e) {
                tooltip.textContent = this.getAttribute('data-full-text');
                tooltip.style.display = 'block';
                tooltip.style.left = e.pageX + 'px';
                tooltip.style.top = e.pageY + 'px';
            });
            el.addEventListener('mouseout', function() {
                tooltip.style.display = 'none';
            });
        });

        // Initial adjustment
        adjustContentHeight();
    </script>
</body>
</html>
`

func NewTestCmd() *cobra.Command {
	var yamlFile string

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run tests for Helix app",
		Long:  `This command runs tests defined in helix.yaml or a specified YAML file and evaluates the results.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTest(cmd, yamlFile)
		},
	}

	cmd.Flags().StringVarP(&yamlFile, "file", "f", "helix.yaml", "Path to the YAML file containing test definitions")

	return cmd
}

func runTest(cmd *cobra.Command, yamlFile string) error {
	appConfig, helixYamlContent, err := readHelixYaml(yamlFile)
	if err != nil {
		return err
	}

	testID := system.GenerateTestRunID()
	namespacedAppName := fmt.Sprintf("%s/%s", testID, appConfig.Name)

	// Deploy the app with the namespaced name and appConfig
	appID, err := deployApp(namespacedAppName, yamlFile)
	if err != nil {
		return fmt.Errorf("error deploying app: %v", err)
	}

	fmt.Printf("Deployed app with ID: %s\n", appID)
	fmt.Printf("Running tests...\n")

	defer func() {
		// Clean up the app after the test
		err := deleteApp(namespacedAppName)
		if err != nil {
			fmt.Printf("Error deleting app: %v\n", err)
		}
	}()

	apiKey, err := getAPIKey()
	if err != nil {
		return err
	}

	helixURL := getHelixURL()

	results, totalTime, err := runTests(appConfig, appID, apiKey, helixURL)
	if err != nil {
		return err
	}

	displayResults(cmd, results, totalTime, helixURL)

	err = writeResultsToFile(results, totalTime, helixYamlContent, testID, namespacedAppName)
	if err != nil {
		return err
	}

	// Check if any test failed
	for _, result := range results {
		if result.Result != "PASS" {
			os.Exit(1)
		}
	}

	return nil
}

func readHelixYaml(yamlFile string) (types.AppHelixConfig, string, error) {
	yamlContent, err := os.ReadFile(yamlFile)
	if err != nil {
		return types.AppHelixConfig{}, "", fmt.Errorf("error reading YAML file %s: %v", yamlFile, err)
	}

	helixYamlContent := string(yamlContent)

	var appConfig types.AppHelixConfig
	err = yaml.Unmarshal(yamlContent, &appConfig)
	if err != nil {
		return types.AppHelixConfig{}, "", fmt.Errorf("error parsing YAML file %s: %v", yamlFile, err)
	}

	return appConfig, helixYamlContent, nil
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
		return "https://app.tryhelix.ai"
	}
	return helixURL
}

func runTests(appConfig types.AppHelixConfig, appID, apiKey, helixURL string) ([]TestResult, time.Duration, error) {
	var results []TestResult
	totalStartTime := time.Now()

	resultsChan := make(chan TestResult)
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for _, assistant := range appConfig.Assistants {
		for _, test := range assistant.Tests {
			for _, step := range test.Steps {
				wg.Add(1)
				go func(assistantName, testName string, step struct {
					Prompt         string `json:"prompt" yaml:"prompt"`
					ExpectedOutput string `json:"expected_output" yaml:"expected_output"`
				}) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					result, err := runSingleTest(assistantName, testName, step, appID, apiKey, helixURL, assistant.Model)
					if err != nil {
						result.Reason = err.Error()
						result.Result = "ERROR"
						fmt.Printf("Error running test %s: %v\n", testName, err)
					}

					resultsChan <- result

					// Output . for pass, F for fail
					if result.Result == "PASS" {
						fmt.Print(".")
					} else {
						fmt.Print("F")
					}
				}(assistant.Name, test.Name, step)
			}
		}
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		results = append(results, result)
	}

	fmt.Println() // Add a newline after all tests have completed

	sort.Slice(results, func(i, j int) bool {
		return results[i].TestName < results[j].TestName
	})

	totalTime := time.Since(totalStartTime)

	return results, totalTime, nil
}

func runSingleTest(assistantName, testName string, step struct {
	Prompt         string `json:"prompt" yaml:"prompt"`
	ExpectedOutput string `json:"expected_output" yaml:"expected_output"`
}, appID, apiKey, helixURL, model string) (TestResult, error) {
	inferenceStartTime := time.Now()

	// partial result in case of error
	result := TestResult{
		TestName: fmt.Sprintf("%s - %s", assistantName, testName),
		Prompt:   step.Prompt,
		Expected: step.ExpectedOutput,
		Model:    model,
		HelixURL: helixURL,
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
		Model:  "llama3.1:8b-instruct-q8_0",
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

	result.Response = responseContent
	result.Result = evalContent[:4]
	result.Reason = evalContent[5:]
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
		return "", ChatResponse{}, fmt.Errorf("error parsing response JSON: %v (%s)", err, string(body))
	}

	if len(chatResp.Choices) == 0 {
		return "", ChatResponse{}, fmt.Errorf("no choices in the response")
	}

	return chatResp.Choices[0].Message.Content, chatResp, nil
}

func generateResultsSummary(results []TestResult, totalTime time.Duration, helixURL string) string {
	var builder strings.Builder
	builder.WriteString("| Test Name | Result | Reason | Model | Inference Time | Evaluation Time | Session Link | Debug Link |\n")
	builder.WriteString("|-----------|--------|--------|-------|----------------|-----------------|--------------|------------|\n")

	overallResult := "PASS"
	for _, result := range results {
		sessionLink := fmt.Sprintf("%s/session/%s", helixURL, result.SessionID)

		debugLink := fmt.Sprintf("%s/dashboard?tab=llm_calls&filter_sessions=%s", helixURL, result.SessionID)
		builder.WriteString(fmt.Sprintf("| %-20s | %-6s | %-50s | %-25s | %-15s | %-15s | [Session](%s) | [Debug](%s) |\n",
			result.TestName,
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

	builder.WriteString(fmt.Sprintf("\nTotal execution time: %s\n", totalTime.Round(time.Millisecond)))
	builder.WriteString(fmt.Sprintf("Overall result: %s\n", overallResult))

	return builder.String()
}

func displayResults(cmd *cobra.Command, results []TestResult, totalTime time.Duration, helixURL string) {
	cmd.Println(generateResultsSummary(results, totalTime, helixURL))
}

func writeResultsToFile(results []TestResult, totalTime time.Duration, helixYamlContent string, testID, namespacedAppName string) error {
	timestamp := time.Now().Format("20060102150405")
	jsonFilename := fmt.Sprintf("results_%s_%s.json", testID, timestamp)
	htmlFilename := fmt.Sprintf("report_%s_%s.html", testID, timestamp)
	summaryFilename := fmt.Sprintf("summary_%s_%s.md", testID, timestamp)

	resultMap := map[string]interface{}{
		"test_id":              testID,
		"namespaced_app_name":  namespacedAppName,
		"tests":                results,
		"total_execution_time": totalTime.String(),
		"helix_yaml":           helixYamlContent,
	}

	// Write JSON results
	jsonResults, err := json.MarshalIndent(resultMap, "", "  ")
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
	}{
		Tests:                results,
		TotalExecutionTime:   totalTime.String(),
		LatestResultsFile:    jsonFilename,
		AvailableResultFiles: []string{jsonFilename},
		HelixYaml:            helixYamlContent,
		HelixURL:             getHelixURL(),
	}

	err = tmpl.Execute(htmlFile, data)
	if err != nil {
		return fmt.Errorf("error executing HTML template: %v", err)
	}

	// Write summary markdown file
	summaryContent := "# Helix Test Summary\n\n" + generateResultsSummary(results, totalTime, getHelixURL())
	err = os.WriteFile(summaryFilename, []byte(summaryContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing summary to markdown file: %v", err)
	}
	err = os.WriteFile("summary_latest.md", []byte(summaryContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing summary to markdown file: %v", err)
	}

	// Create a client for uploading
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	ctx := context.Background()

	// Upload JSON results
	jsonPath := fmt.Sprintf("/test-runs/%s/%s", testID, jsonFilename)
	err = fs.UploadFile(ctx, apiClient, jsonFilename, jsonPath)
	if err != nil {
		return fmt.Errorf("error uploading JSON results: %v", err)
	}

	// Upload HTML report
	htmlPath := fmt.Sprintf("/test-runs/%s/%s", testID, htmlFilename)
	err = fs.UploadFile(ctx, apiClient, htmlFilename, htmlPath)
	if err != nil {
		return fmt.Errorf("error uploading HTML report: %v", err)
	}

	// Upload summary markdown
	summaryPath := fmt.Sprintf("/test-runs/%s/%s", testID, summaryFilename)
	err = fs.UploadFile(ctx, apiClient, summaryFilename, summaryPath)
	if err != nil {
		return fmt.Errorf("error uploading summary markdown: %v", err)
	}

	fmt.Printf("\nResults written to %s\n", jsonFilename)
	fmt.Printf("HTML report written to %s\n", htmlFilename)
	fmt.Printf("Summary written to %s\n", summaryFilename)

	// Attempt to open the HTML report in the default browser
	if isGraphicalEnvironment() {
		openBrowser(getHelixURL() + "/files?path=/test-runs/" + testID)
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
		AppSource: types.AppSourceHelix,
		Global:    false,
		Shared:    false,
		Config: types.AppConfig{
			AllowedDomains: []string{},
			Helix:          *parsedAppConfig,
		},
	}

	createdApp, err := apiClient.CreateApp(app)
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

	// First, we need to look up the app by name
	existingApps, err := apiClient.ListApps(&client.AppFilter{})
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
	if err := apiClient.DeleteApp(appID, true); err != nil {
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
