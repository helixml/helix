package hydra

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

// An Org Bot home is the bot's /home/retro/work: Chrome profiles
// (.chrome-state and custom ones), scripts, local backups. It belongs to the
// (organization, bot) pair, not to a session, so replacing the bot's session
// keeps it. Layout under workspacesBaseDir:
//
//	org-bots/<org>/<bot>/owner.json  hydra's record; never mounted
//	org-bots/<org>/<bot>/home/       bind-mounted at /home/retro/work
//	org-bots/<org>/<bot>/migration.json  present only while adopting a legacy workspace
//	org-bots-retired/<org>/<bot>/<generation>-<unix>/  homes retired by an explicit reset
//	org-bots-trash/<name>  homes being purged; the reaper retries removal
//
// A session's own workspace (sessions/<ses>) is never the bot home, so
// DestroyDevContainer cannot remove it.

const (
	botHomesDirName        = "org-bots"
	botHomesRetiredDirName = "org-bots-retired"
	botHomesTrashDirName   = "org-bots-trash"
	botHomeOwnerFile       = "owner.json"
	botHomeMigrationFile   = "migration.json"
	botHomeDataDir         = "home"
	// containerBotHomeLabel marks a container that mounts a bot home, so a
	// second session of the same bot can be refused while one is running.
	containerBotHomeLabel = "helix.bot_home"
	// DefaultBotHomeRetention is how long a retired home is kept for rollback.
	DefaultBotHomeRetention = 7 * 24 * time.Hour
)

// BotHomeExpectation is the control plane's knowledge of the home.
type BotHomeExpectation string

const (
	// BotHomeExpectNew: no home is recorded as established. Hydra adopts the
	// session's legacy workspace or creates an empty home, and is idempotent
	// for a home it already created with the same generation.
	BotHomeExpectNew BotHomeExpectation = "new"
	// BotHomeExpectExisting: the home was established on this runner. A
	// missing or different home is an error; hydra never seeds a fresh one.
	BotHomeExpectExisting BotHomeExpectation = "existing"
)

// BotHomeSpec identifies a bot home and what the control plane expects of it.
type BotHomeSpec struct {
	OrganizationID string             `json:"organization_id"`
	BotID          string             `json:"bot_id"`
	Generation     string             `json:"generation"`
	Expect         BotHomeExpectation `json:"expect"`
}

// BotHomeOutcome says how CreateDevContainer bound the home.
type BotHomeOutcome string

const (
	BotHomeOutcomeExisting BotHomeOutcome = "existing"
	BotHomeOutcomeCreated  BotHomeOutcome = "created"
	BotHomeOutcomeMigrated BotHomeOutcome = "migrated"
)

// BotHomeResult reports the bound home. It carries no file contents.
type BotHomeResult struct {
	Path                string           `json:"path"`
	Generation          string           `json:"generation"`
	Outcome             BotHomeOutcome   `json:"outcome"`
	MigratedFromSession string           `json:"migrated_from_session,omitempty"`
	Inventory           BotHomeInventory `json:"inventory"`
}

// BotHomeInventory describes a home without reading secrets: entry counts and
// Chrome profile metadata only. Cookie and login databases are stat'ed, never
// opened.
type BotHomeInventory struct {
	TopLevelEntries int                   `json:"top_level_entries"`
	ChromeProfiles  []ChromeProfileStatus `json:"chrome_profiles,omitempty"`
	// BrowserProfile is the profile declared in .helix/browser.json (see
	// desktop/shared/helix-browser.sh); nil when none is declared, in which
	// case the browser uses .chrome-state.
	BrowserProfile *DeclaredBrowserProfile `json:"browser_profile,omitempty"`
}

// DeclaredBrowserProfileState says whether the declared profile can be used.
// Hydra only reports it: a bot without a usable profile still starts, and its
// browser requests fail with the same reason.
type DeclaredBrowserProfileState string

const (
	DeclaredBrowserProfileOK                 DeclaredBrowserProfileState = "ok"
	DeclaredBrowserProfileMissing            DeclaredBrowserProfileState = "missing"
	DeclaredBrowserProfileOutsideWorkspace   DeclaredBrowserProfileState = "outside_workspace"
	DeclaredBrowserProfileWrongIdentity      DeclaredBrowserProfileState = "wrong_identity"
	DeclaredBrowserProfileInconsistent       DeclaredBrowserProfileState = "inconsistent"
	DeclaredBrowserProfileInvalidDeclaration DeclaredBrowserProfileState = "invalid_declaration"
)

