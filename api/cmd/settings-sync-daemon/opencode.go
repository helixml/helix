package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"archive/tar"
	"compress/gzip"
)

// Paths for the opencode runtime. Vars, not consts, so tests can redirect them
// at a tempdir.
var (
	// OpenCodeBakedBinary is the build shipped in the desktop image.
	OpenCodeBakedBinary = "/usr/local/bin/opencode"
	// OpenCodeBakedVersionFile records which release that binary is, written
	// at image build time. Reading a file beats spawning `opencode --version`
	// on every settings sync.
	OpenCodeBakedVersionFile = "/opt/helix/opencode.version"
	// OpenCodeCacheDir holds admin-pinned releases. Bind-mounted from the
	// sandbox host so every container on the host shares one download.
	OpenCodeCacheDir = "/opt/helix/agent-cache/opencode"
	// OpenCodeConfigHome isolates opencode's own config from anything else
	// under ~/.config.
	OpenCodeConfigHome = "/home/retro/.config/helix-opencode"
	// OpenCodeDataHome holds session state. It lives under the workspace so
	// sessions survive a container restart, matching QWEN_HOME.
	OpenCodeDataHome = "/home/retro/work/.opencode-state"
)

// openCodeDownloadTimeout bounds a single artifact download. The archive is
// ~60MB, so this is generous for a slow link while still failing a hung
// transfer quickly enough that it does not wedge a settings sync.
const openCodeDownloadTimeout = 3 * time.Minute

// openCodeRetryInterval throttles reinstall attempts after a failure.
// generateAgentServerConfig runs on every settings merge — including ones
// triggered by a file-watcher event — so without this a persistently broken
// pin would re-attempt a multi-minute download on every keystroke in Zed's
// settings.json. One attempt per poll cycle is enough to self-heal a
// transient outage.
const openCodeRetryInterval = PollInterval

// openCodeConfig is the opencode configuration we hand to the agent through
// OPENCODE_CONFIG_CONTENT. Everything the agent may do is declared here — we
// never rely on opencode's ambient defaults, because the container also
// carries OPENAI_API_KEY / ANTHROPIC_API_KEY for other runtimes and opencode
// would otherwise auto-register those providers and route around Helix.
type openCodeConfig struct {
	Schema           string                    `json:"$schema"`
	Model            string                    `json:"model"`
	SmallModel       string                    `json:"small_model"`
	Permission       map[string]string         `json:"permission"`
	Agent            map[string]openCodeAgent  `json:"agent,omitempty"`
	Autoupdate       bool                      `json:"autoupdate"`
	EnabledProviders []string                  `json:"enabled_providers"`
	Provider         map[string]openCodeVendor `json:"provider"`
}

type openCodeAgent struct {
	Steps int `json:"steps"`
}

type openCodeVendor struct {
	NPM     string                   `json:"npm"`
	Name    string                   `json:"name"`
	Options openCodeVendorOptions    `json:"options"`
	Models  map[string]openCodeModel `json:"models"`
}

type openCodeVendorOptions struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
}

type openCodeModel struct {
	Name       string                   `json:"name"`
	ToolCall   bool                     `json:"tool_call"`
	Attachment bool                     `json:"attachment,omitempty"`
	Modalities *openCodeModelModalities `json:"modalities,omitempty"`
	Limit      *openCodeModelLimit      `json:"limit,omitempty"`
	Options    *openCodeModelOption     `json:"options,omitempty"`
}

type openCodeModelModalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

type openCodeModelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type openCodeModelOption struct {
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

// openCodeProviderID is the single provider opencode is allowed to use. The
// model id the agent selects is "<openCodeProviderID>/<helix provider>/<model>"
// — opencode treats everything after the first slash as the model name, which
// is exactly the provider-prefixed id the Helix proxy routes on.
const openCodeProviderID = "helix"

// DS4 Flash is useful for fast tool work but can keep choosing another agentic
// step when its stop signal degrades. A bounded turn forces a text handoff
// instead of allowing an unbounded sequence of distinct calls that the exact
// duplicate detector cannot catch.
const openCodeDeepSeekV4FlashSteps = 30

// buildOpenCodeConfig renders the config for the current code agent settings.
func (d *SettingsDaemon) buildOpenCodeConfig(baseURL string) openCodeConfig {
	modelID := d.codeAgentConfig.Model
	qualified := openCodeProviderID + "/" + modelID

	model := openCodeModel{
		Name:     strings.TrimPrefix(modelID, d.codeAgentConfig.Provider+"/"),
		ToolCall: true,
	}
	if len(d.codeAgentConfig.InputModalities) > 0 || len(d.codeAgentConfig.OutputModalities) > 0 {
		model.Modalities = &openCodeModelModalities{
			Input:  d.codeAgentConfig.InputModalities,
			Output: d.codeAgentConfig.OutputModalities,
		}
		for _, modality := range d.codeAgentConfig.InputModalities {
			if modality != "text" {
				model.Attachment = true
				break
			}
		}
	}
	// Only declare limits we actually know. Writing zeros would tell opencode
	// the context window is empty and it would compact after every turn.
	if d.codeAgentConfig.MaxTokens > 0 || d.codeAgentConfig.MaxOutputTokens > 0 {
		model.Limit = &openCodeModelLimit{
			Context: d.codeAgentConfig.MaxTokens,
			Output:  d.codeAgentConfig.MaxOutputTokens,
		}
	}
	if d.codeAgentConfig.ReasoningEffort != "" {
		model.Options = &openCodeModelOption{ReasoningEffort: d.codeAgentConfig.ReasoningEffort}
	}

	config := openCodeConfig{
		Schema:     "https://opencode.ai/config.json",
		Model:      qualified,
		SmallModel: qualified,
		// Headless sandbox: nobody is there to answer a permission prompt, so
		// the agent would stall on the first edit. This is opencode's
		// equivalent of the --yolo flag we pass to qwen.
		Permission: map[string]string{
			"*":                  "allow",
			"external_directory": "allow",
			// OpenCode raises doom_loop after three identical tool calls. A blanket
			// allow disabled that safety mechanism in our headless runtime.
			"doom_loop": "deny",
		},
		// Never let opencode swap its own binary mid-session: the running
		// version is pinned by the image or by the admin override, and an
		// in-place upgrade would bypass both plus our digest check.
		Autoupdate: false,
		// The gate that keeps every request inside the Helix proxy.
		EnabledProviders: []string{openCodeProviderID},
		Provider: map[string]openCodeVendor{
			openCodeProviderID: {
				NPM:  "@ai-sdk/openai-compatible",
				Name: "Helix",
				Options: openCodeVendorOptions{
					BaseURL: baseURL,
					// Resolved by opencode from the env we set on the agent
					// server, so the key never appears in Zed's settings.json.
					APIKey: "{env:HELIX_API_KEY}",
				},
				Models: map[string]openCodeModel{modelID: model},
			},
		},
	}
	if strings.HasSuffix(strings.ToLower(modelID), "deepseek-v4-flash") {
		config.Agent = map[string]openCodeAgent{
			"build": {Steps: openCodeDeepSeekV4FlashSteps},
		}
	}
	return config
}

// resolveOpenCodeCommand returns the opencode binary this session must run.
//
// With no admin override (the common case) this is the baked binary and the
// function touches the network zero times. With an override it downloads,
// verifies and caches the pinned release. It returns an error rather than the
// baked path when a pinned release cannot be installed: silently running the
// bundled build would leave an admin believing a rollout had landed when it
// had not.
func (d *SettingsDaemon) resolveOpenCodeCommand() (string, error) {
	pinned := d.codeAgentConfig.OpenCodeBinary
	if pinned == nil || pinned.Version == "" {
		return OpenCodeBakedBinary, nil
	}

	if baked := readOpenCodeBakedVersion(); baked == pinned.Version {
		log.Printf("opencode: pinned version %s matches the bundled build; using %s", pinned.Version, OpenCodeBakedBinary)
		return OpenCodeBakedBinary, nil
	}

	artifact, ok := pinned.Artifacts[runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("pinned opencode %s publishes no %s build", pinned.Version, runtime.GOARCH)
	}

	installed := filepath.Join(OpenCodeCacheDir, pinned.Version, "opencode")
	if fi, err := os.Stat(installed); err == nil && fi.Mode()&0111 != 0 {
		return installed, nil
	}

	// Back off after a failure so a broken pin cannot turn every settings
	// merge into another download attempt.
	if since := time.Since(d.openCodeLastAttempt); !d.openCodeLastAttempt.IsZero() && since < openCodeRetryInterval {
		return "", fmt.Errorf("opencode %s failed to install %s ago; retrying in %s",
			pinned.Version, since.Round(time.Second), (openCodeRetryInterval - since).Round(time.Second))
	}
	d.openCodeLastAttempt = time.Now()

	if err := installOpenCode(installed, artifact); err != nil {
		return "", fmt.Errorf("install opencode %s: %w", pinned.Version, err)
	}
	d.openCodeLastAttempt = time.Time{}
	log.Printf("opencode: installed pinned version %s at %s", pinned.Version, installed)
	return installed, nil
}

// readOpenCodeBakedVersion returns the release baked into the image, or "" if
// the marker file is missing. An empty result is treated as "unknown", which
// forces an override to install rather than assuming a match.
func readOpenCodeBakedVersion() string {
	data, err := os.ReadFile(OpenCodeBakedVersionFile)
	if err != nil {
		log.Printf("opencode: could not read %s (%v); assuming the bundled version is unknown", OpenCodeBakedVersionFile, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

// installOpenCode downloads artifact, verifies its digest, and extracts the
// binary to dest.
//
// The download is hashed and extracted into a temp file next to dest and only
// then renamed, so a torn transfer can never be executed and two containers
// racing on the shared host cache cannot read each other's partial file.
func installOpenCode(dest string, artifact CodeAgentBinaryArtifact) error {
	if artifact.SHA256 == "" {
		return fmt.Errorf("artifact has no sha256; refusing to run an unverified binary")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	client := &http.Client{Timeout: openCodeDownloadTimeout}
	resp, err := client.Get(artifact.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", artifact.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: status %d", artifact.URL, resp.StatusCode)
	}

	// Hash the archive while streaming it to disk so we neither buffer 60MB in
	// memory nor read the body twice.
	archive, err := os.CreateTemp(filepath.Dir(dest), ".opencode-archive-*")
	if err != nil {
		return fmt.Errorf("create temp archive: %w", err)
	}
	defer os.Remove(archive.Name())
	defer archive.Close()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(archive, hasher), resp.Body); err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, artifact.SHA256) {
		return fmt.Errorf("archive digest mismatch: expected %s, got %s", artifact.SHA256, got)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind archive: %w", err)
	}

	binary, err := os.CreateTemp(filepath.Dir(dest), ".opencode-bin-*")
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}
	defer os.Remove(binary.Name())

	if err := extractOpenCodeBinary(archive, binary); err != nil {
		binary.Close()
		return err
	}
	if err := binary.Close(); err != nil {
		return fmt.Errorf("close temp binary: %w", err)
	}
	if err := os.Chmod(binary.Name(), 0755); err != nil {
		return fmt.Errorf("chmod binary: %w", err)
	}
	if err := os.Rename(binary.Name(), dest); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	return nil
}

// extractOpenCodeBinary pulls the single executable out of opencode's release
// tarball. The archive holds one regular file; anything else is a change in
// the release layout we want to fail loudly on rather than guess about.
func extractOpenCodeBinary(archive io.Reader, dest io.Writer) error {
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			if !found {
				return fmt.Errorf("archive contains no opencode binary")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != "opencode" {
			return fmt.Errorf("archive contains unexpected regular file %q", header.Name)
		}
		if found {
			return fmt.Errorf("archive contains multiple opencode binaries")
		}
		if _, err := io.Copy(dest, tr); err != nil {
			return fmt.Errorf("extract %s: %w", header.Name, err)
		}
		found = true
	}
}

// marshalOpenCodeConfig renders the config as the compact JSON we pass through
// an environment variable.
func marshalOpenCodeConfig(cfg openCodeConfig) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal opencode config: %w", err)
	}
	return string(data), nil
}
