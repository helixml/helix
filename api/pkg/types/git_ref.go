package types

import (
	"fmt"
	"strings"
)

// MaxGitBranchNameLength bounds user-supplied branch names; it matches the
// size:255 columns that persist them.
const MaxGitBranchNameLength = 255

// ValidateGitBranchName enforces `git check-ref-format --branch` semantics on a
// user-supplied branch name, and additionally rejects a leading '-' so the
// value can never be parsed as an option when passed positionally to git.
func ValidateGitBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if len(name) > MaxGitBranchNameLength {
		return fmt.Errorf("branch name exceeds %d characters", MaxGitBranchNameLength)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("branch name %q must not start with '-'", name)
	}
	if name == "@" || name == "HEAD" {
		return fmt.Errorf("branch name %q is reserved", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("branch name %q contains a control character", name)
		}
		switch r {
		case ' ', '~', '^', ':', '?', '*', '[', '\\':
			return fmt.Errorf("branch name %q contains forbidden character %q", name, r)
		}
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("branch name %q must not contain '..'", name)
	}
	if strings.Contains(name, "@{") {
		return fmt.Errorf("branch name %q must not contain '@{'", name)
	}
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return fmt.Errorf("branch name %q must not begin or end with '/' or contain empty path components", name)
	}
	if strings.HasSuffix(name, ".") {
		return fmt.Errorf("branch name %q must not end with '.'", name)
	}
	for _, component := range strings.Split(name, "/") {
		if strings.HasPrefix(component, ".") {
			return fmt.Errorf("branch name %q has a path component starting with '.'", name)
		}
		if strings.HasSuffix(component, ".lock") {
			return fmt.Errorf("branch name %q has a path component ending with '.lock'", name)
		}
	}
	return nil
}

// ValidateBranchFields rejects branch references that are not valid git
// branch names. Empty fields are allowed: they select repository defaults.
func (r *CreateTaskRequest) ValidateBranchFields() error {
	fields := []struct {
		name  string
		value string
	}{
		{"base_branch", r.BaseBranch},
		{"branch_prefix", r.BranchPrefix},
		{"working_branch", r.WorkingBranch},
	}
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		if err := ValidateGitBranchName(f.value); err != nil {
			return fmt.Errorf("invalid %s: %w", f.name, err)
		}
	}
	return nil
}