type DeclaredBrowserProfile struct {
	Path  string                      `json:"path,omitempty"`
	State DeclaredBrowserProfileState `json:"state"`
}

// ChromeProfileStatus is the integrity of one Chrome user-data dir. It cannot
// say whether an account is still logged in; that needs the browser.
type ChromeProfileStatus struct {
	Path              string `json:"path"`
	LocalStateValid   bool   `json:"local_state_valid"`
	DefaultPrefsValid bool   `json:"default_prefs_valid"`
	CookiesDBPresent  bool   `json:"cookies_db_present"`
	SingletonLock     bool   `json:"singleton_lock"`
}

// botHomeOwner is owner.json. Only hydra writes it, outside the mounted tree,
// so the bot cannot forge which bot or generation a directory belongs to.
type botHomeOwner struct {
	OrganizationID      string    `json:"organization_id"`
	BotID               string    `json:"bot_id"`
	Generation          string    `json:"generation"`
	CreatedAt           time.Time `json:"created_at"`
	Origin              string    `json:"origin"`
	MigratedFromSession string    `json:"migrated_from_session,omitempty"`
}

type botHomeMigration struct {
	Generation string `json:"generation"`
	SessionID  string `json:"session_id"`
	From       string `json:"from"`
	To         string `json:"to"`
}

// BotHomeKey is the identity used in labels and GC live sets.
func BotHomeKey(orgID, botID string) string { return orgID + "/" + botID }

var safePathElement = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// botHomePathElement maps an id to a directory name. Ids made of safe
// characters are used as is; anything else is hex-encoded behind "~", which
// a safe id can never start with, so two ids never share a directory.
func botHomePathElement(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, "/\\\x00") {
		return "", fmt.Errorf("invalid bot home id %q", id)
	}
	if safePathElement.MatchString(id) && !strings.Contains(id, "..") {
		return id, nil
	}
	return "~" + hex.EncodeToString([]byte(id)), nil
}

func validateBotHomeIDs(orgID, botID string) (orgDir, botDir string, err error) {
	if orgDir, err = botHomePathElement(orgID); err != nil {
		return "", "", fmt.Errorf("organization: %w", err)
	}
	if botDir, err = botHomePathElement(botID); err != nil {
		return "", "", fmt.Errorf("bot: %w", err)
	}
	return orgDir, botDir, nil
}

// BotHomeRelDir returns the home's path relative to the workspaces base, so
// the API can build the mount source with its own base path. Hydra rejects
// any source that differs from its own BotHomeDir.
func BotHomeRelDir(orgID, botID string) (string, error) {
	orgDir, botDir, err := validateBotHomeIDs(orgID, botID)
	if err != nil {
		return "", err
	}
	return filepath.Join(botHomesDirName, orgDir, botDir, botHomeDataDir), nil
}

// BotHomeRoot returns org-bots/<org>/<bot> for the given ids.
func BotHomeRoot(orgID, botID string) (string, error) {
	orgDir, botDir, err := validateBotHomeIDs(orgID, botID)
	if err != nil {
		return "", err
	}
	return filepath.Join(workspacesBaseDir, botHomesDirName, orgDir, botDir), nil
}

// BotHomeDir returns the directory mounted at /home/retro/work.
func BotHomeDir(orgID, botID string) (string, error) {
	root, err := BotHomeRoot(orgID, botID)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, botHomeDataDir), nil
}

type dirState int

const (
	dirMissing dirState = iota
	dirEmpty
	dirPopulated
)

func (d dirState) String() string {
	return [...]string{"missing", "empty", "populated"}[d]
}

func statDir(path string) (dirState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return dirMissing, nil
	}
	if err != nil {
		return dirMissing, err
	}
	if !info.IsDir() {
		return dirMissing, fmt.Errorf("%s is not a directory", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return dirMissing, err
	}
	defer f.Close()
	if _, err := f.Readdirnames(1); errors.Is(err, io.EOF) {
		return dirEmpty, nil
	} else if err != nil {
		return dirMissing, err
	}
	return dirPopulated, nil
}

func readJSONFile(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("parse %s: %w", path, err)
	}
	return true, nil
}

// writeJSONFileAtomic writes v so a crash leaves either the old or the new
// file, never a torn one.
func writeJSONFileAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func legacySessionWorkspace(sessionID string) string {
	return filepath.Join(workspacesBaseDir, "sessions", sessionID)
}

