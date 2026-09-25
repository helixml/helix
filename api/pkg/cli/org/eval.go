package org

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// `helix org eval`: graded eval suites against bot instances. Each case runs on
// a fresh instance (instances copy the prompt at creation, so every run tests
// the current prompt); turns go through /sessions/chat and are graded on the
// final message the customer sees plus the tool calls of the turn.

type evalSuite struct {
	Name     string      `json:"name"`
	Bot      string      `json:"bot"`
	Runtime  string      `json:"runtime"`
	Timeout  int         `json:"timeout"`
	Judge    judgeConfig `json:"judge"`
	Defaults evalExpect  `json:"defaults"`
	Cases    []evalCase  `json:"cases"`
	base     string
}

type judgeConfig struct {
	App   string `json:"app"`
	Model string `json:"model"`
}

type evalCase struct {
	ID       string      `json:"id"`
	Question string      `json:"question"` // single-turn short form (questions.json compatible)
	Must     [][]string  `json:"must"`
	MustNot  []string    `json:"must_not"`
	Turns    []evalTurn  `json:"turns"`
	Files    []evalFile  `json:"files"`
	Simulate *simulation `json:"simulate"`
	Judge    string      `json:"judge"`
	Timeout  int         `json:"timeout"`
}

type evalTurn struct {
	User   string     `json:"user"`
	Attach []string   `json:"attach"`
	Expect evalExpect `json:"expect"`
}

type evalExpect struct {
	Must         [][]string `json:"must,omitempty"`
	MustNot      []string   `json:"must_not,omitempty"`
	Judge        string     `json:"judge,omitempty"`
	MaxSeconds   float64    `json:"max_seconds,omitempty"`
	MaxToolCalls *int       `json:"max_tool_calls,omitempty"`
	ToolsRequire []string   `json:"tools_require,omitempty"`
	ToolsForbid  []string   `json:"tools_forbid,omitempty"`
}

type evalFile struct {
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

type simulation struct {
	Persona  string            `json:"persona"`
	Goal     string            `json:"goal"`
	Facts    map[string]string `json:"facts"`
	Opening  string            `json:"opening"`
	MaxTurns int               `json:"max_turns"`
}

type turnRecord struct {
	User        string          `json:"user"`
	Reply       string          `json:"reply"`
	Seconds     float64         `json:"seconds"`
	ToolCalls   int             `json:"tool_calls"`
	Tools       map[string]int  `json:"tools"`
	Checks      map[string]bool `json:"checks"`
	Failed      []string        `json:"failed,omitempty"`
	JudgeReason string          `json:"judge_reason,omitempty"`
	State       string          `json:"state"`
	Pass        bool            `json:"pass"`
}

type caseRecord struct {
	Tag     string       `json:"tag"`
	Suite   string       `json:"suite"`
	Bot     string       `json:"bot"`
	Case    string       `json:"case"`
	Pass    bool         `json:"pass"`
	Seconds float64      `json:"seconds"`
	Session string       `json:"session"`
	Kept    bool         `json:"kept,omitempty"`
	Error   string       `json:"error,omitempty"`
	Judge   *judgeResult `json:"judge,omitempty"`
	Turns   []turnRecord `json:"turns"`
	TS      string       `json:"ts"`
}

type judgeResult struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

func loadSuite(path string) (*evalSuite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	js, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, err
	}
	s := &evalSuite{}
	if strings.HasPrefix(strings.TrimSpace(string(js)), "[") { // bare questions.json
		if err := json.Unmarshal(js, &s.Cases); err != nil {
			return nil, err
		}
		s.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	} else if err := json.Unmarshal(js, s); err != nil {
		return nil, err
	}
	s.base = filepath.Dir(path)
	for i := range s.Cases {
		c := &s.Cases[i]
		if c.Question != "" && len(c.Turns) == 0 {
			c.Turns = []evalTurn{{User: c.Question, Expect: evalExpect{Must: c.Must, MustNot: c.MustNot}}}
		}
		for j := range c.Turns {
			c.Turns[j].Expect = mergeExpect(s.Defaults, c.Turns[j].Expect)
		}
	}
	return s, nil
}

