package main

import (
	"testing"
)

func TestIsComposeBuildCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "build command",
			args: []string{"build"},
			want: true,
		},
		{
			name: "up with build",
			args: []string{"up", "--build"},
			want: true,
		},
		{
			name: "up without build",
			args: []string{"up", "-d"},
			want: false,
		},
		{
			name: "down command",
			args: []string{"down"},
			want: false,
		},
		{
			name: "up with build after --",
			args: []string{"up", "--", "--build"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComposeBuildCommand(tt.args)
			if got != tt.want {
				t.Errorf("isComposeBuildCommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasComposeCacheFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "no cache flags",
			args: []string{"build"},
			want: false,
		},
		{
			name: "has cache_from in set",
			args: []string{"build", "--set=*.build.cache_from=[...]"},
			want: true,
		},
		{
			name: "has cache_to in set",
			args: []string{"build", "--set=*.build.cache_to=[...]"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasComposeCacheFlags(tt.args)
			if got != tt.want {
				t.Errorf("hasComposeCacheFlags(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want bool
	}{
		{"equal versions", "2.24.0", "2.24.0", true},
		{"v1 newer major", "3.0.0", "2.24.0", true},
		{"v1 newer minor", "2.25.0", "2.24.0", true},
		{"v1 newer patch", "2.24.1", "2.24.0", true},
		{"v1 older", "2.23.0", "2.24.0", false},
		{"with v prefix", "v2.24.0", "2.24.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}