// prepareBotHome binds a bot home for sessionID according to spec. It never
// overwrites a populated directory and never creates an empty home when the
// control plane says one already exists.
func prepareBotHome(spec *BotHomeSpec, sessionID string) (*BotHomeResult, error) {
	if spec == nil {
		return nil, errors.New("bot home spec is required")
	}
	if !isResourceID(sessionID, "ses_") {
		return nil, fmt.Errorf("invalid session id %q", sessionID)
	}
	if spec.Generation == "" || !safePathElement.MatchString(spec.Generation) {
		return nil, fmt.Errorf("invalid bot home generation %q", spec.Generation)
	}
	if spec.Expect != BotHomeExpectNew && spec.Expect != BotHomeExpectExisting {
		return nil, fmt.Errorf("invalid bot home expectation %q", spec.Expect)
	}
	root, err := BotHomeRoot(spec.OrganizationID, spec.BotID)
	if err != nil {
		return nil, err
	}
	home := filepath.Join(root, botHomeDataDir)
	ownerPath := filepath.Join(root, botHomeOwnerFile)
	legacy := legacySessionWorkspace(sessionID)

	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create bot home root: %w", err)
	}

	var owner botHomeOwner
	hasOwner, err := readJSONFile(ownerPath, &owner)
	if err != nil {
		return nil, fmt.Errorf("read bot home owner record: %w", err)
	}
	if hasOwner && (owner.OrganizationID != spec.OrganizationID || owner.BotID != spec.BotID) {
		return nil, fmt.Errorf("bot home %s belongs to %s, not %s", root,
			BotHomeKey(owner.OrganizationID, owner.BotID), BotHomeKey(spec.OrganizationID, spec.BotID))
	}

	var migration botHomeMigration
	hasMigration, err := readJSONFile(filepath.Join(root, botHomeMigrationFile), &migration)
	if err != nil {
		return nil, fmt.Errorf("read bot home migration journal: %w", err)
	}
	if hasMigration {
		if err := finishBotHomeMigration(spec, root, migration); err != nil {
			return nil, err
		}
		if _, err := readJSONFile(ownerPath, &owner); err != nil {
			return nil, fmt.Errorf("read bot home owner record: %w", err)
		}
		hasOwner = true
	}

	homeState, err := statDir(home)
	if err != nil {
		return nil, fmt.Errorf("inspect bot home: %w", err)
	}
	legacyState, err := statDir(legacy)
	if err != nil {
		return nil, fmt.Errorf("inspect legacy session workspace: %w", err)
	}

	result := &BotHomeResult{Path: home, Generation: spec.Generation}
	switch {
	case hasOwner:
		if owner.Generation != spec.Generation {
			return nil, fmt.Errorf("bot home %s has generation %s but the control plane expects %s; refusing to use or replace it",
				home, owner.Generation, spec.Generation)
		}
		if homeState == dirMissing {
			return nil, fmt.Errorf("bot home %s is recorded but its data directory is missing; refusing to seed a fresh home", home)
		}
		if legacyState == dirPopulated {
			return nil, fmt.Errorf("both bot home %s and legacy session workspace %s contain data; refusing to choose or overwrite either", home, legacy)
		}
		result.Outcome = BotHomeOutcomeExisting
		if hasMigration {
			result.Outcome = BotHomeOutcomeMigrated
			result.MigratedFromSession = migration.SessionID
		}
	case spec.Expect == BotHomeExpectExisting:
		return nil, fmt.Errorf("bot home for %s is not present on this runner (data dir %s); refusing to seed a fresh home",
			BotHomeKey(spec.OrganizationID, spec.BotID), homeState)
	case homeState != dirMissing:
		return nil, fmt.Errorf("bot home %s exists without an owner record; refusing to adopt data of unknown origin", home)
	case legacyState == dirPopulated:
		journal := botHomeMigration{Generation: spec.Generation, SessionID: sessionID, From: legacy, To: home}
		if err := writeJSONFileAtomic(filepath.Join(root, botHomeMigrationFile), journal); err != nil {
			return nil, fmt.Errorf("write bot home migration journal: %w", err)
		}
		if err := finishBotHomeMigration(spec, root, journal); err != nil {
			return nil, err
		}
		result.Outcome = BotHomeOutcomeMigrated
		result.MigratedFromSession = sessionID
	default:
		if legacyState == dirEmpty {
			if err := os.Remove(legacy); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("remove empty legacy session workspace: %w", err)
			}
		}
		if err := os.Mkdir(home, 0o755); err != nil {
			return nil, fmt.Errorf("create bot home: %w", err)
		}
		if err := writeJSONFileAtomic(ownerPath, botHomeOwner{
			OrganizationID: spec.OrganizationID, BotID: spec.BotID, Generation: spec.Generation,
			CreatedAt: time.Now().UTC(), Origin: string(BotHomeOutcomeCreated),
		}); err != nil {
			return nil, fmt.Errorf("write bot home owner record: %w", err)
		}
		result.Outcome = BotHomeOutcomeCreated
	}
	result.Inventory = inventoryBotHome(home)
	return result, nil
}

