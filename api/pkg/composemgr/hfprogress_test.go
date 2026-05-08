package composemgr

import "testing"

func TestParseHFProgress_Shards(t *testing.T) {
	logs := []string{
		"INFO 11-12 09:33:01 some other line",
		"Downloading shards:  25%|██▌      | 12/47 [09:22<27:18,  1.99s/it]",
	}
	got := ParseHFProgress(logs)
	if got.Percent != 25 || got.Current != 12 || got.Total != 47 || got.ETA != "27:18" || got.Stage != "shards" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseHFProgress_Files(t *testing.T) {
	logs := []string{
		"Fetching 17 files:   24%|██▎      | 4/17 [00:32<01:44,  6.50s/file]",
	}
	got := ParseHFProgress(logs)
	if got.Percent != 24 || got.Current != 4 || got.Total != 17 || got.ETA != "01:44" || got.Stage != "files" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseHFProgress_WeightsBytes(t *testing.T) {
	logs := []string{
		"model.safetensors:   78%|██████▋  | 7.81G/10.0G [03:14<00:55, 39.7MB/s]",
	}
	got := ParseHFProgress(logs)
	if got.Percent != 78 || got.Stage != "weights" || got.ETA != "00:55" {
		t.Fatalf("got %+v", got)
	}
	// Bytes get converted via suffixToInt — sanity check the ratio is sensible.
	if got.Current == 0 || got.Total == 0 || got.Current >= got.Total {
		t.Fatalf("byte conversion looks wrong: cur=%d tot=%d", got.Current, got.Total)
	}
}

func TestParseHFProgress_LatestWins(t *testing.T) {
	logs := []string{
		"Downloading shards:  10%|█        | 5/47 [01:00<10:00,  2.00s/it]",
		"Downloading shards:  50%|████▌    | 23/47 [05:00<05:00,  2.00s/it]",
		"Downloading shards:  90%|█████████| 42/47 [08:30<00:30,  2.00s/it]",
	}
	got := ParseHFProgress(logs)
	if got.Percent != 90 || got.Current != 42 {
		t.Fatalf("expected last line to win, got %+v", got)
	}
}

func TestParseHFProgress_NoMatch(t *testing.T) {
	logs := []string{
		"INFO: starting up",
		"WARNING: something",
		"random text without progress format",
	}
	got := ParseHFProgress(logs)
	if got.Percent != 0 || got.Stage != "" {
		t.Fatalf("expected zero, got %+v", got)
	}
}

func TestParseHFProgress_BadPercent(t *testing.T) {
	// Out-of-range percent should be rejected, not clamped.
	logs := []string{
		"Downloading shards:  150%|███     | 12/47 [09:22<27:18,  1.99s/it]",
	}
	got := ParseHFProgress(logs)
	if got.Percent != 0 {
		t.Fatalf("expected zero on bad percent, got %+v", got)
	}
}