func mergeExpect(d, e evalExpect) evalExpect {
	if e.Must == nil {
		e.Must = d.Must
	}
	e.MustNot = append(append([]string{}, d.MustNot...), e.MustNot...)
	if e.Judge == "" {
		e.Judge = d.Judge
	}
	if e.MaxSeconds == 0 {
		e.MaxSeconds = d.MaxSeconds
	}
	if e.MaxToolCalls == nil {
		e.MaxToolCalls = d.MaxToolCalls
	}
	e.ToolsRequire = append(append([]string{}, d.ToolsRequire...), e.ToolsRequire...)
	e.ToolsForbid = append(append([]string{}, d.ToolsForbid...), e.ToolsForbid...)
	return e
}

// altMatch: case-insensitive substring, or "re:<regex>" on the lower-cased text.
func altMatch(alt, text string) bool {
	if strings.HasPrefix(alt, "re:") {
		re, err := regexp.Compile(alt[3:])
		return err == nil && re.MatchString(text)
	}
	return strings.Contains(text, strings.ToLower(alt))
}

func gradeTurn(e evalExpect, reply string, secs float64, calls []entry) (map[string]bool, []string) {
	text := strings.ReplaceAll(strings.ToLower(reply), " ", " ")
	checks := map[string]bool{}
	var failed []string
	if len(e.Must) > 0 {
		ok := true
		for _, g := range e.Must {
			hit := false
			for _, a := range g {
				if altMatch(a, text) {
					hit = true
					break
				}
			}
			if !hit {
				ok = false
				failed = append(failed, "missing one of "+strings.Join(g, " | "))
			}
		}
		checks["must"] = ok
	}
	if len(e.MustNot) > 0 {
		ok := true
		for _, a := range e.MustNot {
			if altMatch(a, text) {
				ok = false
				failed = append(failed, "contains "+a)
			}
		}
		checks["must_not"] = ok
	}
	if e.MaxSeconds > 0 {
		checks["max_seconds"] = secs <= e.MaxSeconds
	}
	if e.MaxToolCalls != nil {
		checks["max_tool_calls"] = len(calls) <= *e.MaxToolCalls
	}
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.ToolName)
	}
	anyName := func(sub string) bool {
		for _, n := range names {
			if strings.Contains(n, sub) {
				return true
			}
		}
		return false
	}
	for _, t := range e.ToolsRequire {
		checks["uses:"+t] = anyName(t)
	}
	for _, t := range e.ToolsForbid {
		checks["avoids:"+t] = !anyName(t)
	}
	return checks, failed
}

// llm talks to Helix's OpenAI-compatible endpoint scoped by a NEUTRAL judge app:
// ?app_id gives the call an org for billing, and the app's system prompt
// replaces any system message we send — so instructions go in the user message.
type llm struct {
	c     *httpClient
	app   string
	model string
}

func (l *llm) ok() bool { return l.app != "" }

func (l *llm) complete(ctx context.Context, instructions, input string, temperature float64) (string, error) {
	body := map[string]any{
		"messages":    []any{map[string]any{"role": "user", "content": instructions + "\n\n" + input}},
		"temperature": temperature,
	}
	if l.model != "" {
		body["model"] = l.model
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	base := strings.TrimSuffix(l.c.base, "/api/v1")
	cp := *l.c
	cp.base = base
	if err := cp.doJSON(ctx, http.MethodPost, "/v1/chat/completions?app_id="+l.app, body, &resp, 3*time.Minute); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("judge: empty response")
	}
	return stripThinking(resp.Choices[0].Message.Content), nil
}

var jsonObjRE = regexp.MustCompile(`(?s)\{.*\}`)
var passFailRE = regexp.MustCompile(`\b(PASS|FAIL)\b`)

func (l *llm) judge(ctx context.Context, rubric, transcript string) judgeResult {
	out, err := l.complete(ctx, "You grade a support chatbot transcript against a rubric. Be strict: pass only if every "+
		"requirement in the rubric is met by the BOT's messages. Reply with one JSON object "+
		`{"pass": true|false, "reason": "<one sentence>"} and nothing else.`,
		"RUBRIC:\n"+rubric+"\n\nTRANSCRIPT:\n"+transcript, 0)
	if err != nil {
		return judgeResult{Reason: "judge error: " + err.Error()}
	}
	var v struct {
		Pass   *bool  `json:"pass"`
		Reason string `json:"reason"`
	}
	if m := jsonObjRE.FindString(out); m != "" && json.Unmarshal([]byte(m), &v) == nil && v.Pass != nil {
		return judgeResult{Pass: *v.Pass, Reason: v.Reason}
	}
	if m := passFailRE.FindString(strings.ToUpper(out)); m != "" {
		return judgeResult{Pass: m == "PASS", Reason: truncate(out, 200)}
	}
	return judgeResult{Reason: "unparseable judge output: " + truncate(out, 200)}
}