// finishBotHomeMigration completes a journaled rename of a legacy session
// workspace into the bot home. Every step is idempotent, so a crash at any
// point resumes on the next start.
func finishBotHomeMigration(spec *BotHomeSpec, root string, j botHomeMigration) error {
	home := filepath.Join(root, botHomeDataDir)
	if j.Generation != spec.Generation || j.To != home || !isResourceID(j.SessionID, "ses_") || j.From != legacySessionWorkspace(j.SessionID) {
		return fmt.Errorf("bot home migration journal in %s does not match this start (generation %s, from %s); resolve it manually", root, j.Generation, j.From)
	}
	fromState, err := statDir(j.From)
	if err != nil {
		return fmt.Errorf("inspect migration source: %w", err)
	}
	toState, err := statDir(j.To)
	if err != nil {
		return fmt.Errorf("inspect migration destination: %w", err)
	}
	switch {
	case fromState != dirMissing && toState == dirMissing:
		if err := os.Rename(j.From, j.To); err != nil {
			return fmt.Errorf("move legacy session workspace %s to bot home %s: %w", j.From, j.To, err)
		}
		if err := syncDir(root); err != nil {
			return fmt.Errorf("sync bot home root: %w", err)
		}
	case fromState == dirMissing && toState != dirMissing:
		// Rename already happened.
	case fromState != dirMissing && toState != dirMissing:
		return fmt.Errorf("interrupted migration left both %s (%s) and %s (%s); refusing to overwrite either", j.From, fromState, j.To, toState)
	default:
		return fmt.Errorf("interrupted migration lost both %s and %s; refusing to seed a fresh home", j.From, j.To)
	}
	ownerPath := filepath.Join(root, botHomeOwnerFile)
	var owner botHomeOwner
	hasOwner, err := readJSONFile(ownerPath, &owner)
	if err != nil {
		return fmt.Errorf("read bot home owner record: %w", err)
	}
	if !hasOwner {
		if err := writeJSONFileAtomic(ownerPath, botHomeOwner{
			OrganizationID: spec.OrganizationID, BotID: spec.BotID, Generation: spec.Generation,
			CreatedAt: time.Now().UTC(), Origin: string(BotHomeOutcomeMigrated), MigratedFromSession: j.SessionID,
		}); err != nil {
			return fmt.Errorf("write bot home owner record: %w", err)
		}
	}
	if err := os.Remove(filepath.Join(root, botHomeMigrationFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear bot home migration journal: %w", err)
	}
	log.Info().Str("bot_home", BotHomeKey(spec.OrganizationID, spec.BotID)).Str("from", j.From).Str("to", j.To).
		Msg("Migrated legacy session workspace into bot home")
	return nil
}

// inventoryBotHome reports Chrome profile integrity without opening cookie or
// login databases: the default .chrome-state plus any top-level directory
// that is a Chrome user-data dir (has "Local State").
func inventoryBotHome(home string) BotHomeInventory {
	var inv BotHomeInventory
	entries, err := os.ReadDir(home)
	if err != nil {
		return inv
	}
	inv.TopLevelEntries = len(entries)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(home, entry.Name())
		if entry.Name() != ".chrome-state" {
			if _, err := os.Stat(filepath.Join(dir, "Local State")); err != nil {
				continue
			}
		}
		inv.ChromeProfiles = append(inv.ChromeProfiles, chromeProfileStatus(home, dir))
	}
	inv.BrowserProfile = declaredBrowserProfile(home)
	return inv
}

