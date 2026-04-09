package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

func isGoogleProvider(baseURL string) bool {
	return strings.Contains(baseURL, "generativelanguage.googleapis.com")
}

type ListGoogleModelsResponse struct {
	Models        []GoogleModel `json:"models"`
	NextPageToken string        `json:"nextPageToken"`
}

type GoogleModel struct {
	Name                       string   `json:"name"`
	Version                    string   `json:"version"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Temperature                float64  `json:"temperature"`
	TopP                       float64  `json:"topP"`
	TopK                       int      `json:"topK"`
	MaxTemperature             float64  `json:"maxTemperature"`
}

// Google models are served on https://generativelanguage.googleapis.com/v1beta/models
func (c *RetryableClient) listGoogleModels(ctx context.Context) ([]types.OpenAIModel, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + c.apiKey

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to provider's models endpoint: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to provider's models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get models from '%s' provider: %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from provider's models endpoint: %w", err)
	}

	var response ListGoogleModelsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint: %w", err)
	}
	// TODO: add paginations

	var openaiModels []types.OpenAIModel
	for _, model := range response.Models {
		if !isGenerativeGoogleModel(&model) {
			continue
		}

		openaiModels = append(openaiModels, types.OpenAIModel{
			ID:            strings.TrimPrefix(model.Name, "models/"),
			Description:   model.Description,
			Type:          "chat",
			ContextLength: model.InputTokenLimit,
		})
	}

	return openaiModels, nil
}

func isGenerativeGoogleModel(model *GoogleModel) bool {
	return slices.Contains(model.SupportedGenerationMethods, "generateContent")
}

// globalThoughtSigCache is a package-level cache for Gemini thought signatures.
// It must be global because different RetryableClient instances (e.g. reasoning
// vs generation models) may handle different turns of the same conversation.
// Call IDs are UUIDs so there's no collision risk across conversations.
var globalThoughtSigCache = &thoughtSignatureCache{
	store: make(map[string][]byte),
}

// thoughtSignatureCache stores thought signatures from Gemini responses so they
// can be echoed back when the conversation continues with tool results.
// The native genai SDK includes ThoughtSignature on Part, but since we convert
// through OpenAI types (which strip them), we need to preserve them separately.
type thoughtSignatureCache struct {
	mu    sync.RWMutex
	store map[string][]byte // FunctionCall.ID -> ThoughtSignature bytes
}

func (c *thoughtSignatureCache) Set(id string, sig []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[id] = sig
}

func (c *thoughtSignatureCache) Get(id string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.store[id]
	return v, ok
}

// createGoogleChatCompletion uses the native genai SDK for chat completions.
func (c *RetryableClient) createGoogleChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("failed to create genai client: %w", err)
	}

	contents, config := openaiToGenai(request, globalThoughtSigCache)

	resp, err := client.Models.GenerateContent(ctx, request.Model, contents, config)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("genai GenerateContent error: %w", err)
	}

	// Store thought signatures from the response for future requests
	assignIDsAndStoreSignatures(resp, globalThoughtSigCache)

	return genaiToOpenaiResponse(resp, request.Model), nil
}

// createGoogleChatCompletionStream uses the native genai SDK for streaming.
// It returns an openai.ChatCompletionStream by writing SSE chunks into a pipe,
// following the same pattern as helix_openai_client.go.
func (c *RetryableClient) createGoogleChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	contents, config := openaiToGenai(request, globalThoughtSigCache)

	// Create a pipe to feed the openai stream adapter
	pr, pw := io.Pipe()
	ht := &helixTransport{reader: pr, writer: pw}

	oaiConfig := openai.DefaultConfig("genai-google")
	oaiConfig.HTTPClient = &http.Client{Transport: ht}
	oaiClient := openai.NewClientWithConfig(oaiConfig)

	// Start consuming the genai stream in a goroutine
	go func() {
		defer pw.Close()

		chunkIndex := 0
		iter := client.Models.GenerateContentStream(ctx, request.Model, contents, config)

		for resp, err := range iter {
			if err != nil {
				log.Error().Err(err).Msg("genai stream error")
				// Write an error as the final chunk so the openai stream surfaces it
				break
			}

			// Store thought signatures from streaming chunks
			assignIDsAndStoreSignatures(resp, globalThoughtSigCache)

			chunk := genaiToOpenaiStreamChunk(resp, request.Model, chunkIndex)
			chunkIndex++

			bts, marshalErr := json.Marshal(chunk)
			if marshalErr != nil {
				log.Error().Err(marshalErr).Msg("failed to marshal stream chunk")
				break
			}

			if writeErr := writeChunk(pw, bts); writeErr != nil {
				log.Error().Err(writeErr).Msg("failed to write stream chunk")
				break
			}
		}
	}()

	stream, err := oaiClient.CreateChatCompletionStream(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create openai stream adapter: %w", err)
	}

	return stream, nil
}

// openaiToGenai converts an OpenAI request into genai Contents and config.
func openaiToGenai(req openai.ChatCompletionRequest, sigCache *thoughtSignatureCache) ([]*genai.Content, *genai.GenerateContentConfig) {
	config := &genai.GenerateContentConfig{}

	// Temperature
	if req.Temperature != 0 {
		t := float32(req.Temperature)
		config.Temperature = &t
	}

	// TopP
	if req.TopP != 0 {
		p := float32(req.TopP)
		config.TopP = &p
	}

	// MaxTokens
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	if req.MaxCompletionTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxCompletionTokens)
	}

	// Stop sequences
	if len(req.Stop) > 0 {
		config.StopSequences = req.Stop
	}

	// Tools
	if len(req.Tools) > 0 {
		config.Tools = openaiToGenaiTools(req.Tools)
	}

	// Tool choice
	if req.ToolChoice != nil {
		config.ToolConfig = openaiToGenaiToolConfig(req.ToolChoice)
	}

	// Build a map of tool call ID → function name from assistant messages,
	// since OpenAI tool-role messages only carry ToolCallID, not the function name.
	toolCallNames := make(map[string]string)
	for _, msg := range req.Messages {
		for _, tc := range msg.ToolCalls {
			toolCallNames[tc.ID] = tc.Function.Name
		}
	}

	// Convert messages
	var contents []*genai.Content
	for _, msg := range req.Messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			// System messages become system instruction
			config.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(msg.Content)},
			}

		case openai.ChatMessageRoleUser:
			content := &genai.Content{Role: "user"}
			if msg.Content != "" {
				content.Parts = append(content.Parts, genai.NewPartFromText(msg.Content))
			}
			for _, part := range msg.MultiContent {
				switch part.Type {
				case openai.ChatMessagePartTypeText:
					content.Parts = append(content.Parts, genai.NewPartFromText(part.Text))
				case openai.ChatMessagePartTypeImageURL:
					if part.ImageURL != nil {
						content.Parts = append(content.Parts, &genai.Part{
							FileData: &genai.FileData{
								FileURI:  part.ImageURL.URL,
								MIMEType: "image/jpeg",
							},
						})
					}
				}
			}
			contents = append(contents, content)

		case openai.ChatMessageRoleAssistant:
			content := &genai.Content{Role: "model"}
			if msg.Content != "" {
				content.Parts = append(content.Parts, genai.NewPartFromText(msg.Content))
			}
			// Convert tool calls to FunctionCall parts
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
				part := &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   tc.ID,
						Name: tc.Function.Name,
						Args: args,
					},
				}
				// Restore thought signature if we have one cached for this tool call
				if sigCache != nil {
					if sig, ok := sigCache.Get(tc.ID); ok {
						part.ThoughtSignature = sig
					}
				}
				content.Parts = append(content.Parts, part)
			}
			contents = append(contents, content)

		case openai.ChatMessageRoleTool:
			// Tool results become FunctionResponse parts
			var response map[string]any
			if err := json.Unmarshal([]byte(msg.Content), &response); err != nil {
				// If content isn't valid JSON, wrap it
				response = map[string]any{"result": msg.Content}
			}
			// Resolve function name: prefer msg.Name, fall back to lookup from assistant tool calls
			funcName := msg.Name
			if funcName == "" {
				funcName = toolCallNames[msg.ToolCallID]
			}
			content := &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							ID:       msg.ToolCallID,
							Name:     funcName,
							Response: response,
						},
					},
				},
			}
			contents = append(contents, content)
		}
	}

	// Merge consecutive same-role contents (Gemini requires alternating roles)
	contents = mergeConsecutiveContents(contents)

	return contents, config
}

// mergeConsecutiveContents merges consecutive Content entries with the same role,
// which is required by the Gemini API.
func mergeConsecutiveContents(contents []*genai.Content) []*genai.Content {
	if len(contents) <= 1 {
		return contents
	}

	var merged []*genai.Content
	current := contents[0]

	for i := 1; i < len(contents); i++ {
		if contents[i].Role == current.Role {
			current.Parts = append(current.Parts, contents[i].Parts...)
		} else {
			merged = append(merged, current)
			current = contents[i]
		}
	}
	merged = append(merged, current)

	return merged
}

// openaiToGenaiTools converts OpenAI tool definitions to genai tools.
func openaiToGenaiTools(tools []openai.Tool) []*genai.Tool {
	var funcDecls []*genai.FunctionDeclaration
	for _, tool := range tools {
		if tool.Type != openai.ToolTypeFunction || tool.Function == nil {
			continue
		}
		fd := &genai.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}
		// Use ParametersJsonSchema which accepts raw JSON schema directly
		if tool.Function.Parameters != nil {
			fd.ParametersJsonSchema = tool.Function.Parameters
		}
		funcDecls = append(funcDecls, fd)
	}
	if len(funcDecls) == 0 {
		return nil
	}
	return []*genai.Tool{{FunctionDeclarations: funcDecls}}
}

// openaiToGenaiToolConfig converts OpenAI tool_choice to genai ToolConfig.
func openaiToGenaiToolConfig(toolChoice any) *genai.ToolConfig {
	switch v := toolChoice.(type) {
	case string:
		switch v {
		case "auto":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAuto,
				},
			}
		case "none":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeNone,
				},
			}
		case "required":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAny,
				},
			}
		}
	}
	return nil
}

// assignIDsAndStoreSignatures generates synthetic IDs for FunctionCall parts
// that lack them (Gemini native API doesn't use IDs) and caches any thought
// signatures keyed by those IDs. This must be called before converting the
// response to OpenAI types.
func assignIDsAndStoreSignatures(resp *genai.GenerateContentResponse, cache *thoughtSignatureCache) {
	if resp == nil || cache == nil {
		return
	}
	for _, candidate := range resp.Candidates {
		if candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall == nil {
				continue
			}
			// Generate a stable ID if the API didn't provide one
			if part.FunctionCall.ID == "" {
				part.FunctionCall.ID = "call_" + uuid.New().String()[:8]
			}
			if len(part.ThoughtSignature) > 0 {
				log.Debug().
					Str("tool_call_id", part.FunctionCall.ID).
					Msg("Cached thought signature from genai response")
				cache.Set(part.FunctionCall.ID, part.ThoughtSignature)
			}
		}
	}
}

// genaiToOpenaiResponse converts a genai response to an OpenAI response.
func genaiToOpenaiResponse(resp *genai.GenerateContentResponse, model string) openai.ChatCompletionResponse {
	result := openai.ChatCompletionResponse{
		Model: model,
	}

	if resp.ResponseID != "" {
		result.ID = resp.ResponseID
	}

	if resp.UsageMetadata != nil {
		result.Usage = openai.Usage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
	}

	for i, candidate := range resp.Candidates {
		choice := openai.ChatCompletionChoice{
			Index: i,
			Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
			},
			FinishReason: genaiToOpenaiFinishReason(candidate.FinishReason),
		}

		if candidate.Content != nil {
			var textParts []string
			for _, part := range candidate.Content.Parts {
				if part.Thought {
					// Skip thought parts in the output (they're internal reasoning)
					continue
				}
				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					choice.Message.ToolCalls = append(choice.Message.ToolCalls, openai.ToolCall{
						ID:   part.FunctionCall.ID,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}
			if len(textParts) > 0 {
				choice.Message.Content = strings.Join(textParts, "")
			}
		}

		result.Choices = append(result.Choices, choice)
	}

	return result
}

// genaiToOpenaiStreamChunk converts a genai streaming response to an OpenAI stream chunk.
func genaiToOpenaiStreamChunk(resp *genai.GenerateContentResponse, model string, index int) openai.ChatCompletionStreamResponse {
	chunk := openai.ChatCompletionStreamResponse{
		Model: model,
	}

	if resp.ResponseID != "" {
		chunk.ID = resp.ResponseID
	}

	if resp.UsageMetadata != nil {
		chunk.Usage = &openai.Usage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
	}

	for i, candidate := range resp.Candidates {
		delta := openai.ChatCompletionStreamChoiceDelta{
			Role: openai.ChatMessageRoleAssistant,
		}

		var finishReason openai.FinishReason
		if candidate.FinishReason != "" {
			finishReason = genaiToOpenaiFinishReason(candidate.FinishReason)
		}

		if candidate.Content != nil {
			var textParts []string
			for _, part := range candidate.Content.Parts {
				if part.Thought {
					continue
				}
				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					delta.ToolCalls = append(delta.ToolCalls, openai.ToolCall{
						Index: genai.Ptr(i),
						ID:    part.FunctionCall.ID,
						Type:  openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}
			if len(textParts) > 0 {
				delta.Content = strings.Join(textParts, "")
			}
		}

		chunk.Choices = append(chunk.Choices, openai.ChatCompletionStreamChoice{
			Index:        i,
			Delta:        delta,
			FinishReason: finishReason,
		})
	}

	return chunk
}

func genaiToOpenaiFinishReason(reason genai.FinishReason) openai.FinishReason {
	switch reason {
	case genai.FinishReasonStop:
		return openai.FinishReasonStop
	case genai.FinishReasonMaxTokens:
		return openai.FinishReasonLength
	case genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent, genai.FinishReasonSPII:
		return openai.FinishReasonContentFilter
	case "":
		return ""
	default:
		return openai.FinishReasonStop
	}
}
