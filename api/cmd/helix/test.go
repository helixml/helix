package helix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type HelixYaml struct {
	Assistants []struct {
		Model string `yaml:"model"`
	} `yaml:"assistants"`
	Tests []struct {
		Name  string `yaml:"name"`
		Steps []struct {
			Prompt         string `yaml:"prompt"`
			ExpectedOutput string `yaml:"expected_output"`
		} `yaml:"steps"`
	} `yaml:"tests"`
}

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
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run tests for Helix app",
		Long:  `This command runs tests defined in helix.yaml and evaluates the results.`,
		RunE:  runTest,
	}

	return cmd
}

func runTest(cmd *cobra.Command, args []string) error {
	helixYaml, helixYamlContent, err := readHelixYaml()
	if err != nil {
		return err
	}

	appID, apiKey, helixURL, err := getEnvironmentVariables()
	if err != nil {
		return err
	}

	results, totalTime, err := runTests(helixYaml, appID, apiKey, helixURL)
	if err != nil {
		return err
	}

	displayResults(cmd, results, totalTime, helixURL)

	return writeResultsToFile(results, totalTime, helixYamlContent)
}

func readHelixYaml() (HelixYaml, string, error) {
	yamlFile, err := os.ReadFile("helix.yaml")
	if err != nil {
		return HelixYaml{}, "", fmt.Errorf("error reading helix.yaml: %v", err)
	}

	helixYamlContent := string(yamlFile)

	var helixYaml HelixYaml
	err = yaml.Unmarshal(yamlFile, &helixYaml)
	if err != nil {
		return HelixYaml{}, "", fmt.Errorf("error parsing helix.yaml: %v", err)
	}

	return helixYaml, helixYamlContent, nil
}

func getEnvironmentVariables() (string, string, string, error) {
	appID := os.Getenv("HELIX_APP_ID")
	if appID == "" {
		return "", "", "", fmt.Errorf("HELIX_APP_ID environment variable not set")
	}

	apiKey := os.Getenv("HELIX_API_KEY")
	if apiKey == "" {
		return "", "", "", fmt.Errorf("HELIX_API_KEY environment variable not set")
	}

	helixURL := os.Getenv("HELIX_URL")
	if helixURL == "" {
		return "", "", "", fmt.Errorf("HELIX_URL environment variable not set")
	}

	return appID, apiKey, helixURL, nil
}

func runTests(helixYaml HelixYaml, appID, apiKey, helixURL string) ([]TestResult, time.Duration, error) {
	var results []TestResult
	totalStartTime := time.Now()

	resultsChan := make(chan TestResult)
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for _, test := range helixYaml.Tests {
		for _, step := range test.Steps {
			wg.Add(1)
			go func(test string, step struct {
				Prompt         string `yaml:"prompt"`
				ExpectedOutput string `yaml:"expected_output"`
			}) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				result, err := runSingleTest(test, step, appID, apiKey, helixURL, helixYaml.Assistants[0].Model)
				if err != nil {
					fmt.Printf("Error running test %s: %v\n", test, err)
					return
				}

				resultsChan <- result
			}(test.Name, step)
		}
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].TestName < results[j].TestName
	})

	totalTime := time.Since(totalStartTime)

	return results, totalTime, nil
}

func runSingleTest(testName string, step struct {
	Prompt         string `yaml:"prompt"`
	ExpectedOutput string `yaml:"expected_output"`
}, appID, apiKey, helixURL, model string) (TestResult, error) {
	inferenceStartTime := time.Now()

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
		return TestResult{}, err
	}

	inferenceTime := time.Since(inferenceStartTime)

	evaluationStartTime := time.Now()

	evalReq := ChatRequest{
		Model:  "llama3.1:8b-instruct-q8_0",
		System: "You are an AI assistant tasked with evaluating test results. Output only PASS or FAIL followed by a brief explanation on the next line. Be liberal about what you consider to be a PASS, as long as everything specifically requested is present.",
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
		return TestResult{}, err
	}

	evaluationTime := time.Since(evaluationStartTime)

	return TestResult{
		TestName:       testName,
		Prompt:         step.Prompt,
		Response:       responseContent,
		Expected:       step.ExpectedOutput,
		Result:         evalContent[:4],
		Reason:         evalContent[5:],
		SessionID:      chatResp.ID,
		Model:          model,
		InferenceTime:  inferenceTime,
		EvaluationTime: evaluationTime,
	}, nil
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

func displayResults(cmd *cobra.Command, results []TestResult, totalTime time.Duration, helixURL string) {
	cmd.Println("| Test Name | Result | Reason | Model | Inference Time | Evaluation Time | Session Link | Debug Link |")
	cmd.Println("|-----------|--------|--------|-------|----------------|-----------------|--------------|------------|")
	for _, result := range results {
		sessionLink := fmt.Sprintf("%s/session/%s", helixURL, result.SessionID)
		debugLink := fmt.Sprintf("%s/dashboard?tab=llm_calls&filter_sessions=%s", helixURL, result.SessionID)
		cmd.Printf("| %-20s | %-6s | %-50s | %-25s | %-15s | %-15s | [Session](%s) | [Debug](%s) |\n",
			result.TestName,
			result.Result,
			result.Reason,
			result.Model,
			result.InferenceTime.Round(time.Millisecond),
			result.EvaluationTime.Round(time.Millisecond),
			sessionLink,
			debugLink)
	}

	cmd.Printf("\nTotal execution time: %s\n", totalTime.Round(time.Millisecond))

	overallResult := "PASS"
	for _, result := range results {
		if result.Result != "PASS" {
			overallResult = "FAIL"
			break
		}
	}
	cmd.Printf("Overall result: %s\n", overallResult)
}

func writeResultsToFile(results []TestResult, totalTime time.Duration, helixYamlContent string) error {
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("results_%s.json", timestamp)

	resultMap := map[string]interface{}{
		"tests":                results,
		"total_execution_time": totalTime.String(),
		"helix_yaml":           helixYamlContent,
	}

	jsonResults, err := json.MarshalIndent(resultMap, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling results to JSON: %v", err)
	}

	err = os.WriteFile(filename, jsonResults, 0644)
	if err != nil {
		return fmt.Errorf("error writing results to file: %v", err)
	}

	fmt.Printf("\nResults written to %s\n", filename)
	return nil
}