// declaredBrowserProfile checks the declared profile the way helix-browser
// does before launching: inside the home, present, carrying the declared
// identity, with a readable Local State. The home is bot-writable, so paths
// are confined to it after resolving symlinks.
func declaredBrowserProfile(home string) *DeclaredBrowserProfile {
	var decl struct {
		Profile   string `json:"profile"`
		ProfileID string `json:"profile_id"`
	}
	found, err := readJSONFile(filepath.Join(home, ".helix", "browser.json"), &decl)
	if !found {
		return nil
	}
	status := &DeclaredBrowserProfile{Path: decl.Profile}
	rel := filepath.Clean(decl.Profile)
	if err != nil || decl.ProfileID == "" || decl.Profile == "" || filepath.IsAbs(rel) || !filepath.IsLocal(rel) {
		status.State = DeclaredBrowserProfileInvalidDeclaration
		return status
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		status.State = DeclaredBrowserProfileInconsistent
		return status
	}
	dir, err := filepath.EvalSymlinks(filepath.Join(home, rel))
	if errors.Is(err, fs.ErrNotExist) {
		status.State = DeclaredBrowserProfileMissing
		return status
	}
	if err != nil {
		status.State = DeclaredBrowserProfileInconsistent
		return status
	}
	if inside, relErr := filepath.Rel(realHome, dir); relErr != nil || !filepath.IsLocal(inside) {
		status.State = DeclaredBrowserProfileOutsideWorkspace
		return status
	}
	var localState map[string]any
	if ok, err := readJSONFile(filepath.Join(dir, "Local State"), &localState); !ok || err != nil {
		status.State = DeclaredBrowserProfileInconsistent
		return status
	}
	var identity struct {
		ProfileID string `json:"profile_id"`
	}
	if ok, err := readJSONFile(filepath.Join(dir, ".helix-profile.json"), &identity); !ok || err != nil || identity.ProfileID != decl.ProfileID {
		status.State = DeclaredBrowserProfileWrongIdentity
		return status
	}
	status.State = DeclaredBrowserProfileOK
	return status
}

func chromeProfileStatus(home, dir string) ChromeProfileStatus {
	rel, _ := filepath.Rel(home, dir)
	status := ChromeProfileStatus{Path: rel}
	var v map[string]any
	if ok, err := readJSONFile(filepath.Join(dir, "Local State"), &v); ok && err == nil {
		status.LocalStateValid = true
	}
	if ok, err := readJSONFile(filepath.Join(dir, "Default", "Preferences"), &v); ok && err == nil {
		status.DefaultPrefsValid = true
	}
	if _, err := os.Stat(filepath.Join(dir, "Default", "Cookies")); err == nil {
		status.CookiesDBPresent = true
	} else if _, err := os.Stat(filepath.Join(dir, "Default", "Network", "Cookies")); err == nil {
		status.CookiesDBPresent = true
	}
	if _, err := os.Lstat(filepath.Join(dir, "SingletonLock")); err == nil {
		status.SingletonLock = true
	}
	return status
}

// BotHomeDestroyMode chooses what an erase does with the data.
type BotHomeDestroyMode string

const (
	// BotHomeRetire moves the home to org-bots-retired for rollback; the
	// reaper purges it after the retention period. Used by explicit reset.
	BotHomeRetire BotHomeDestroyMode = "retire"
	// BotHomePurge deletes the home now. Used when the bot itself is deleted.
	BotHomePurge BotHomeDestroyMode = "purge"
)

// DestroyBotHomeRequest erases one bot's home.
type DestroyBotHomeRequest struct {
	OrganizationID string `json:"organization_id"`
	BotID          string `json:"bot_id"`
	// Generation, when set, must match the home on disk; a different
	// generation is never erased.
	Generation string             `json:"generation,omitempty"`
	Mode       BotHomeDestroyMode `json:"mode"`
}

// DestroyBotHomeResponse says exactly what happened to the data.
type DestroyBotHomeResponse struct {
	Existed     bool   `json:"existed"`
	Generation  string `json:"generation,omitempty"`
	RetiredPath string `json:"retired_path,omitempty"`
	Purged      bool   `json:"purged"`
}

// ErrBotHomeInUse is returned when a running container still mounts the home.
var ErrBotHomeInUse = errors.New("bot home is mounted by a running container")

