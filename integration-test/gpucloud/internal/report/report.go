// Package report writes the harness's per-entry results as JUnit XML
// (for CI consumption) and a Markdown summary (for PR comments / Slack).
package report

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/integration-test/gpucloud/internal/scenarios"
)

// Result is one matrix entry's outcome.
type Result struct {
	Entry           string
	Passed          bool
	DurationSeconds int
	Failure         string
	Scenarios       []scenarios.Result
	CachedFrom      time.Time // non-zero when this result came from the cache
}

// --- JUnit XML ---

type junitTestsuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Time     float64         `xml:"time,attr"`
	Cases    []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Name      string         `xml:"name,attr"`
	Classname string         `xml:"classname,attr"`
	Time      float64        `xml:"time,attr"`
	Failure   *junitFailure  `xml:"failure,omitempty"`
	Skipped   *junitSkipped  `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// WriteJUnit emits a flat testsuite where each scenario across all
// matrix entries is its own test case.
func WriteJUnit(path string, results []Result) error {
	suite := junitTestsuite{Name: "gpucloud-it"}
	for _, r := range results {
		for _, s := range r.Scenarios {
			tc := junitTestcase{
				Name:      s.Name,
				Classname: r.Entry,
				Time:      float64(s.DurationSeconds),
			}
			if !s.Passed {
				tc.Failure = &junitFailure{Message: s.Failure}
				suite.Failures++
			}
			suite.Cases = append(suite.Cases, tc)
			suite.Tests++
			suite.Time += float64(s.DurationSeconds)
		}
		// If the entry failed before scenarios ran (e.g. provision
		// failed), emit a single failing case for the entry itself.
		if len(r.Scenarios) == 0 && !r.Passed && r.CachedFrom.IsZero() {
			tc := junitTestcase{
				Name:      "provision",
				Classname: r.Entry,
				Time:      float64(r.DurationSeconds),
				Failure:   &junitFailure{Message: r.Failure},
			}
			suite.Cases = append(suite.Cases, tc)
			suite.Tests++
			suite.Failures++
		}
	}
	data, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(xml.Header+string(data)+"\n"), 0o644)
}

// --- Markdown summary ---

// WriteMarkdown emits a PR-comment-friendly markdown summary.
func WriteMarkdown(path string, results []Result) error {
	var b strings.Builder
	b.WriteString("# RunPod integration test results\n\n")
	pass, fail := 0, 0
	for _, r := range results {
		if r.Passed {
			pass++
		} else {
			fail++
		}
	}
	b.WriteString(fmt.Sprintf("**%d passed, %d failed** out of %d matrix entries.\n\n",
		pass, fail, len(results)))
	b.WriteString("| Entry | Result | Duration | Notes |\n")
	b.WriteString("|-------|--------|----------|-------|\n")
	for _, r := range results {
		mark := "✅"
		if !r.Passed {
			mark = "❌"
		}
		notes := ""
		if !r.CachedFrom.IsZero() {
			notes = "cached " + r.CachedFrom.Format(time.RFC3339)
		} else if r.Failure != "" {
			notes = "wrapper: " + r.Failure
		} else {
			failed := []string{}
			for _, s := range r.Scenarios {
				if !s.Passed {
					failed = append(failed, s.Name)
				}
			}
			if len(failed) > 0 {
				notes = "failed scenarios: " + strings.Join(failed, ", ")
			}
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | %ds | %s |\n", r.Entry, mark, r.DurationSeconds, notes))
	}
	for _, r := range results {
		if r.Passed {
			continue
		}
		b.WriteString(fmt.Sprintf("\n### `%s` failure detail\n\n", r.Entry))
		if r.Failure != "" {
			b.WriteString("- **Wrapper error**: " + r.Failure + "\n")
		}
		for _, s := range r.Scenarios {
			if s.Passed {
				continue
			}
			b.WriteString(fmt.Sprintf("- **%s** (%ds): %s\n", s.Name, s.DurationSeconds, s.Failure))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
