package composemgr

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// ParseHFProgress walks vLLM container log lines (newest-last is OK) and
// returns the most recent download progress it can extract. Returns the
// zero value if no recognisable progress line is found.
//
// Recognised tqdm/HF lines look like:
//
//	Downloading shards:  25%|██▌      | 12/47 [09:22<27:18,  1.99s/it]
//	Fetching 17 files:   24%|██▎      | 4/17 [00:32<01:44,  6.50s/file]
//	model.safetensors:   78%|██████▋  | 7.81G/10.0G [03:14<00:55, 39.7MB/s]
//
// We don't try to parse every variant — just the common ones that
// vLLM/transformers emit during weight download. The function is forgiving:
// anything it doesn't recognise is silently skipped.
func ParseHFProgress(logLines []string) types.ServiceDownloadProgress {
	var latest types.ServiceDownloadProgress
	for _, line := range logLines {
		if p, ok := parseLine(line); ok {
			latest = p
		}
	}
	return latest
}

// fragmentRE matches the canonical tqdm middle: "PCT%|bar| CUR/TOT [elapsed<eta...]".
// Captures: 1=percent, 2=current, 3=total, 4=eta.
var fragmentRE = regexp.MustCompile(`(\d+)%\|[^|]*\|\s*([0-9.]+[KMGT]?)/([0-9.]+[KMGT]?)\s*\[[^<]*<([^,\]]+)`)

// stageRE pulls a human-readable stage tag from the line prefix.
var stageRE = regexp.MustCompile(`(?i)(downloading shards|fetching \d+ files|model[\w.\-]*\.(?:safetensors|bin|pt|gguf))`)

func parseLine(line string) (types.ServiceDownloadProgress, bool) {
	m := fragmentRE.FindStringSubmatch(line)
	if m == nil {
		return types.ServiceDownloadProgress{}, false
	}
	percent, err := strconv.Atoi(m[1])
	if err != nil || percent < 0 || percent > 100 {
		return types.ServiceDownloadProgress{}, false
	}
	cur := suffixToInt(m[2])
	tot := suffixToInt(m[3])
	eta := strings.TrimSpace(m[4])
	stage := ""
	if sm := stageRE.FindStringSubmatch(line); sm != nil {
		s := strings.ToLower(sm[1])
		switch {
		case strings.Contains(s, "shard"):
			stage = "shards"
		case strings.Contains(s, "fetching"):
			stage = "files"
		default:
			stage = "weights"
		}
	}
	return types.ServiceDownloadProgress{
		Percent: percent,
		Current: cur,
		Total:   tot,
		ETA:     eta,
		Stage:   stage,
	}, true
}

// suffixToInt converts a tqdm-style human number ("7.81G", "47", "10.0M")
// to an int. Returns 0 if it can't parse — Current/Total are advisory
// fields, not load-bearing for the percent calculation.
func suffixToInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := 1
	switch s[len(s)-1] {
	case 'K', 'k':
		mult = 1 << 10
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1 << 20
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1 << 30
		s = s[:len(s)-1]
	case 'T', 't':
		mult = 1 << 40
		s = s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(f * float64(mult))
}
