package main

import (
	"reflect"
	"testing"
)

func TestIsBuildCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "simple build",
			args: []string{"build", "."},
			want: true,
		},
		{
			name: "build with flags",
			args: []string{"build", "-t", "myimage", "."},
			want: true,
		},
		{
			name: "buildx build",
			args: []string{"buildx", "build", "-t", "myimage", "."},
			want: true,
		},
		{
			name: "run command",
			args: []string{"run", "-it", "ubuntu"},
			want: false,
		},
		{
			name: "pull command",
			args: []string{"pull", "nginx"},
			want: false,
		},
		{
			name: "empty args",
			args: []string{},
			want: false,
		},
		{
			name: "buildx without build",
			args: []string{"buildx", "ls"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBuildCommand(tt.args)
			if got != tt.want {
				t.Errorf("isBuildCommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestExtractImageTag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "short flag",
			args: []string{"build", "-t", "myimage", "."},
			want: "myimage",
		},
		{
			name: "long flag",
			args: []string{"build", "--tag", "myimage:latest", "."},
			want: "myimage:latest",
		},
		{
			name: "short flag with equals",
			args: []string{"build", "-t=myimage", "."},
			want: "myimage",
		},
		{
			name: "long flag with equals",
			args: []string{"build", "--tag=myimage:v1", "."},
			want: "myimage:v1",
		},
		{
			name: "no tag",
			args: []string{"build", "."},
			want: "",
		},
		{
			name: "multiple tags - returns first",
			args: []string{"build", "-t", "first", "-t", "second", "."},
			want: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractImageTag(tt.args)
			if got != tt.want {
				t.Errorf("extractImageTag(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestSanitizeForPath(t *testing.T) {
	tests := []struct {
		imageName string
		want      string
	}{
		{"myimage", "myimage"},
		{"myimage:latest", "myimage"},
		{"registry.example.com/foo/bar:v1", "foo_bar"},
		{"docker.io/library/nginx:latest", "library_nginx"},
		{"foo/bar/baz", "foo_bar_baz"},
		{"image@sha256:abc123", "image"},
		{"", "default"},
		{"my-image_v2.0", "my-image_v2.0"},
	}

	for _, tt := range tests {
		t.Run(tt.imageName, func(t *testing.T) {
			got := sanitizeForPath(tt.imageName)
			if got != tt.want {
				t.Errorf("sanitizeForPath(%q) = %q, want %q", tt.imageName, got, tt.want)
			}
		})
	}
}

func TestHasCacheFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "no cache flags",
			args: []string{"build", "-t", "myimage", "."},
			want: false,
		},
		{
			name: "has cache-from",
			args: []string{"build", "--cache-from", "type=local,src=/tmp", "-t", "myimage", "."},
			want: true,
		},
		{
			name: "has cache-to",
			args: []string{"build", "--cache-to", "type=local,dest=/tmp", "-t", "myimage", "."},
			want: true,
		},
		{
			name: "has cache-from with equals",
			args: []string{"build", "--cache-from=type=local,src=/tmp", "."},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCacheFlags(tt.args)
			if got != tt.want {
				t.Errorf("hasCacheFlags(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestInjectBuildCacheFlags(t *testing.T) {
	t.Run("no cache dir - no injection", func(t *testing.T) {
		args := []string{"build", "-t", "myimage", "."}
		got, err := injectBuildCacheFlags(args)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, args) {
			t.Errorf("Expected no change when cache dir doesn't exist, got %v", got)
		}
	})

	t.Run("non-build command - no injection", func(t *testing.T) {
		args := []string{"run", "-it", "ubuntu"}
		got, err := injectBuildCacheFlags(args)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, args) {
			t.Errorf("Expected no change for non-build command, got %v", got)
		}
	})

	t.Run("already has cache flags - no injection", func(t *testing.T) {
		args := []string{"build", "--cache-from=type=local,src=/other", "-t", "myimage", "."}
		got, err := injectBuildCacheFlags(args)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, args) {
			t.Errorf("Expected no change when cache flags present, got %v", got)
		}
	})
}

func TestProcessDockerArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "non-build passthrough",
			args: []string{"run", "-v", "/home/user/project:/app", "ubuntu"},
			want: []string{"run", "-v", "/home/user/project:/app", "ubuntu"},
		},
		{
			name: "named volume passthrough",
			args: []string{"run", "-v", "myvolume:/data", "ubuntu"},
			want: []string{"run", "-v", "myvolume:/data", "ubuntu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processDockerArgs(tt.args)
			if err != nil {
				t.Fatalf("processDockerArgs(%v) returned error: %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processDockerArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Mode
	}{
		{
			name: "docker binary",
			args: []string{"/usr/bin/docker", "run", "ubuntu"},
			want: ModeDocker,
		},
		{
			name: "docker-compose binary",
			args: []string{"/usr/bin/docker-compose", "up"},
			want: ModeCompose,
		},
		{
			name: "docker compose plugin",
			args: []string{"/usr/bin/docker", "compose", "up"},
			want: ModeCompose,
		},
		{
			name: "empty args",
			args: []string{},
			want: ModeDocker,
		},
		{
			name: "docker build",
			args: []string{"docker", "build", "."},
			want: ModeDocker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMode(tt.args)
			if got != tt.want {
				t.Errorf("detectMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
