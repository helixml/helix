package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNeedsComposeProcessing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "up command",
			args: []string{"up", "-d"},
			want: true,
		},
		{
			name: "down command",
			args: []string{"down"},
			want: true,
		},
		{
			name: "build command",
			args: []string{"build"},
			want: true,
		},
		{
			name: "logs command",
			args: []string{"logs", "-f"},
			want: true,
		},
		{
			name: "version command",
			args: []string{"version"},
			want: false,
		},
		{
			name: "help command",
			args: []string{"--help"},
			want: false,
		},
		{
			name: "empty args",
			args: []string{},
			want: false,
		},
		{
			name: "command after --",
			args: []string{"--", "up"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsComposeProcessing(tt.args)
			if got != tt.want {
				t.Errorf("needsComposeProcessing(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestFindComposeFiles(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "short flag",
			args: []string{"-f", "docker-compose.yaml", "up"},
			want: []string{"docker-compose.yaml"},
		},
		{
			name: "long flag",
			args: []string{"--file", "compose.yml", "up"},
			want: []string{"compose.yml"},
		},
		{
			name: "short flag with equals",
			args: []string{"-f=docker-compose.yaml", "up"},
			want: []string{"docker-compose.yaml"},
		},
		{
			name: "multiple files",
			args: []string{"-f", "base.yaml", "-f", "override.yaml", "up"},
			want: []string{"base.yaml", "override.yaml"},
		},
		{
			name: "no files specified",
			args: []string{"up", "-d"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findComposeFiles(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findComposeFiles(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasProjectFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "short flag",
			args: []string{"-p", "myproject", "up"},
			want: true,
		},
		{
			name: "long flag",
			args: []string{"--project-name", "myproject", "up"},
			want: true,
		},
		{
			name: "short flag with equals",
			args: []string{"-p=myproject", "up"},
			want: true,
		},
		{
			name: "long flag with equals",
			args: []string{"--project-name=myproject", "up"},
			want: true,
		},
		{
			name: "no project flag",
			args: []string{"up", "-d"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasProjectFlag(tt.args)
			if got != tt.want {
				t.Errorf("hasProjectFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestGetProjectArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		taskNumber string
		sessionID  string
		want       []string
	}{
		{
			name:       "already has project flag",
			args:       []string{"-p", "existing", "up"},
			taskNumber: "42",
			sessionID:  "ses_123",
			want:       nil,
		},
		{
			name:       "use task number",
			args:       []string{"up"},
			taskNumber: "42",
			sessionID:  "ses_123",
			want:       []string{"-p", "helix-task-42"},
		},
		{
			name:       "use session id",
			args:       []string{"up"},
			taskNumber: "",
			sessionID:  "ses_123",
			want:       []string{"-p", "helix-ses_123"},
		},
		{
			name:       "no env vars",
			args:       []string{"up"},
			taskNumber: "",
			sessionID:  "",
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			os.Unsetenv("HELIX_TASK_NUMBER")
			os.Unsetenv("HELIX_SESSION_ID")
			if tt.taskNumber != "" {
				os.Setenv("HELIX_TASK_NUMBER", tt.taskNumber)
			}
			if tt.sessionID != "" {
				os.Setenv("HELIX_SESSION_ID", tt.sessionID)
			}
			defer os.Unsetenv("HELIX_TASK_NUMBER")
			defer os.Unsetenv("HELIX_SESSION_ID")

			got := getProjectArgs(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getProjectArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

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
		{
			name: "equal versions",
			v1:   "2.24.0",
			v2:   "2.24.0",
			want: true,
		},
		{
			name: "v1 newer major",
			v1:   "3.0.0",
			v2:   "2.24.0",
			want: true,
		},
		{
			name: "v1 newer minor",
			v1:   "2.25.0",
			v2:   "2.24.0",
			want: true,
		},
		{
			name: "v1 newer patch",
			v1:   "2.24.1",
			v2:   "2.24.0",
			want: true,
		},
		{
			name: "v1 older",
			v1:   "2.23.0",
			v2:   "2.24.0",
			want: false,
		},
		{
			name: "with v prefix",
			v1:   "v2.24.0",
			v2:   "2.24.0",
			want: true,
		},
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

func TestProcessComposeVolume(t *testing.T) {
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/123")
	defer os.Unsetenv("WORKSPACE_DIR")

	baseDir := "/some/project"

	tests := []struct {
		name    string
		vol     string
		baseDir string
		want    string
	}{
		{
			name:    "absolute path translation",
			vol:     "/home/retro/work/project:/app",
			baseDir: baseDir,
			want:    "/data/workspaces/123/project:/app",
		},
		{
			name:    "named volume unchanged",
			vol:     "myvolume:/data",
			baseDir: baseDir,
			want:    "myvolume:/data",
		},
		{
			name:    "relative path resolved",
			vol:     "./src:/app",
			baseDir: baseDir,
			want:    "/some/project/src:/app", // Relative paths are resolved to absolute via baseDir
		},
		{
			name:    "volume with options",
			vol:     "/home/retro/work/project:/app:ro",
			baseDir: baseDir,
			want:    "/data/workspaces/123/project:/app:ro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := processComposeVolume(tt.vol, tt.baseDir)
			if got != tt.want {
				t.Errorf("processComposeVolume(%q, %q) = %q, want %q", tt.vol, tt.baseDir, got, tt.want)
			}
		})
	}
}

func TestProcessComposeFile(t *testing.T) {
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/123")
	defer os.Unsetenv("WORKSPACE_DIR")

	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "compose-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test compose file
	inputContent := `version: "3"
services:
  app:
    image: myapp
    volumes:
      - /home/retro/work/project:/app
      - myvolume:/data
  db:
    image: postgres
    volumes:
      - /home/retro/work/data:/var/lib/postgresql/data
`
	inputFile := filepath.Join(tmpDir, "docker-compose.yaml")
	outputFile := filepath.Join(tmpDir, ".hydra-resolved.docker-compose.yaml")

	if err := os.WriteFile(inputFile, []byte(inputContent), 0644); err != nil {
		t.Fatalf("Failed to write input file: %v", err)
	}

	// Process the file
	if err := processComposeFile(inputFile, outputFile); err != nil {
		t.Fatalf("processComposeFile failed: %v", err)
	}

	// Read the output
	outputContent, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(outputContent)

	// Check that paths were translated
	if !contains(output, "/data/workspaces/123/project:/app") {
		t.Errorf("Expected translated path for app service, got:\n%s", output)
	}
	if !contains(output, "/data/workspaces/123/data:/var/lib/postgresql/data") {
		t.Errorf("Expected translated path for db service, got:\n%s", output)
	}
	// Named volume should be unchanged
	if !contains(output, "myvolume:/data") {
		t.Errorf("Expected named volume to be unchanged, got:\n%s", output)
	}
}

func TestCleanupTmpFiles(t *testing.T) {
	// Create temp files
	tmpDir, err := os.MkdirTemp("", "cleanup-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	os.WriteFile(file1, []byte("test"), 0644)
	os.WriteFile(file2, []byte("test"), 0644)

	// Cleanup should remove them
	cleanupTmpFiles([]string{file1, file2})

	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Errorf("Expected file1 to be removed")
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Errorf("Expected file2 to be removed")
	}

	// Cleanup of non-existent files should not error
	cleanupTmpFiles([]string{"/nonexistent/file.txt"})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