func (l *llm) customer(ctx context.Context, s *simulation, transcript string) (string, error) {
	facts, _ := json.Marshal(s.Facts)
	return l.complete(ctx, fmt.Sprintf("You are role-playing a CUSTOMER talking to a support bot, to test it. Stay in character.\n"+
		"Persona: %s\nGoal: %s\nFacts you know (share only when asked): %s\n"+
		"Write ONLY your next message to the bot, short and natural. If the goal is reached, the bot is stuck or "+
		"looping, or it asks for something you do not have, reply exactly DONE.", s.Persona, s.Goal, facts),
		"Conversation so far:\n"+transcript+"\n\nYour next message:", 0.4)
}

func render(turns []turnRecord) string {
	var sb strings.Builder
	for _, t := range turns {
		fmt.Fprintf(&sb, "CUSTOMER: %s\nBOT: %s\n", t.User, t.Reply)
	}
	return sb.String()
}

type evalOpts struct {
	orgID, tag, runtime, out string
	keep, keepFailed         bool
	judge                    *llm
	mu                       sync.Mutex
}

func runCase(ctx context.Context, c *httpClient, s *evalSuite, bot string, ec evalCase, o *evalOpts) caseRecord {
	rec := caseRecord{Tag: o.tag, Suite: s.Name, Bot: bot, Case: ec.ID, TS: time.Now().Format(time.RFC3339)}
	t0 := time.Now()
	runtime := o.runtime
	if runtime == "" {
		runtime = s.Runtime
	}
	inst, err := c.createInstance(ctx, o.orgID, bot, truncate("eval "+o.tag+" "+ec.ID, 60), runtime, "")
	if err != nil {
		rec.Error = err.Error()
		return rec
	}
	sid := inst.SessionID
	rec.Session = sid
	defer func() {
		if o.keep || (o.keepFailed && !rec.Pass) {
			rec.Kept = true
			return
		}
		_ = c.deleteInstance(context.Background(), o.orgID, bot, sid)
	}()
	if err := waitSandbox(ctx, o.orgID, sid, 3*time.Minute); err != nil {
		rec.Error = err.Error()
		rec.Seconds = time.Since(t0).Seconds()
		return rec
	}
	fail := func(err error) caseRecord {
		rec.Error = truncate(err.Error(), 1000)
		rec.Seconds = time.Since(t0).Seconds()
		return rec
	}
	for _, f := range ec.Files {
		dest := f.Dest
		if !strings.HasPrefix(dest, "/") {
			dest = "/home/retro/work/" + dest
		}
		src := f.Src
		if !filepath.IsAbs(src) {
			src = filepath.Join(s.base, src)
		}
		if err := putFile(ctx, o.orgID, sid, src, dest); err != nil {
			return fail(err)
		}
	}
	timeout := ec.Timeout
	if timeout == 0 {
		timeout = s.Timeout
	}
	if timeout == 0 {
		timeout = 900
	}
	turns := ec.Turns
	if ec.Simulate != nil {
		opening := ec.Simulate.Opening
		if opening == "" {
			opening = "Hi"
		}
		turns = []evalTurn{{User: opening}}
	}
	for i := 0; i < len(turns); i++ {
		t := turns[i]
		var attach []string
		for _, a := range t.Attach {
			if !filepath.IsAbs(a) {
				a = filepath.Join(s.base, a)
			}
			attach = append(attach, a)
		}
		r, err := c.sendTurn(ctx, sid, t.User, attach, "", time.Duration(timeout)*time.Second)
		if err != nil {
			return fail(err)
		}
		tr := turnRecord{User: t.User, Reply: truncate(r.Reply, 4000), Seconds: r.Seconds, ToolCalls: len(r.ToolCalls), Tools: map[string]int{}}
		for _, tc := range r.ToolCalls {
			tr.Tools[tc.ToolName]++
		}
		if r.Interaction != nil {
			tr.State = string(r.Interaction.State)
		}
		tr.Checks, tr.Failed = gradeTurn(t.Expect, r.Reply, r.Seconds, r.ToolCalls)
		if t.Expect.Judge != "" && o.judge.ok() {
			j := o.judge.judge(ctx, t.Expect.Judge, render(append(rec.Turns, tr)))
			tr.Checks["judge"], tr.JudgeReason = j.Pass, j.Reason
		}
		tr.Pass = true
		for _, v := range tr.Checks {
			tr.Pass = tr.Pass && v
		}
		rec.Turns = append(rec.Turns, tr)
		if ec.Simulate != nil && o.judge.ok() {
			max := ec.Simulate.MaxTurns
			if max == 0 {
				max = 6
			}
			if len(rec.Turns) < max {
				next, err := o.judge.customer(ctx, ec.Simulate, render(rec.Turns))
				if err == nil && strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(next)), ".") != "DONE" {
					turns = append(turns, evalTurn{User: strings.TrimSpace(next)})
				}
			}
		}
	}
	rec.Pass = true
	for _, t := range rec.Turns {
		rec.Pass = rec.Pass && t.Pass
	}
	if ec.Judge != "" {
		if o.judge.ok() {
			j := o.judge.judge(ctx, ec.Judge, render(rec.Turns))
			rec.Judge = &j
			rec.Pass = rec.Pass && j.Pass
		} else {
			rec.Judge = &judgeResult{Reason: "no judge configured (--judge-app / $HELIX_EVAL_JUDGE_APP)"}
		}
	}
	rec.Seconds = time.Since(t0).Seconds()
	return rec
}

