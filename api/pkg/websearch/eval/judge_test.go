package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestJudgments(t *testing.T, items ...Judgment) *Judgments {
	t.Helper()
	path := filepath.Join(t.TempDir(), JudgmentsFile)
	j, err := LoadJudgments(path, "judge-model")
	require.NoError(t, err)
	for _, it := range items {
		j.put(it)
	}
	return j
}

func TestPageKey(t *testing.T) {
	require.Equal(t, "example.com/a", PageKey("https://www.Example.com/a/"))
	require.Equal(t, "example.com/a", PageKey("http://example.com:80/a#frag"))
	require.Equal(t, "example.com/a?b=1", PageKey("https://example.com/a?b=1"))
	require.Equal(t, "example.com", PageKey("https://example.com"))
}

func TestPoolDedupsAndKeepsLongestText(t *testing.T) {
	helix := QueryRun{Hits: []Hit{
		{URL: "https://z.com/page", Title: "Z"},
		{URL: "https://www.a.com/", Title: "A short", Text: "short"},
	}}
	exa := QueryRun{Hits: []Hit{
		{URL: "https://a.com", Title: "A long", Text: "much longer text"},
		{URL: "https://z.com/page/", Title: "Z again"},
	}}
	got := Pool(helix, exa)
	require.Equal(t, []Doc{
		{Key: "a.com", URL: "https://a.com", Title: "A long", Text: "much longer text"},
		{Key: "z.com/page", URL: "https://z.com/page", Title: "Z"},
	}, got)
}

type fakeGrader struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeGrader) Grade(_ context.Context, q Query, d Doc) (Verdict, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, q.ID+" "+d.Key)
	return Verdict{Grade: 2, Reason: "ok"}, nil
}

func TestJudgeAllJudgesOnlyWhatTheCacheLacks(t *testing.T) {
	queries := []Query{{ID: "q1"}, {ID: "q2"}}
	cache := newTestJudgments(t,
		Judgment{QueryID: "q1", Key: "cached.com", HadText: true, Verdict: Verdict{Grade: 3}},
		Judgment{QueryID: "q1", Key: "titleonly.com", HadText: false, Verdict: Verdict{Grade: 0}},
		Judgment{QueryID: "q2", Key: "stilltitleonly.com", HadText: false, Verdict: Verdict{Grade: 1}},
	)
	pools := map[string][]Doc{
		"q1": {
			{Key: "cached.com", Text: "new text does not re-judge"},
			{Key: "titleonly.com", Text: "text arrived"},
			{Key: "new.com"},
		},
		// The same page under another query is a separate judgment.
		"q2": {{Key: "cached.com"}, {Key: "stilltitleonly.com"}},
	}
	grader := &fakeGrader{}
	n, err := JudgeAll(context.Background(), grader, cache, queries, pools, 2, noCheckpoint)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.ElementsMatch(t, []string{"q1 titleonly.com", "q1 new.com", "q2 cached.com"}, grader.calls)

	got, ok := cache.Lookup("q1", "titleonly.com")
	require.True(t, ok)
	require.Equal(t, 2, got.Grade)
	require.True(t, got.HadText)
	got, _ = cache.Lookup("q1", "cached.com")
	require.Equal(t, 3, got.Grade)

	n, err = JudgeAll(context.Background(), grader, cache, queries, pools, 2, noCheckpoint)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestJudgmentsRoundTripAndBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), JudgmentsFile)
	j, err := LoadJudgments(path, "judge-model")
	require.NoError(t, err)
	j.put(Judgment{QueryID: "q2", Key: "b.com", Verdict: Verdict{Grade: 1}})
	j.put(Judgment{QueryID: "q1", Key: "a.com", HadText: true, Verdict: Verdict{Grade: 3, Reason: "r"}})
	require.NoError(t, j.Save(path))

	loaded, err := LoadJudgments(path, "judge-model")
	require.NoError(t, err)
	require.Equal(t, "q1", loaded.Items[0].QueryID)
	got, ok := loaded.Lookup("q2", "b.com")
	require.True(t, ok)
	require.Equal(t, 1, got.Grade)

	_, err = LoadJudgments(path, "other-model")
	require.ErrorContains(t, err, "start a new series")
}

// The judge must not learn which provider returned a page.
func TestLLMJudgeRequestIsBlind(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		body, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"reason\":\"answers it\",\"grade\":3}"}}]}`)
	}))
	defer srv.Close()

	judge, err := NewLLMJudge(srv.URL+"/v1", "secret", "judge-model")
	require.NoError(t, err)
	q := Query{ID: "tech-01", Kind: "technical", Query: "configure rate limiting"}
	d := Doc{Key: "docs.test/rate", URL: "https://docs.test/rate", Title: "Rate limiting", Text: strings.Repeat("é", 900)}
	v, err := judge.Grade(context.Background(), q, d)
	require.NoError(t, err)
	require.Equal(t, Verdict{Grade: 3, Reason: "answers it"}, v)

	var req struct {
		Model       string   `json:"model"`
		Temperature *float64 `json:"temperature"`
		Messages    []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &req))
	require.Equal(t, "judge-model", req.Model)
	require.NotNil(t, req.Temperature, "temperature 0 must be sent, not omitted")
	require.Zero(t, *req.Temperature)
	// The request carries the frozen prompt and exactly the page evidence:
	// no provider, rank, query id or kind.
	require.Len(t, req.Messages, 1)
	want := judgePrompt + "\n\nQuery: configure rate limiting\n\nURL: https://docs.test/rate\nTitle: Rate limiting\nPage text:\n" + strings.Repeat("é", judgeExcerptChars)
	require.Equal(t, want, req.Messages[0].Content)
}

func TestLLMJudgeRejectsBadOutput(t *testing.T) {
	for name, content := range map[string]string{
		"not json":       "three",
		"grade too high": `{"reason":"x","grade":4}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
				_, _ = w.Write(b)
			}))
			defer srv.Close()
			judge, err := NewLLMJudge(srv.URL, "k", "m")
			require.NoError(t, err)
			_, err = judge.Grade(context.Background(), Query{Query: "q"}, Doc{URL: "https://x.test"})
			require.Error(t, err)
		})
	}
}

func noCheckpoint(int) error { return nil }

// A long judging run saves as it goes, so an interrupted run keeps the
// grades it paid for.
func TestJudgeAllCheckpoints(t *testing.T) {
	queries := []Query{{ID: "q", Query: "q"}}
	var pool []Doc
	for i := range 2*checkpointEvery + 7 {
		pool = append(pool, Doc{Key: fmt.Sprintf("k%d", i), URL: fmt.Sprintf("https://x.test/%d", i), Title: "t"})
	}
	var calls []int
	n, err := JudgeAll(context.Background(), &fakeGrader{}, newTestJudgments(t), queries, map[string][]Doc{"q": pool}, 4,
		func(graded int) error { calls = append(calls, graded); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != len(pool) || len(calls) != 2 || calls[0] != checkpointEvery || calls[1] != 2*checkpointEvery {
		t.Fatalf("graded %d, checkpoints %v", n, calls)
	}
}