// destroyBotHome removes (purge) or retires one bot's home. The root is
// renamed away first, so the home is either fully present or fully gone; a
// purge that fails after the rename leaves the data in org-bots-trash, which
// the error names and the reaper retries.
func destroyBotHome(req *DestroyBotHomeRequest, now time.Time) (*DestroyBotHomeResponse, error) {
	if req.Mode != BotHomeRetire && req.Mode != BotHomePurge {
		return nil, fmt.Errorf("invalid bot home destroy mode %q", req.Mode)
	}
	if req.Generation != "" && !safePathElement.MatchString(req.Generation) {
		return nil, fmt.Errorf("invalid bot home generation %q", req.Generation)
	}
	orgDir, botDir, err := validateBotHomeIDs(req.OrganizationID, req.BotID)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(workspacesBaseDir, botHomesDirName, orgDir, botDir)
	rootState, err := statDir(root)
	if err != nil {
		return nil, fmt.Errorf("inspect bot home: %w", err)
	}
	if rootState == dirMissing {
		return &DestroyBotHomeResponse{}, nil
	}
	var owner botHomeOwner
	hasOwner, err := readJSONFile(filepath.Join(root, botHomeOwnerFile), &owner)
	if err != nil {
		return nil, fmt.Errorf("read bot home owner record: %w", err)
	}
	if hasOwner && (owner.OrganizationID != req.OrganizationID || owner.BotID != req.BotID) {
		return nil, fmt.Errorf("bot home %s belongs to %s", root, BotHomeKey(owner.OrganizationID, owner.BotID))
	}
	if req.Generation != "" && hasOwner && owner.Generation != req.Generation {
		return nil, fmt.Errorf("bot home generation is %s, not %s; refusing to erase it", owner.Generation, req.Generation)
	}
	resp := &DestroyBotHomeResponse{Existed: true, Generation: owner.Generation}
	generation := owner.Generation
	if generation == "" {
		generation = "unrecorded"
	}
	stamp := fmt.Sprintf("%s-%d", generation, now.UnixNano())

	if req.Mode == BotHomeRetire {
		dest := filepath.Join(workspacesBaseDir, botHomesRetiredDirName, orgDir, botDir, stamp)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("create retired bot home dir: %w", err)
		}
		if err := os.Rename(root, dest); err != nil {
			return nil, fmt.Errorf("retire bot home %s: %w (data is still at %s)", root, err, root)
		}
		resp.RetiredPath = dest
		return resp, nil
	}

	trash := filepath.Join(workspacesBaseDir, botHomesTrashDirName, orgDir+"__"+botDir+"__"+stamp)
	if err := os.MkdirAll(filepath.Dir(trash), 0o755); err != nil {
		return nil, fmt.Errorf("create bot home trash dir: %w", err)
	}
	if err := os.Rename(root, trash); err != nil {
		return nil, fmt.Errorf("purge bot home %s: %w (data is still at %s)", root, err, root)
	}
	if err := os.RemoveAll(trash); err != nil {
		return nil, fmt.Errorf("purge bot home: %w (data may still exist at %s; the reaper retries)", err, trash)
	}
	resp.Purged = true
	return resp, nil
}

// runningBotHomeSessions lists running containers that mount the bot home.
func runningBotHomeSessions(ctx context.Context, dockerClient *client.Client, key string) ([]string, error) {
	list, err := dockerClient.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", containerBotHomeLabel+"="+key)),
	})
	if err != nil {
		return nil, fmt.Errorf("list containers using bot home %s: %w", key, err)
	}
	var sessions []string
	for _, c := range list {
		sessions = append(sessions, c.Labels[containerSessionIDLabel])
	}
	return sessions, nil
}