func putFile(ctx context.Context, orgID, sessionID, src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	sb, err := sandboxForSession(ctx, orgID, sessionID, 3*time.Minute)
	if err != nil {
		return err
	}
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return err
	}
	return apiClient.WriteSandboxFile(ctx, orgID, sb.ID, dest, data, 0)
}

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run graded eval suites against bot instances; report and compare runs",
		Long: `Run a suite (YAML/JSON) against one or more bots, one fresh instance per case.

Suite: {name, bot, runtime, timeout, judge:{app, model}, defaults:{…expect}, cases:[…]}
Case:  {id, question+must | turns:[{user, attach:[…], expect:{…}}], files:[{src, dest}],
        simulate:{persona, goal, facts, opening, max_turns}, judge: "<rubric>"}
Expect: must (AND of OR-groups; "re:" regex), must_not, max_seconds, max_tool_calls,
        tools_require, tools_forbid (tool-name substrings), judge (rubric).
The judge and simulated customer need a NEUTRAL judge app (never the bot's own app).

Examples:
  helix org eval run support.eval.yaml --parallel 3 --keep-failed
  helix org eval run questions.json --bot sup-a,sup-b --tag cmp1
  helix org eval report runs/cmp1.jsonl
  helix org eval compare runs/before.jsonl runs/after.jsonl`,
	}
	cmd.AddCommand(newEvalRunCmd(), newEvalReportCmd(), newEvalCompareCmd())
	return cmd
}

