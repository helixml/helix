package server

import "testing"

func TestCleanGeneratedTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "trims model formatting", input: `  "Fix task title generation."  `, max: 60, want: "Fix task title generation."},
		{name: "counts unicode characters", input: "改善任务标题生成流程", max: 6, want: "改善任务标题"},
		{name: "leaves short title", input: "Name build-only tasks early", max: 60, want: "Name build-only tasks early"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanGeneratedTitle(tt.input, tt.max); got != tt.want {
				t.Fatalf("cleanGeneratedTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
