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
	"github.com/sashabaranov/go-openai"
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

// Template for HTML report
var htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Helix Test Results</title>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 20px;
            color: #333;
            background-color: #f9f9f9;
        }
        h1, h2, h3 {
            color: #0066cc;
        }
        .summary {
            margin-bottom: 30px;
            padding: 15px;
            background-color: #fff;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .summary table {
            width: 100%;
            border-collapse: collapse;
        }
        .summary th, .summary td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        .summary th {
            background-color: #f2f2f2;
        }
        .pass {
            color: green;
            font-weight: bold;
        }
        .fail {
            color: red;
            font-weight: bold;
        }
        .error {
            color: orange;
            font-weight: bold;
        }
        .test {
            margin-bottom: 20px;
            padding: 15px;
            background-color: #fff;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .test-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            cursor: pointer;
            padding: 10px;
            background-color: #f2f2f2;
            border-radius: 5px;
        }
        .test-content {
            display: none;
            margin-top: 10px;
        }
        .expanded .test-content {
            display: block;
        }
        pre {
            white-space: pre-wrap;
            background-color: #f5f5f5;
            padding: 10px;
            border-radius: 5px;
            overflow-x: auto;
        }
        .turn {
            border: 1px solid #ddd;
            border-radius: 5px;
            margin-bottom: 10px;
            padding: 10px;
        }
        .turn-header {
            font-weight: bold;
            margin-bottom: 5px;
        }
        .toggle-button {
            background-color: #0066cc;
            color: white;
            border: none;
            padding: 5px 10px;
            border-radius: 4px;
            cursor: pointer;
        }
        .toggle-button:hover {
            background-color: #004c99;
        }
    </style>