func newEvalRunCmd() *cobra.Command {
	var (
		orgFlag, bots, tag, out, only, runtime, judgeApp, judgeModel string
		parallel, repeat                                             int
		keep, keepFailed                                             bool
	)
	cmd := &cobra.Command{
		Use:   "run <suite>",
		Short: "Run a suite; one JSON line per case to runs/<tag>.jsonl",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSuite(args[0])
			if err != nil {
				return err
			}
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			orgID, err := c.resolveOrg(ctx, orgFlag)
			if err != nil {
				return err
			}
			if bots == "" {
				bots = s.Bot
			}
			if bots == "" {
				return fmt.Errorf(`no bot: set "bot" in the suite or pass --bot`)
			}
			if tag == "" {
				tag = s.Name + "-" + time.Now().Format("0102-1504")
			}
			if out == "" {
				out = filepath.Join("runs", tag+".jsonl")
			}
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			j := &llm{c: c, app: firstNonEmpty(judgeApp, s.Judge.App, os.Getenv("HELIX_EVAL_JUDGE_APP")),
				model: firstNonEmpty(judgeModel, s.Judge.Model, os.Getenv("HELIX_EVAL_JUDGE_MODEL"))}
			o := &evalOpts{orgID: orgID, tag: tag, runtime: runtime, out: out, keep: keep, keepFailed: keepFailed, judge: j}
			var cases []evalCase
			keepIDs := map[string]bool{}
			for _, id := range strings.Split(only, ",") {
				if id != "" {
					keepIDs[id] = true
				}
			}
			needsJudge := false
			for _, ec := range s.Cases {
				if len(keepIDs) == 0 || keepIDs[ec.ID] {
					cases = append(cases, ec)
					needsJudge = needsJudge || ec.Judge != "" || ec.Simulate != nil
					for _, t := range ec.Turns {
						needsJudge = needsJudge || t.Expect.Judge != ""
					}
				}
			}
			if needsJudge && !j.ok() {
				fmt.Fprintln(os.Stderr, "warning: suite uses judge/simulate but no judge app is set (--judge-app)")
			}
			type job struct {
				bot string
				ec  evalCase
			}
			var jobs []job
			for r := 0; r < repeat; r++ {
				for _, b := range strings.Split(bots, ",") {
					for _, ec := range cases {
						jobs = append(jobs, job{strings.TrimSpace(b), ec})
					}
				}
			}
			fmt.Printf("tag=%s bots=%s cases=%d repeat=%d → %s\n", tag, bots, len(cases), repeat, out)
			f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			defer f.Close()
			sem := make(chan struct{}, max(parallel, 1))
			var wg sync.WaitGroup
			for n, jb := range jobs {
				wg.Add(1)
				sem <- struct{}{}
				go func(jb job) {
					defer wg.Done()
					defer func() { <-sem }()
					rec := runCase(ctx, c, s, jb.bot, jb.ec, o)
					o.mu.Lock()
					defer o.mu.Unlock()
					line, _ := json.Marshal(rec)
					_, _ = f.Write(append(line, '\n'))
					printCaseLine(rec)
				}(jb)
				if n < parallel {
					time.Sleep(2 * time.Second) // stagger cold starts
				}
			}
			wg.Wait()
			return report(out, tag, true)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVar(&bots, "bot", "", "Comma list of bots (overrides the suite's bot) — run a matrix of variants")
	cmd.Flags().StringVar(&tag, "tag", "", "Run tag (default <suite>-<MMDD-HHMM>)")
	cmd.Flags().StringVar(&out, "out", "", "Output JSONL (default runs/<tag>.jsonl)")
	cmd.Flags().StringVar(&only, "only", "", "Comma list of case ids")
	cmd.Flags().StringVar(&runtime, "runtime", "", "headless-ubuntu | ubuntu-desktop")
	cmd.Flags().StringVar(&judgeApp, "judge-app", "", "Neutral judge app id ($HELIX_EVAL_JUDGE_APP)")
	cmd.Flags().StringVar(&judgeModel, "judge-model", "", "Judge model ($HELIX_EVAL_JUDGE_MODEL)")
	cmd.Flags().IntVar(&parallel, "parallel", 3, "Cases in parallel")
	cmd.Flags().IntVar(&repeat, "repeat", 1, "Repeat the suite N times (measure noise)")
	cmd.Flags().BoolVar(&keep, "keep", false, "Keep every instance")
	cmd.Flags().BoolVar(&keepFailed, "keep-failed", false, "Keep failed cases' instances for debugging")
	return cmd
}

func printCaseLine(r caseRecord) {
	status := "PASS"
	if !r.Pass {
		status = "FAIL"
	}
	tools := 0
	var why []string
	for n, t := range r.Turns {
		tools += t.ToolCalls
		for k, v := range t.Checks {
			if !v {
				why = append(why, fmt.Sprintf("t%d %s", n+1, k))
			}
		}
		why = append(why, t.Failed...)
	}
	if r.Error != "" {
		why = append(why, r.Error)
	}
	if r.Judge != nil && !r.Judge.Pass {
		why = append(why, "judge: "+r.Judge.Reason)
	}
	line := fmt.Sprintf("%s %s %s %.1fs turns=%d tools=%d", status, r.Bot, r.Case, r.Seconds, len(r.Turns), tools)
	if r.Kept {
		line += " session=" + r.Session
	}
	if !r.Pass && len(why) > 0 {
		line += "  ← " + truncate(strings.Join(why, "; "), 300)
	}
	fmt.Println(line)
}

func loadRecords(path, tag string) ([]caseRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []caseRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r caseRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil && (tag == "" || r.Tag == tag) {
			out = append(out, r)
		}
	}
	return out, sc.Err()
}

