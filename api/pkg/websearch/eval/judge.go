package eval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// judgePrompt is frozen: changing it changes every grade, which starts a new
// series (see design/websearch-eval/README.md).
const judgePrompt = `You grade web search results for relevance. You are given a search query and one result: its URL, title and the start of its page text. Grade how well this page serves someone who sent the query.

3 = exactly what was asked: directly answers the question, or is precisely the kind of page requested.
2 = relevant and useful, but not the best possible match (partial answer, broader page, older version).
1 = tangentially related: same topic area, but would not satisfy the query.
0 = irrelevant, spam, SEO filler, a login or cookie wall, an error page, or written in a different language from a query whose language is clear.

Judge the page on its own merits from the evidence shown. If the page text is missing, judge from the URL and title alone. Ignore page length: a short page that answers the query beats a long one that does not.
Reply with JSON: a reason of at most one sentence, then the grade.`

// judgeInputFormat lays out one page for the judge: query, URL, title, text.
const judgeInputFormat = "Query: %s\n\nURL: %s\nTitle: %s\nPage text:\n%s"

// judgeExcerptChars is how much page text the judge sees.
const judgeExcerptChars = 800

// The grade has no enum: Helix's OpenAI-compatible gateway accepts string
// enums only. Grade checks the range.
var judgeSchema = json.RawMessage(`{"type":"object","properties":{"reason":{"type":"string"},"grade":{"type":"integer"}},"required":["reason","grade"],"additionalProperties":false}`)

// PromptVersion fingerprints the judge prompt, input layout and output schema.
func PromptVersion() string {
	sum := sha256.Sum256([]byte(judgePrompt + judgeInputFormat + string(judgeSchema)))
	return hex.EncodeToString(sum[:6])
}

// Grader grades one pooled page for one query.
type Grader interface {
	Grade(ctx context.Context, q Query, d Doc) (Verdict, error)
}

type Verdict struct {
	Grade  int    `json:"grade"`
	Reason string `json:"reason"`
}

// LLMJudge grades through an OpenAI-compatible chat completions endpoint at
// temperature 0 with a structured output schema.
type LLMJudge struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