// bindBotHome prepares the bot home for a CreateDevContainer request. It runs
// before any mount source is created so a missing home can never be replaced
// by an empty directory.
func (dm *DevContainerManager) bindBotHome(ctx context.Context, dockerClient *client.Client, req *CreateDevContainerRequest) (*BotHomeResult, error) {
	spec := req.BotHome
	home, err := BotHomeDir(spec.OrganizationID, spec.BotID)
	if err != nil {
		return nil, err
	}
	workspaceMounts := 0
	for _, m := range req.Mounts {
		if m.Type == "volume" {
			continue
		}
		if m.Destination == "/home/retro/work" || m.Source == legacySessionWorkspace(req.SessionID) {
			if m.Source != home {
				return nil, fmt.Errorf("workspace mount %s -> %s does not match bot home %s", m.Source, m.Destination, home)
			}
		}
		if m.Source == home {
			workspaceMounts++
		}
	}
	if workspaceMounts == 0 {
		return nil, fmt.Errorf("request has no workspace mount for bot home %s", home)
	}
	key := BotHomeKey(spec.OrganizationID, spec.BotID)

	// The caller holds reserveBotHome(key) until the container is started.
	sessions, err := runningBotHomeSessions(ctx, dockerClient, key)
	if err != nil {
		return nil, err
	}
	for _, other := range sessions {
		if other != req.SessionID {
			return nil, fmt.Errorf("%w: bot home %s is in use by session %s", ErrBotHomeInUse, key, other)
		}
	}
	// An older container of this session may still point at the legacy
	// workspace. Never start it again over the migrated data.
	if existing, err := dockerClient.ContainerInspect(ctx, req.ContainerName); err == nil {
		if !containerMountsSource(existing.Mounts, home, "/home/retro/work") {
			if existing.State != nil && existing.State.Running {
				return nil, fmt.Errorf("session %s is running with a session-keyed workspace; stop it before binding its bot home", req.SessionID)
			}
			if err := dockerClient.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
				return nil, fmt.Errorf("remove container bound to the legacy workspace: %w", err)
			}
		}
	} else if !client.IsErrNotFound(err) {
		return nil, fmt.Errorf("inspect existing container %s: %w", req.ContainerName, err)
	}
	result, err := prepareBotHome(spec, req.SessionID)
	if err != nil {
		return nil, err
	}
	log.Info().
		Str("session_id", req.SessionID).
		Str("bot_home", key).
		Str("generation", result.Generation).
		Str("outcome", string(result.Outcome)).
		Int("top_level_entries", result.Inventory.TopLevelEntries).
		Interface("chrome_profiles", result.Inventory.ChromeProfiles).
		Interface("browser_profile", result.Inventory.BrowserProfile).
		Msg("Bound Org Bot home")
	if bp := result.Inventory.BrowserProfile; bp != nil && bp.State != DeclaredBrowserProfileOK {
		log.Warn().Str("session_id", req.SessionID).Str("bot_home", key).
			Str("profile", bp.Path).Str("state", string(bp.State)).
			Msg("Org Bot's declared browser profile is not usable; browser requests will fail until it is restored")
	}
	return result, nil
}

func containerMountsSource(mounts []dockertypes.MountPoint, source, destination string) bool {
	for _, m := range mounts {
		if m.Source == source && m.Destination == destination {
			return true
		}
	}
	return false
}

// DestroyBotHome erases a bot home after checking no container mounts it.
func (dm *DevContainerManager) DestroyBotHome(ctx context.Context, req *DestroyBotHomeRequest) (*DestroyBotHomeResponse, error) {
	if _, _, err := validateBotHomeIDs(req.OrganizationID, req.BotID); err != nil {
		return nil, err
	}
	dockerClient, err := dm.getDockerClient("")
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	defer dockerClient.Close()
	key := BotHomeKey(req.OrganizationID, req.BotID)
	release := dm.reserveBotHome(key)
	defer release()
	sessions, err := runningBotHomeSessions(ctx, dockerClient, key)
	if err != nil {
		return nil, err
	}
	if len(sessions) > 0 {
		return nil, fmt.Errorf("%w: %s by session(s) %s", ErrBotHomeInUse, key, strings.Join(sessions, ","))
	}
	resp, err := destroyBotHome(req, time.Now())
	if err != nil {
		return nil, err
	}
	log.Info().Str("bot_home", key).Str("mode", string(req.Mode)).Bool("existed", resp.Existed).
		Str("retired_path", resp.RetiredPath).Bool("purged", resp.Purged).Msg("Destroyed Org Bot home")
	return resp, nil
}