</head>
<body>
    <h1>Helix Test Results</h1>
    
    <div class="summary">
        <h2>Summary</h2>
        <table>
            <tr>
                <th>Total Tests</th>
                <th>Passed</th>
                <th>Failed</th>
                <th>Errors</th>
                <th>Duration</th>
            </tr>
            <tr>
                <td>{{ .TotalTests }}</td>
                <td class="pass">{{ .Passed }}</td>
                <td class="fail">{{ .Failed }}</td>
                <td class="error">{{ .Errors }}</td>
                <td>{{ .Duration }}</td>
            </tr>
        </table>
        {{ if .ResultsTable }}
        <h3>Results</h3>
        {{ .ResultsTable }}
        {{ end }}
    </div>
    
    <h2>Test Details</h2>
    
    {{ range .Results }}
    <div class="test">
        <div class="test-header" onclick="toggleTest(this.parentElement)">
            <div>
                <span class="{{ .Result | lower }}">{{ .Result }}</span> - {{ .TestName }}
                {{ if .IsMultiTurn }}
                <span>[Turn {{ .TurnNumber }}/{{ .TotalTurns }}]</span>
                {{ end }}
            </div>
            <button class="toggle-button">Show/Hide Details</button>
        </div>
        <div class="test-content">
            <h3>Details</h3>
            <p><strong>Model:</strong> {{ .Model }}</p>
            <p><strong>Evaluation Model:</strong> {{ .EvaluationModel }}</p>
            <p><strong>Inference Time:</strong> {{ .InferenceTime }}</p>
            <p><strong>Evaluation Time:</strong> {{ .EvaluationTime }}</p>
            <p><strong>Reason:</strong> {{ .Reason }}</p>
            <p><strong>Session ID:</strong> {{ .SessionID }}</p>
            
            {{ if .IsMultiTurn }}
            <h3>Current Turn</h3>
            <div class="turn">
                <div class="turn-header">User Prompt:</div>
                <pre>{{ .Prompt }}</pre>
                <div class="turn-header">Assistant Response:</div>
                <pre>{{ .Response }}</pre>
                <div class="turn-header">Expected Output:</div>
                <pre>{{ .Expected }}</pre>
            </div>
            {{ else }}
            <h3>Prompt and Response</h3>
            <p><strong>Prompt:</strong></p>
            <pre>{{ .Prompt }}</pre>
            <p><strong>Response:</strong></p>
            <pre>{{ .Response }}</pre>
            <p><strong>Expected Output:</strong></p>
            <pre>{{ .Expected }}</pre>
            {{ end }}
            
            <h3>Links</h3>
            <p><a href="{{ .HelixURL }}/session/{{ .SessionID }}" target="_blank">View Session</a></p>
            <p><a href="{{ .HelixURL }}/session/{{ .SessionID }}/debug" target="_blank">Debug Session</a></p>
        </div>
    </div>
    {{ end }}
    
    <script>
        function toggleTest(element) {
            element.classList.toggle('expanded');
        }
        
        // Expand tests that failed
        document.addEventListener('DOMContentLoaded', function() {
            const failedTests = document.querySelectorAll('.test:has(.fail)');
            failedTests.forEach(function(test) {
                test.classList.add('expanded');
            });
        });
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

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run tests for Helix app",
		Long:  `This command runs tests defined in helix.yaml or a specified YAML file and evaluates the results.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTest(cmd, yamlFile, evaluationModel, syncFiles, deleteExtraFiles, knowledgeTimeout)
		},
	}

	cmd.Flags().StringVarP(&yamlFile, "file", "f", "helix.yaml", "Path to the YAML file containing test definitions")
	cmd.Flags().StringVar(&evaluationModel, "evaluation-model", "", "Model to use for evaluating test results")
	cmd.Flags().StringSliceVar(&syncFiles, "rsync", []string{}, "Sync local files to the filestore for knowledge sources. Format: ./local/path[:knowledge_name]. If knowledge_name is omitted, uses the first knowledge source. Can be specified multiple times.")
	cmd.Flags().BoolVar(&deleteExtraFiles, "delete", false, "When used with --rsync, delete files in filestore that don't exist locally (similar to rsync --delete)")
	cmd.Flags().DurationVar(&knowledgeTimeout, "knowledge-timeout", 5*time.Minute, "Timeout when waiting for knowledge indexing")

	return cmd
}

func runTest(cmd *cobra.Command, yamlFile string, evaluationModel string, syncFiles []string, deleteExtraFiles bool, knowledgeTimeout time.Duration) error {
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
	fmt.Printf("Running tests...\n")

	// Handle the --rsync flag to sync local files to the filestore
	if len(syncFiles) > 0 {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		mappings, err := cliutil.ParseSyncMappings(syncFiles, &appConfig)
		if err != nil {
			return err
		}

		for _, mapping := range mappings {
			fmt.Printf("Syncing local directory '%s' to knowledge source '%s' (path: %s)\n",
				mapping.LocalDir, mapping.KnowledgeName, mapping.RemotePath)

			err = cliutil.SyncLocalDirToFilestore(cmd.Context(), apiClient, mapping.LocalDir, mapping.RemotePath, deleteExtraFiles)
			if err != nil {
				return fmt.Errorf("failed to sync files for knowledge '%s': %w", mapping.KnowledgeName, err)
			}
		}
	}

	// Wait for knowledge to be fully indexed before running tests
	fmt.Println("Waiting for knowledge to be indexed before running tests...")
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return err
	}

	err = cliutil.WaitForKnowledgeReady(cmd.Context(), apiClient, appID, knowledgeTimeout)
	if err != nil {
		return err
	}

	defer func() {
		// Clean up the app after the test
		err := deleteApp(namespacedAppName)
		if err != nil {
			fmt.Printf("Error deleting app: %v\n", err)
		}
	}()

	results, totalTime, err := runTests(appConfig, appID, apiKey, helixURL, evaluationModel)
	if err != nil {
		return err
	}

	displayResults(cmd, results, totalTime, helixURL, testID)

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
		return "https://app.tryhelix.ai"
	}
	return helixURL
}

func runTests(appConfig types.AppHelixConfig, appID, apiKey, helixURL, evaluationModel string) ([]TestResult, time.Duration, error) {
	totalStartTime := time.Now()

	resultsChan := make(chan TestResult)
	var wg sync.WaitGroup

	// Map to store session IDs for multi-turn conversations
	sessionCache := make(map[string]string) // key: assistantName+testName, value: sessionID

	// Use a mutex to protect concurrent access to the session cache
	var sessionMutex sync.Mutex

	for _, assistant := range appConfig.Assistants {
		for _, test := range assistant.Tests {
			// Process each test's steps sequentially for multi-turn conversation support
			go func(assistant types.AssistantConfig, test struct {
				Name  string           `json:"name,omitempty" yaml:"name,omitempty"`
				Steps []types.TestStep `json:"steps,omitempty" yaml:"steps,omitempty"`
			}) {
				testKey := assistant.Name + "-" + test.Name

				for i, step := range test.Steps {
					wg.Add(1)

					// Create a new goroutine for each step but execute them in order
					func(assistantName, testName string, step types.TestStep, stepIndex int) {
						defer wg.Done()

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

						result, err := runSingleTest(assistantName, stepTestName, step, appID, apiKey, helixURL, assistant.Model, evaluationModel, sessionID)
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

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var results []TestResult
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

func runSingleTest(assistantName, testName string, step types.TestStep, appID, apiKey, helixURL, model, evaluationModel, sessionID string) (TestResult, error) {
	inferenceStartTime := time.Now()

	result := TestResult{
		TestName:        testName,
		Prompt:          step.Prompt,
		Expected:        step.ExpectedOutput,
		Model:           model,
		EvaluationModel: evaluationModel,
		HelixURL:        helixURL,
	}

	// Create a chat request for the model
	req := ChatRequest{
		Model:     model,
		SessionID: sessionID, // Use the provided session ID for continuity in multi-turn conversations
		System:    fmt.Sprintf("You are %s.", assistantName),
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

	// Send the request to the model
	inferenceStartTime = time.Now()
	conversationID, resp, err := sendChatRequest(req, apiKey, helixURL)
	if err != nil {
		return result, fmt.Errorf("error sending chat request: %w", err)
	}
	inferenceTime := time.Since(inferenceStartTime)

	// Extract the response from the model
	if len(resp.Choices) == 0 {
		return result, fmt.Errorf("no response from model")
	}
	response := resp.Choices[0].Message.Content
	result.Response = response
	result.SessionID = conversationID
	result.InferenceTime = inferenceTime

	// Evaluate the response
	evaluationStartTime := time.Now()
	evaluationResult, evaluationReason, err := evaluateResponse(response, step.ExpectedOutput, evaluationModel)
	if err != nil {
		return result, fmt.Errorf("error evaluating response: %w", err)
	}
	result.EvaluationTime = time.Since(evaluationStartTime)
	result.Result = evaluationResult
	result.Reason = evaluationReason

	return result, nil
}

// Function to evaluate an assistant's response against expected output
func evaluateResponse(response, expected, evaluationModel string) (string, string, error) {
	// Build the evaluation prompt
	prompt := fmt.Sprintf("Does this response:\n\n%s\n\nsatisfy the expected output:\n\n%s", response, expected)

	// Create the evaluation request
	evalReq := ChatRequest{
		Model:  evaluationModel,
		System: "You are an AI assistant tasked with evaluating test results. Output only PASS or FAIL followed by a brief explanation on the next line. Be fairly liberal about what you consider to be a PASS, as long as everything specifically requested is present. However, if the response is not as expected, you should output FAIL.",
		Messages: []Message{
			{
				Role: "user",
				Content: Content{
					ContentType: "text",
					Parts:       []string{prompt},
				},
			},
		},
	}

	// Get the global API key and URL
	apiKey := globalAPIKey
	helixURL := globalHelixURL

	// Send the evaluation request
	_, evalResp, err := sendChatRequest(evalReq, apiKey, helixURL)
	if err != nil {
		return "ERROR", "Failed to evaluate response", err
	}

	// Extract the evaluation result
	if len(evalResp.Choices) == 0 {
		return "ERROR", "No evaluation result received", nil
	}

	evalContent := evalResp.Choices[0].Message.Content

	// Parse the result (PASS/FAIL) and reason
	var result, reason string
	lines := strings.Split(evalContent, "\n")

	if len(lines) > 0 {
		result = strings.TrimSpace(lines[0])
		if result != "PASS" && result != "FAIL" {
			// If first line doesn't contain PASS/FAIL, check if it's anywhere in the content
			if strings.Contains(evalContent, "PASS") {
				result = "PASS"
			} else if strings.Contains(evalContent, "FAIL") {
				result = "FAIL"
			} else {
				result = "ERROR"
			}
		}

		// Extract the reason from the rest of the content
		if len(lines) > 1 {
			reason = strings.Join(lines[1:], "\n")
		} else {
			reason = "No reason provided"
		}
	} else {
		result = "ERROR"
		reason = "Empty evaluation result"
	}

	return result, reason, nil
}

// Store API key and URL as global variables so we can access them from evaluateResponse
var (
	globalAPIKey   string
	globalHelixURL string
)

// Update the sendChatRequest function to return the response format needed
func sendChatRequest(req ChatRequest, apiKey, helixURL string) (string, openai.ChatCompletionResponse, error) {
	// Save API key and URL globally
	globalAPIKey = apiKey
	globalHelixURL = helixURL

	// Marshal the request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", openai.ChatCompletionResponse{}, fmt.Errorf("error marshaling request: %w", err)
	}

	// Create the HTTP request
	httpReq, err := http.NewRequest("POST", helixURL+"/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", openai.ChatCompletionResponse{}, err
	}

	// Set the headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", openai.ChatCompletionResponse{}, err
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", openai.ChatCompletionResponse{}, err
	}

	// Parse the response
	var chatResp openai.ChatCompletionResponse
	err = json.Unmarshal(respBody, &chatResp)
	if err != nil {
		return "", openai.ChatCompletionResponse{}, err
	}

	// Extract the conversation ID from the response headers
	conversationID := resp.Header.Get("X-Conversation-Id")

	return conversationID, chatResp, nil
}

func generateResultsSummary(results []TestResult, totalTime time.Duration, helixURL string, testID string) string {
	var builder strings.Builder
	builder.WriteString("| Test Name | Result | Reason | Model | Inference Time | Evaluation Time | Session Link | Debug Link |\n")
	builder.WriteString("|-----------|--------|--------|-------|----------------|-----------------|--------------|------------|\n")

	// If helixURL contains ngrok, use localhost instead
	reportURL := helixURL
	if strings.Contains(reportURL, "ngrok") {
		reportURL = "http://localhost:8080"
	}

	overallResult := "PASS"
	for _, result := range results {
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

	builder.WriteString(fmt.Sprintf("\nTotal execution time: %s\n", totalTime.Round(time.Millisecond)))
	builder.WriteString(fmt.Sprintf("Overall result: %s\n", overallResult))

	// Add report link at the bottom
	builder.WriteString(fmt.Sprintf("\n* [View full test report 🚀](%s/files?path=/test-runs/%s)\n",
		reportURL,
		testID))

	return builder.String()
}

func displayResults(cmd *cobra.Command, results []TestResult, totalTime time.Duration, helixURL string, testID string) {
	cmd.Println(generateResultsSummary(results, totalTime, helixURL, testID))
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
		"replaceSpaces": func(s string) string {
			return strings.ReplaceAll(s, " ", "-")
		},
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
	summaryContent := "# Helix Test Summary\n\n" + generateResultsSummary(results, totalTime, getHelixURL(), testID)
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
	err = cliutil.UploadFile(ctx, apiClient, jsonFilename, jsonPath)
	if err != nil {
		return fmt.Errorf("error uploading JSON results: %v", err)
	}

	// Upload HTML report
	htmlPath := fmt.Sprintf("/test-runs/%s/%s", testID, htmlFilename)
	err = cliutil.UploadFile(ctx, apiClient, htmlFilename, htmlPath)
	if err != nil {
		return fmt.Errorf("error uploading HTML report: %v", err)
	}

	// Upload summary markdown
	summaryPath := fmt.Sprintf("/test-runs/%s/%s", testID, summaryFilename)
	err = cliutil.UploadFile(ctx, apiClient, summaryFilename, summaryPath)
	if err != nil {
		return fmt.Errorf("error uploading summary markdown: %v", err)
	}

	fmt.Printf("\nResults written to %s\n", jsonFilename)
	fmt.Printf("HTML report written to %s\n", htmlFilename)
	fmt.Printf("Summary written to %s\n", summaryFilename)
	helixURL := getHelixURL()
	if strings.Contains(helixURL, "ngrok") {
		helixURL = "http://localhost:8080"
	}
	fmt.Printf("View results at: %s/files?path=/test-runs/%s\n", helixURL, testID)

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