func NewLLMJudge(baseURL, apiKey, model string) (*LLMJudge, error) {
	if baseURL == "" || apiKey == "" || model == "" {
		return nil, errors.New("judge needs a base URL, an API key (JUDGE_API_KEY) and a model")
	}
	return &LLMJudge{
		url:    strings.TrimRight(baseURL, "/") + "/chat/completions",
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

// chatRequest is written by hand rather than with go-openai, whose
// omitempty drops temperature 0 and leaves the provider's default.
type chatRequest struct {
	Model          string        `json:"model"`
	Temperature    float64       `json:"temperature"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat any           `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (j *LLMJudge) Grade(ctx context.Context, q Query, d Doc) (Verdict, error) {
	body, err := json.Marshal(chatRequest{
		Model: j.model,
		// One user message: the glm-5.3-flash route on the dev stack ignores
		// system messages.
		Messages: []chatMessage{{Role: "user", Content: judgePrompt + "\n\n" + judgeInput(q, d)}},
		ResponseFormat: map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "relevance", "strict": true, "schema": judgeSchema},
		},
	})
	if err != nil {
		return Verdict{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.url, bytes.NewReader(body))
	if err != nil {
		return Verdict{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.apiKey)
	resp, err := j.client.Do(req)
	if err != nil {
		return Verdict{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Verdict{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Verdict{}, fmt.Errorf("judge returned %s: %s", resp.Status, respBody)
	}
	var completion struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return Verdict{}, fmt.Errorf("judge response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return Verdict{}, errors.New("judge returned no choices")
	}
	var v Verdict
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &v); err != nil {
		return Verdict{}, fmt.Errorf("judge returned invalid JSON %q: %w", content, err)
	}
	if v.Grade < 0 || v.Grade > 3 {
		return Verdict{}, fmt.Errorf("judge returned grade %d", v.Grade)
	}
	return v, nil
}

func judgeInput(q Query, d Doc) string {
	text := d.Text
	if r := []rune(text); len(r) > judgeExcerptChars {
		text = string(r[:judgeExcerptChars])
	}
	if strings.TrimSpace(text) == "" {
		text = "(page text unavailable)"
	}
	return fmt.Sprintf(judgeInputFormat, q.Query, d.URL, d.Title, text)
}

// Judgments is the judgment cache: one grade per (query, page), reused by
// every later run so rounds are graded identically and only new pages cost a
// judge call. It is bound to one judge model and prompt version.
type Judgments struct {
	JudgeModel    string     `json:"judgeModel"`
	PromptVersion string     `json:"promptVersion"`
	Items         []Judgment `json:"judgments"`

	mu    sync.Mutex
	index map[judgmentKey]int
}

type Judgment struct {
	QueryID string `json:"queryId"`
	Key     string `json:"key"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	// HadText records whether the judge saw page text; a page first judged
	// from its title alone is judged again once a run supplies its text.
	HadText bool `json:"hadText"`
	Verdict
}

type judgmentKey struct{ queryID, key string }

// LoadJudgments reads the cache at path, or starts an empty one when the file
// does not exist. A cache made by another judge model or prompt is an error.
func LoadJudgments(path, judgeModel string) (*Judgments, error) {
	j := &Judgments{JudgeModel: judgeModel, PromptVersion: PromptVersion()}
	if err := readJSON(path, j); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if j.JudgeModel != judgeModel || j.PromptVersion != PromptVersion() {
		return nil, fmt.Errorf("%s was graded by %s with prompt %s, not %s with prompt %s: start a new series",
			path, j.JudgeModel, j.PromptVersion, judgeModel, PromptVersion())
	}
	j.index = map[judgmentKey]int{}
	for i, it := range j.Items {
		j.index[judgmentKey{it.QueryID, it.Key}] = i
	}
	return j, nil
}

func (j *Judgments) Save(path string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	sort.Slice(j.Items, func(a, b int) bool {
		if j.Items[a].QueryID != j.Items[b].QueryID {
			return j.Items[a].QueryID < j.Items[b].QueryID
		}
		return j.Items[a].Key < j.Items[b].Key
	})
	for i, it := range j.Items {
		j.index[judgmentKey{it.QueryID, it.Key}] = i
	}
	return writeJSON(path, j)
}

// Lookup returns the judgment of a page for a query.
func (j *Judgments) Lookup(queryID, key string) (Judgment, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	i, ok := j.index[judgmentKey{queryID, key}]
	if !ok {
		return Judgment{}, false
	}
	return j.Items[i], true
}

func (j *Judgments) put(it Judgment) {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := judgmentKey{it.QueryID, it.Key}
	if i, ok := j.index[k]; ok {
		j.Items[i] = it
		return
	}
	j.index[k] = len(j.Items)
	j.Items = append(j.Items, it)
}

func (j *Judgments) needs(queryID string, d Doc) bool {
	have, ok := j.Lookup(queryID, d.Key)
	return !ok || (!have.HadText && d.Text != "")
}

// JudgeAll grades every pooled page the cache lacks and returns how many it
// graded. Pages graded before an error stay in the cache.
func JudgeAll(ctx context.Context, grader Grader, cache *Judgments, queries []Query, pools map[string][]Doc, concurrency int) (int, error) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	var (
		mu    sync.Mutex
		count int
	)
	for _, q := range queries {
		for _, d := range pools[q.ID] {
			if !cache.needs(q.ID, d) {
				continue
			}
			g.Go(func() error {
				v, err := grader.Grade(ctx, q, d)
				if err != nil {
					return fmt.Errorf("judge %s %s: %w", q.ID, d.URL, err)
				}
				cache.put(Judgment{QueryID: q.ID, Key: d.Key, URL: d.URL, Title: d.Title, HadText: d.Text != "", Verdict: v})
				mu.Lock()
				count++
				mu.Unlock()
				return nil
			})
		}
	}
	err := g.Wait()
	return count, err
}