func pct(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	return s[int(p*float64(len(s)-1)+0.5)]
}

func report(path, tag string, perCase bool) error {
	recs, err := loadRecords(path, tag)
	if err != nil {
		return err
	}
	byBot := map[string][]caseRecord{}
	for _, r := range recs {
		byBot[r.Bot] = append(byBot[r.Bot], r)
	}
	botsSorted := make([]string, 0, len(byBot))
	for b := range byBot {
		botsSorted = append(botsSorted, b)
	}
	sort.Strings(botsSorted)
	fmt.Printf("\n%-28s %-7s %-14s %-11s %-10s %s\n", "BOT", "PASS", "MEDIAN TURN s", "P90 TURN s", "TOOLS", "ERRORS")
	for _, b := range botsSorted {
		pass, tools, errs := 0, 0, 0
		var secs []float64
		for _, r := range byBot[b] {
			if r.Pass {
				pass++
			}
			if r.Error != "" {
				errs++
			}
			for _, t := range r.Turns {
				secs = append(secs, t.Seconds)
				tools += t.ToolCalls
			}
		}
		fmt.Printf("%-28s %-7s %-14.1f %-11.1f %-10d %d\n", b, fmt.Sprintf("%d/%d", pass, len(byBot[b])), pct(secs, .5), pct(secs, .9), tools, errs)
	}
	if perCase {
		fmt.Println()
		for _, r := range recs {
			mark := "✅"
			if !r.Pass {
				mark = "❌"
			}
			note := r.Error
			if note == "" && r.Judge != nil {
				note = r.Judge.Reason
			}
			fmt.Printf("%-28s %-26s %s %6.1fs turns=%d %s\n", r.Bot, truncate(r.Case, 26), mark, r.Seconds, len(r.Turns), truncate(note, 90))
		}
	}
	return nil
}

func newEvalReportCmd() *cobra.Command {
	var tag string
	var cases bool
	cmd := &cobra.Command{
		Use:   "report <runs.jsonl>",
		Short: "Summarise a run: pass rate, median/p90 turn time, tool calls, errors",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return report(args[0], tag, cases)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "Only this tag")
	cmd.Flags().BoolVar(&cases, "cases", false, "Per-case lines")
	return cmd
}

func newEvalCompareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <before.jsonl> <after.jsonl>",
		Short: "Per-case FIXED / REGRESSED, turn time and tool calls between two runs",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			a, err := loadRecords(args[0], "")
			if err != nil {
				return err
			}
			b, err := loadRecords(args[1], "")
			if err != nil {
				return err
			}
			key := func(r caseRecord) string { return r.Bot + "\x00" + r.Case }
			ia := map[string]caseRecord{}
			for _, r := range a {
				ia[key(r)] = r
			}
			sum := func(r caseRecord) (float64, int) {
				s, t := 0.0, 0
				for _, x := range r.Turns {
					s += x.Seconds
					t += x.ToolCalls
				}
				return s, t
			}
			mark := func(p bool) string {
				if p {
					return "✅"
				}
				return "❌"
			}
			pa, pb := 0, 0
			for _, r := range a {
				if r.Pass {
					pa++
				}
			}
			fmt.Printf("%-28s %-26s %-6s %-10s %-12s %s\n", "BOT", "CASE", "PASS", "", "TURN s", "TOOLS")
			for _, rb := range b {
				if rb.Pass {
					pb++
				}
				ra, ok := ia[key(rb)]
				if !ok {
					continue
				}
				flag := ""
				if ra.Pass != rb.Pass {
					flag = "REGRESSED"
					if rb.Pass {
						flag = "FIXED"
					}
				}
				sa, ta := sum(ra)
				sb, tb := sum(rb)
				fmt.Printf("%-28s %-26s %s→%s %-10s %-12s %d→%d\n", rb.Bot, truncate(rb.Case, 26), mark(ra.Pass), mark(rb.Pass), flag,
					fmt.Sprintf("%.0f→%.0f", sa, sb), ta, tb)
			}
			fmt.Printf("\npass %d/%d → %d/%d. Replicates vary ±25-50%% in time: judge speed on totals over ≥10 cases.\n", pa, len(a), pb, len(b))
			return nil
		},
	}
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
