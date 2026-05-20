package model

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/types"
)

//go:embed model_info.json
var modelInfo embed.FS

//go:generate mockgen -source $GOFILE -destination model_info_mocks.go -package $GOPACKAGE

type ModelInfoProvider interface { //nolint:revive
	GetModelInfo(ctx context.Context, request *ModelInfoRequest) (*types.ModelInfo, error)
}

type ModelInfoRequest struct { //nolint:revive
	BaseURL  string
	Provider string
	Model    string
}

type BaseModelInfoProvider struct {
	dataMu     *sync.RWMutex
	data       map[string]types.ModelInfo // Keyed by provider_model_id
	normalized map[string]types.ModelInfo // Keyed by normalizeModelID(provider_model_id)
	providers  map[string]string          // Keyed by provider base URL
}

func NewBaseModelInfoProvider() (*BaseModelInfoProvider, error) {
	jsonFile, err := modelInfo.ReadFile("model_info.json")
	if err != nil {
		return nil, err
	}

	var response ModelInfoResponse
	err = json.Unmarshal(jsonFile, &response)
	if err != nil {
		return nil, err
	}

	providers := make(map[string]string)

	// Preserve last-write-wins for the primary `data` map (matches
	// pre-change behaviour) and build a parallel `normalized` index
	// so requests for plain ids resolve to Bedrock-prefixed entries
	// (e.g. claude-sonnet-4-6 -> eu.anthropic.claude-sonnet-4-6).
	// On collision in the normalized index, prefer the entry whose
	// upstream provider slug is "anthropic" - that's the direct API
	// route, not a regional Bedrock/Vertex variant.
	data := make(map[string]types.ModelInfo, len(response.Data))
	normalized := make(map[string]types.ModelInfo)
	normalizedIsDirect := make(map[string]bool)
	for _, m := range response.Data {
		pmid := m.Endpoint.ProviderModelID
		if pmid == "" {
			continue
		}
		providers[m.Endpoint.ProviderInfo.BaseURL] = m.Endpoint.ProviderInfo.Slug
		mi := toModelInfo(m)
		data[pmid] = mi
		n := normalizeModelID(pmid)
		if n == "" || n == pmid {
			continue
		}
		isDirect := isAnthropicDirectSlug(m.Endpoint.ProviderInfo.Slug)
		if _, exists := normalized[n]; !exists || (isDirect && !normalizedIsDirect[n]) {
			normalized[n] = mi
			normalizedIsDirect[n] = isDirect
		}
	}

	return &BaseModelInfoProvider{
		dataMu:     &sync.RWMutex{},
		data:       data,
		normalized: normalized,
		providers:  providers,
	}, nil
}

func toModelInfo(m ModelInfoData) types.ModelInfo {
	return types.ModelInfo{
		ProviderSlug:        m.Endpoint.ProviderSlug,
		ProviderModelID:     m.Endpoint.ProviderModelID,
		Name:                m.Name,
		Slug:                m.Slug,
		Permaslug:           m.Permaslug,
		Author:              m.Author,
		Description:         m.Description,
		InputModalities:     m.InputModalities,
		OutputModalities:    m.OutputModalities,
		SupportsReasoning:   m.Endpoint.SupportsReasoning,
		ContextLength:       m.ContextLength,
		SupportedParameters: m.Endpoint.SupportedParameters,
		MaxCompletionTokens: m.Endpoint.MaxCompletionTokens,
		Pricing:             m.Endpoint.Pricing,
	}
}

func isAnthropicDirectSlug(s string) bool {
	return s == "anthropic"
}

func (p *BaseModelInfoProvider) GetModelInfo(_ context.Context, request *ModelInfoRequest) (*types.ModelInfo, error) {
	p.dataMu.RLock()
	defer p.dataMu.RUnlock()

	modelName := request.Model

	// Try to get directly
	modelInfo, ok := p.data[modelName]
	if ok {
		return &modelInfo, nil
	}

	// If it has "<prefix>/" strip it as we will be looking up by model name
	if strings.Contains(modelName, "/") {
		// Strip the prefix
		parts := strings.SplitN(modelName, "/", 2)
		modelName = parts[1]
	}

	// Try again
	modelInfo, ok = p.data[modelName]
	if ok {
		return &modelInfo, nil
	}

	provider, ok := p.getProvider(request.BaseURL)
	if !ok {
		provider = request.Provider
	}

	slug := fmt.Sprintf("%s/%s", provider, modelName)

	var trimmedSlug string

	if provider == "anthropic" {
		trimmedSlug = trimAnthropicDateSuffix(slug)
	}

	for _, model := range p.data {
		if model.Name == modelName {
			return &model, nil
		}

		if model.Slug == slug || model.Permaslug == slug {
			return &model, nil
		}

		if trimmedSlug != "" && (model.Slug == trimmedSlug || model.Permaslug == trimmedSlug) {
			return &model, nil
		}
	}

	// Last resort: normalize the request id and consult the normalized
	// index. Catches plain ids that match a Bedrock-prefixed entry
	// (e.g. claude-sonnet-4-6 -> eu.anthropic.claude-sonnet-4-6), and
	// vice versa, plus Vertex date syntax (X@YYYYMMDD -> X-YYYYMMDD).
	if n := normalizeModelID(modelName); n != "" {
		if mi, ok := p.normalized[n]; ok {
			return &mi, nil
		}
	}

	return nil, fmt.Errorf("model info not found for model: %s (%s)", modelName, slug)
}

// normalizeModelID strips region prefixes, Bedrock version suffixes, and
// Vertex date syntax so equivalent ids from different providers map to the
// same key. The function is intentionally conservative — only known prefix
// shapes are stripped, so it won't shadow unrelated ids.
func normalizeModelID(id string) string {
	s := strings.ToLower(id)
	// Bedrock region prefixes: us./eu./apac./global./ap./ca. + vendor.
	regions := []string{"us.", "eu.", "apac.", "global.", "ap.", "ca."}
	vendors := []string{"anthropic.", "amazon.", "cohere.", "meta.", "mistral.", "ai21."}
	for _, r := range regions {
		for _, v := range vendors {
			if strings.HasPrefix(s, r+v) {
				s = s[len(r)+len(v):]
				break
			}
		}
	}
	// Bedrock version suffix: "-v1:0" first (longest), then "-v1", then ":0".
	s = strings.TrimSuffix(s, "-v1:0")
	s = strings.TrimSuffix(s, "-v1")
	s = strings.TrimSuffix(s, ":0")
	// Vertex date syntax: replace "@YYYYMMDD" with "-YYYYMMDD".
	if i := strings.Index(s, "@"); i >= 0 {
		suf := s[i+1:]
		if len(suf) == 8 && allDigits(suf) {
			s = s[:i] + "-" + suf
		}
	}
	return s
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (p *BaseModelInfoProvider) getProvider(baseURL string) (string, bool) {
	if baseURL == "" {
		return "", false
	}

	provider, ok := p.providers[baseURL]
	if ok {
		return provider, true
	}

	// If it's google, remove the /openai suffix
	baseURL = strings.TrimSuffix(baseURL, "/openai")

	provider, ok = p.providers[baseURL]
	if ok {
		return provider, true
	}

	// If provider doesn't have /v1 suffix, add it
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = fmt.Sprintf("%s/v1", baseURL)
	}

	provider, ok = p.providers[baseURL]
	if ok {
		return provider, true
	}

	return "", false
}

var anthropicDateSuffixRe = regexp.MustCompile(`-\d{8}$`)

func trimAnthropicDateSuffix(slug string) string {
	return anthropicDateSuffixRe.ReplaceAllString(slug, "")
}
