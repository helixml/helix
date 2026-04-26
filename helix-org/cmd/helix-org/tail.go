package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/helixml/helix-org/server"
)

// runTail is the human's "what's happening?" view. It long-polls the
// server's /tail endpoint and prints each event as it lands.
//
// Usage:
//
//	helix-org tail                  # all channels (same as `tail '*'`)
//	helix-org tail 'c-*'            # any channel id starting with c-
//	helix-org tail c-newsletter     # one channel
//	helix-org tail 'c-news*' c-drafts
//
// Globs match Channel IDs via Go's path.Match: '*', '?', '[abc]'.
// Quote globs in the shell so they aren't expanded against the cwd.
func runTail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	urlFlag := fs.String("url", "http://localhost:8080", "Server base URL")
	wait := fs.Int("wait", 30, "Long-poll wait per request in seconds (0..60)")
	limit := fs.Int("limit", 100, "Max events per poll (1..500)")
	plain := fs.Bool("no-color", false, "Disable ANSI colour output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}

	useColor := !*plain && term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // stdout fd is always a small non-negative int

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	since := ""
	for {
		if ctx.Err() != nil {
			return nil
		}
		events, err := tailFetch(ctx, *urlFlag, patterns, since, *wait, *limit)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			fmt.Fprintf(os.Stderr, "tail: %v (retrying in 2s)\n", err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil
			}
			continue
		}
		for _, e := range events {
			fmt.Println(formatTailEvent(e, useColor))
			since = e.ID
		}
	}
}

type tailEvent struct {
	ID        string    `json:"-"`
	ChannelID string    `json:"channelId"`
	Source    string    `json:"source"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

func tailFetch(ctx context.Context, baseURL string, patterns []string, since string, waitSecs, limit int) ([]tailEvent, error) {
	q := url.Values{}
	for _, p := range patterns {
		q.Add("match", p)
	}
	if since != "" {
		q.Set("since", since)
	}
	if waitSecs > 0 {
		q.Set("wait", fmt.Sprintf("%d", waitSecs))
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	full := strings.TrimRight(baseURL, "/") + "/tail?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	// Give the request slightly longer than the server's wait so the
	// server gets to respond before we time out.
	client := &http.Client{Timeout: time.Duration(waitSecs+10) * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if res.StatusCode >= 400 {
		var env apiErrorsEnvelope
		if jerr := json.Unmarshal(body, &env); jerr == nil && len(env.Errors) > 0 {
			return nil, &apiError{Status: res.StatusCode, Title: env.Errors[0].Title, Detail: env.Errors[0].Detail}
		}
		return nil, fmt.Errorf("http %d: %s", res.StatusCode, string(body))
	}

	var envelope struct {
		Data []server.Resource `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	events := make([]tailEvent, 0, len(envelope.Data))
	for _, r := range envelope.Data {
		var attrs tailEvent
		if err := json.Unmarshal(r.Attributes, &attrs); err != nil {
			return nil, fmt.Errorf("decode event %s: %w", r.ID, err)
		}
		attrs.ID = r.ID
		events = append(events, attrs)
	}
	return events, nil
}

// formatTailEvent renders one event as one logical block:
//
//	HH:MM:SS  channel  source  first-line-of-body
//	                           subsequent-line
//	                           subsequent-line
func formatTailEvent(e tailEvent, color bool) string {
	const (
		cReset  = "\033[0m"
		cCyan   = "\033[36m"
		cYellow = "\033[33m"
		cDim    = "\033[2m"
	)
	ts := e.CreatedAt.Local().Format("15:04:05")
	channel := e.ChannelID
	source := e.Source
	if source == "" {
		source = "(system)"
	}
	if color {
		ts = cDim + ts + cReset
		channel = cCyan + channel + cReset
		source = cYellow + source + cReset
	}
	lines := strings.Split(strings.TrimRight(e.Body, "\n"), "\n")
	header := fmt.Sprintf("%s  %s  %s  %s", ts, channel, source, lines[0])
	if len(lines) == 1 {
		return header
	}
	// Indent continuation lines under the body column.
	indent := strings.Repeat(" ", len("HH:MM:SS  ")+len(e.ChannelID)+2+len(e.Source)+2)
	var b strings.Builder
	b.WriteString(header)
	for _, l := range lines[1:] {
		b.WriteByte('\n')
		b.WriteString(indent)
		b.WriteString(l)
	}
	return b.String()
}
