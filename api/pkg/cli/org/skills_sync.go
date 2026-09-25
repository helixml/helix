package org

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/client"
)

// Skills sync: a bot spec's `skills_dir` (a folder of <name>/SKILL.md skills,
// e.g. in the bot's own GitHub repo) is pushed into the bot project's primary
// Helix repo under .agents/skills/. Sandboxes clone that repo at start and link
// .agents/skills for the main session and for every instance, so a new instance
// picks up the synced skills. Skills that apply previously synced but are no
// longer in skills_dir are removed; skills added by hand in the repo are kept.

const skillsManifest = ".agents/skills/.synced-by-helix-apply"

// ensureBotRepo returns the clone URL (with credentials) of the bot project's
// primary repo, creating the project first if the bot was never activated.
func ensureBotRepo(ctx context.Context, c *client.HelixClient, orgID, botID string) (string, error) {
	detail, err := c.GetOrgBot(ctx, orgID, botID)
	if err != nil {
		return "", err
	}
	projectID := firstNonEmpty(detail.Bot.ProjectID, detail.ProjectID)
	if projectID == "" {
		ensureCtx, cancel := callCtx(ctx, 60*time.Second)
		defer cancel()
		ensured, err := c.EnsureOrgBotChat(ensureCtx, orgID, botID)
		if err != nil {
			return "", fmt.Errorf("ensure bot project: %w", err)
		}
		projectID = ensured.ProjectID
	}
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	if project.DefaultRepoID == "" {
		return "", fmt.Errorf("bot project %s has no primary repo yet (start the bot once)", projectID)
	}
	return c.GitCloneURL(project.DefaultRepoID)
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// syncSkills pushes skillsDir into the bot repo. Returns a one-line summary.
func syncSkills(ctx context.Context, c *client.HelixClient, orgID, botID, skillsDir, source string, dryRun bool) (string, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", fmt.Errorf("skills_dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(skillsDir, e.Name(), "SKILL.md")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("skills_dir %s has no <name>/SKILL.md skills", skillsDir)
	}
	sort.Strings(names)
	cloneURL, err := ensureBotRepo(ctx, c, orgID, botID)
	if err != nil {
		return "", err
	}
	secret := ""
	if u, err := url.Parse(cloneURL); err == nil {
		secret, _ = u.User.Password()
	}
	tmp, err := os.MkdirTemp("", "helix-skills-sync-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if _, err := git(tmp, "clone", "--quiet", "--depth", "1", cloneURL, "repo"); err != nil {
		return "", fmt.Errorf("clone bot repo: %w", redact(err, secret))
	}
	repo := filepath.Join(tmp, "repo")
	dest := filepath.Join(repo, ".agents", "skills")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	// Remove skills this command synced before that are gone from the source.
	if prev, err := os.ReadFile(filepath.Join(repo, skillsManifest)); err == nil {
		keep := map[string]bool{}
		for _, n := range names {
			keep[n] = true
		}
		for _, n := range strings.Fields(string(prev)) {
			if !keep[n] {
				_ = os.RemoveAll(filepath.Join(dest, n))
			}
		}
	}
	for _, n := range names {
		_ = os.RemoveAll(filepath.Join(dest, n))
		if err := copyTree(filepath.Join(skillsDir, n), filepath.Join(dest, n)); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(repo, skillsManifest), []byte(strings.Join(names, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}
	if _, err := git(repo, "add", "-A", ".agents/skills"); err != nil {
		return "", err
	}
	changed, _ := git(repo, "diff", "--cached", "--stat")
	if changed == "" {
		return fmt.Sprintf("skills up to date (%s)", strings.Join(names, ", ")), nil
	}
	if dryRun {
		return "skills would change:\n" + changed, nil
	}
	msg := "helix apply: sync skills " + strings.Join(names, ", ")
	if source != "" {
		msg += "\n\nSource: " + source
	}
	if _, err := git(repo, "-c", "user.name=helix apply", "-c", "user.email=apply@helix.local", "commit", "--quiet", "-m", msg); err != nil {
		return "", err
	}
	if _, err := git(repo, "push", "--quiet", "origin", "HEAD"); err != nil {
		return "", fmt.Errorf("push bot repo: %w", redact(err, secret))
	}
	sha, _ := git(repo, "rev-parse", "--short", "HEAD")
	return fmt.Sprintf("skills synced (%s) → bot repo %s; new instances pick them up, the main session on restart", strings.Join(names, ", "), sha), nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func redact(err error, secret string) error {
	if secret == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), url.QueryEscape(secret), "***")
	return fmt.Errorf("%s", strings.ReplaceAll(msg, secret, "***"))
}

// resolveProvider lets specs name a provider endpoint ("ds4-flash-node06")
// instead of an environment-specific id; ids and global/* pass through.
func resolveProvider(ctx context.Context, c *client.HelixClient, orgID, provider string) (string, error) {
	if provider == "" || strings.HasPrefix(provider, "pe_") || strings.HasPrefix(provider, "global/") {
		return provider, nil
	}
	eps, err := c.ListProviderEndpoints(ctx, &client.ProviderEndpointFilter{OrganizationID: orgID})
	if err != nil {
		return "", err
	}
	for _, e := range eps {
		if e.Name == provider {
			return e.ID, nil
		}
	}
	return "", fmt.Errorf("provider %q not found (helix provider list)", provider)
}

// providerName maps a provider endpoint id back to its name ("" if unknown).
func providerName(ctx context.Context, c *client.HelixClient, orgID, id string) string {
	if !strings.HasPrefix(id, "pe_") {
		return ""
	}
	eps, err := c.ListProviderEndpoints(ctx, &client.ProviderEndpointFilter{OrganizationID: orgID})
	if err != nil {
		return ""
	}
	for _, e := range eps {
		if e.ID == id {
			return e.Name
		}
	}
	return ""
}
