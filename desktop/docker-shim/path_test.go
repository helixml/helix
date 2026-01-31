package main

import (
	"os"
	"testing"
)

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		workspaceDir string
		want         string
	}{
		{
			name:         "workspace dir translation",
			path:         "/home/retro/work/myproject/file.txt",
			workspaceDir: "/data/workspaces/spec-tasks/123",
			want:         "/data/workspaces/spec-tasks/123/myproject/file.txt",
		},
		{
			name:         "workspace dir exact match",
			path:         "/home/retro/work",
			workspaceDir: "/data/workspaces/spec-tasks/123",
			want:         "/data/workspaces/spec-tasks/123",
		},
		{
			name:         "no workspace dir set",
			path:         "/home/retro/work/myproject",
			workspaceDir: "",
			want:         "/home/retro/work/myproject",
		},
		{
			name:         "path not under user path",
			path:         "/var/lib/docker/stuff",
			workspaceDir: "/data/workspaces/spec-tasks/123",
			want:         "/var/lib/docker/stuff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or clear WORKSPACE_DIR
			if tt.workspaceDir != "" {
				os.Setenv("WORKSPACE_DIR", tt.workspaceDir)
			} else {
				os.Unsetenv("WORKSPACE_DIR")
			}
			defer os.Unsetenv("WORKSPACE_DIR")

			got := resolvePath(tt.path)
			if got != tt.want {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsNamedVolume(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"myvolume", true},
		{"db_data", true},
		{"/var/lib/data", false},
		{"./relative", false},
		{"../parent", false},
		{".", false},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			got := isNamedVolume(tt.src)
			if got != tt.want {
				t.Errorf("isNamedVolume(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestProcessVolumeArg(t *testing.T) {
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/123")
	defer os.Unsetenv("WORKSPACE_DIR")

	tests := []struct {
		vol  string
		want string
	}{
		{
			vol:  "/home/retro/work/project:/app",
			want: "/data/workspaces/123/project:/app",
		},
		{
			vol:  "/home/retro/work/project:/app:ro",
			want: "/data/workspaces/123/project:/app:ro",
		},
		{
			vol:  "myvolume:/data",
			want: "myvolume:/data",
		},
		{
			vol:  "/var/lib/stuff:/stuff",
			want: "/var/lib/stuff:/stuff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.vol, func(t *testing.T) {
			got := processVolumeArg(tt.vol)
			if got != tt.want {
				t.Errorf("processVolumeArg(%q) = %q, want %q", tt.vol, got, tt.want)
			}
		})
	}
}

func TestProcessMountArg(t *testing.T) {
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/123")
	defer os.Unsetenv("WORKSPACE_DIR")

	tests := []struct {
		mount string
		want  string
	}{
		{
			mount: "type=bind,source=/home/retro/work/project,target=/app",
			want:  "type=bind,source=/data/workspaces/123/project,target=/app",
		},
		{
			mount: "type=bind,src=/home/retro/work/project,dst=/app",
			want:  "type=bind,src=/data/workspaces/123/project,dst=/app",
		},
		{
			mount: "type=volume,source=myvolume,target=/data",
			want:  "type=volume,source=myvolume,target=/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.mount, func(t *testing.T) {
			got := processMountArg(tt.mount)
			if got != tt.want {
				t.Errorf("processMountArg(%q) = %q, want %q", tt.mount, got, tt.want)
			}
		})
	}
}