// ReconcileOrphanBotHomes purges homes of bots that no longer exist (after
// grace), retired homes older than retention, and leftover trash. It does
// nothing to live bots' homes and nothing at all unless the caller supplied
// an authoritative live set.
//
// reserve takes the same per-home reservation CreateDevContainer holds; a
// home is removed only under it and only if inUse, asked again under it,
// says no container mounts the home.
func ReconcileOrphanBotHomes(live map[string]bool, inUse func(key string) bool, reserve func(key string) func(), grace, retention time.Duration, dryRun bool, now time.Time) (reaped []string, skipped []GCSkip) {
	base := filepath.Join(workspacesBaseDir, botHomesDirName)
	orgs, err := os.ReadDir(base)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, []GCSkip{{Name: base, Reason: "list error: " + err.Error()}}
	}
	for _, org := range orgs {
		if !org.IsDir() || strings.HasPrefix(org.Name(), ".") {
			continue
		}
		orgBase := filepath.Join(base, org.Name())
		bots, err := os.ReadDir(orgBase)
		if err != nil {
			skipped = append(skipped, GCSkip{Name: orgBase, Reason: "list error: " + err.Error()})
			continue
		}
		liveNames := map[string]bool{}
		keys := map[string]string{}
		for _, bot := range bots {
			if !bot.IsDir() {
				continue
			}
			var owner botHomeOwner
			ok, err := readJSONFile(filepath.Join(orgBase, bot.Name(), botHomeOwnerFile), &owner)
			if err != nil || !ok {
				// Unknown provenance: a creation in progress or a damaged
				// record. Keep it and report it.
				liveNames[bot.Name()] = true
				reason := "no owner record"
				if err != nil {
					reason = "owner record error: " + err.Error()
				}
				skipped = append(skipped, GCSkip{Name: filepath.Join(botHomesDirName, org.Name(), bot.Name()), Reason: reason})
				continue
			}
			key := BotHomeKey(owner.OrganizationID, owner.BotID)
			keys[filepath.Join(orgBase, bot.Name())] = key
			if live[key] || inUse(key) {
				liveNames[bot.Name()] = true
			}
		}
		remove := func(dir string) error {
			key := keys[dir]
			release := reserve(key)
			defer release()
			if inUse(key) {
				return fmt.Errorf("%w: %s", ErrBotHomeInUse, key)
			}
			return os.RemoveAll(dir)
		}
		r, s := reconcileIDDirs(orgBase, botHomesDirName+"-"+org.Name(), "", liveNames, grace, dryRun, remove)
		for _, sk := range s {
			if sk.Reason != "live" {
				skipped = append(skipped, sk)
			}
		}
		reaped = append(reaped, r...)
	}

	retiredBase := filepath.Join(workspacesBaseDir, botHomesRetiredDirName)
	cutoff := now.Add(-retention)
	_ = filepath.WalkDir(retiredBase, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(retiredBase, path)
		if relErr != nil || strings.Count(rel, string(filepath.Separator)) != 2 {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return filepath.SkipDir
		}
		// Rename preserves the directory's mtime, so record when it was
		// retired by the ctime-independent name suffix instead.
		retiredAt, ok := retiredAtFromName(d.Name())
		if !ok {
			retiredAt = info.ModTime()
		}
		if retiredAt.After(cutoff) {
			return filepath.SkipDir
		}
		if dryRun {
			reaped = append(reaped, path)
		} else if err := os.RemoveAll(path); err != nil {
			skipped = append(skipped, GCSkip{Name: path, Reason: "error: " + err.Error()})
		} else {
			reaped = append(reaped, path)
		}
		return filepath.SkipDir
	})

	trashBase := filepath.Join(workspacesBaseDir, botHomesTrashDirName)
	if entries, err := os.ReadDir(trashBase); err == nil {
		for _, e := range entries {
			path := filepath.Join(trashBase, e.Name())
			if dryRun {
				reaped = append(reaped, path)
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				skipped = append(skipped, GCSkip{Name: path, Reason: "error: " + err.Error()})
				continue
			}
			reaped = append(reaped, path)
		}
	}
	return reaped, skipped
}

func retiredAtFromName(name string) (time.Time, bool) {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return time.Time{}, false
	}
	var nanos int64
	if _, err := fmt.Sscanf(name[i+1:], "%d", &nanos); err != nil || nanos <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}

// reserveBotHome blocks until no other create, destroy or reap holds the
// home, then holds it until the returned release is called.
func (dm *DevContainerManager) reserveBotHome(key string) func() {
	dm.botHomeLocksMu.Lock()
	if dm.botHomeLocks == nil {
		dm.botHomeLocks = make(map[string]*sync.Mutex)
	}
	mu, ok := dm.botHomeLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		dm.botHomeLocks[key] = mu
	}
	dm.botHomeLocksMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// botHomeInUseChecker reports whether a running container mounts a bot home.
// When Docker cannot be asked, every home counts as in use.
func (dm *DevContainerManager) botHomeInUseChecker(ctx context.Context) func(string) bool {
	return func(key string) bool {
		dockerClient, err := dm.getDockerClient("")
		if err != nil {
			return true
		}
		defer dockerClient.Close()
		sessions, err := runningBotHomeSessions(ctx, dockerClient, key)
		return err != nil || len(sessions) > 0
	}
}
