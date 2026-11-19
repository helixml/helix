package hydra

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testOrg     = "org_a"
	testBot     = "b-birding-promoter"
	testSession = "ses_old"
	testGen     = "bh_gen1"
)

func useTempWorkspaces(t *testing.T) string {
	t.Helper()
	old := workspacesBaseDir
	workspacesBaseDir = t.TempDir()
	t.Cleanup(func() { workspacesBaseDir = old })
	return workspacesBaseDir
}

func spec(expect BotHomeExpectation) *BotHomeSpec {
	return &BotHomeSpec{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Expect: expect}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// populateLegacy creates a session workspace holding canaries, the default
// profile and a custom profile.
func populateLegacy(t *testing.T, sessionID string) string {
	t.Helper()
	legacy := legacySessionWorkspace(sessionID)
	writeFile(t, filepath.Join(legacy, "canary.txt"), "home-canary-1", 0o600)
	writeFile(t, filepath.Join(legacy, "scripts", "post.sh"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(legacy, ".chrome-state", "Local State"), `{"profile":{}}`, 0o600)
	writeFile(t, filepath.Join(legacy, ".chrome-state", "Default", "Preferences"), `{"browser":{}}`, 0o600)
	writeFile(t, filepath.Join(legacy, "chrome-profile", "Local State"), `{"profile":{}}`, 0o600)
	writeFile(t, filepath.Join(legacy, "chrome-profile", "Default", "Preferences"), `{"x":1}`, 0o600)
	writeFile(t, filepath.Join(legacy, "chrome-profile", "Default", "Cookies"), "SECRET-COOKIE-BYTES", 0o600)
	writeFile(t, filepath.Join(legacy, "chrome-profile", "profile-canary.txt"), "profile-canary-1", 0o600)
	require.NoError(t, os.Chmod(filepath.Join(legacy, "chrome-profile"), 0o700))
	return legacy
}

func TestPrepareBotHome_CreatesWhenNothingExists(t *testing.T) {
	useTempWorkspaces(t)
	res, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.NoError(t, err)
	require.Equal(t, BotHomeOutcomeCreated, res.Outcome)
	home, _ := BotHomeDir(testOrg, testBot)
	require.Equal(t, home, res.Path)
	require.DirExists(t, home)
	var owner botHomeOwner
	ok, err := readJSONFile(filepath.Join(filepath.Dir(home), botHomeOwnerFile), &owner)
	require.True(t, ok)
	require.NoError(t, err)
	require.Equal(t, testGen, owner.Generation)
	require.Equal(t, testBot, owner.BotID)

	// Same generation again is the existing home, not a new one.
	writeFile(t, filepath.Join(home, "canary.txt"), "after-create", 0o600)
	res, err = prepareBotHome(spec(BotHomeExpectExisting), "ses_new")
	require.NoError(t, err)
	require.Equal(t, BotHomeOutcomeExisting, res.Outcome)
	require.Equal(t, "after-create", readFile(t, filepath.Join(home, "canary.txt")))
}

func TestPrepareBotHome_MigratesLegacyWorkspacePreservingData(t *testing.T) {
	useTempWorkspaces(t)
	legacy := populateLegacy(t, testSession)

	res, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.NoError(t, err)
	require.Equal(t, BotHomeOutcomeMigrated, res.Outcome)
	require.Equal(t, testSession, res.MigratedFromSession)
	require.NoDirExists(t, legacy)

	home := res.Path
	require.Equal(t, "home-canary-1", readFile(t, filepath.Join(home, "canary.txt")))
	require.Equal(t, "profile-canary-1", readFile(t, filepath.Join(home, "chrome-profile", "profile-canary.txt")))
	info, err := os.Stat(filepath.Join(home, "chrome-profile"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(home, "scripts", "post.sh"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	require.NoFileExists(t, filepath.Join(filepath.Dir(home), botHomeMigrationFile))

	// Inventory sees both profiles and never carries cookie contents.
	require.Len(t, res.Inventory.ChromeProfiles, 2)
	for _, p := range res.Inventory.ChromeProfiles {
		require.True(t, p.LocalStateValid, p.Path)
		require.True(t, p.DefaultPrefsValid, p.Path)
	}
	encoded, err := json.Marshal(res)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "SECRET-COOKIE-BYTES")
	require.NotContains(t, string(encoded), "canary")

	// The next session of the same bot sees the same data.
	res, err = prepareBotHome(spec(BotHomeExpectExisting), "ses_replacement")
	require.NoError(t, err)
	require.Equal(t, BotHomeOutcomeExisting, res.Outcome)
	require.Equal(t, "home-canary-1", readFile(t, filepath.Join(res.Path, "canary.txt")))
}

func TestPrepareBotHome_RefusesToSeedMissingEstablishedHome(t *testing.T) {
	base := useTempWorkspaces(t)
	_, err := prepareBotHome(spec(BotHomeExpectExisting), testSession)
	require.ErrorContains(t, err, "refusing to seed a fresh home")
	home, _ := BotHomeDir(testOrg, testBot)
	require.NoDirExists(t, home)

	// Recorded home whose data dir vanished.
	_, err = prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.NoError(t, err)
	require.NoError(t, os.Rename(home, filepath.Join(base, "moved-away")))
	_, err = prepareBotHome(spec(BotHomeExpectExisting), testSession)
	require.ErrorContains(t, err, "data directory is missing")
	require.NoDirExists(t, home)
}

func TestPrepareBotHome_RefusesGenerationMismatch(t *testing.T) {
	useTempWorkspaces(t)
	_, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.NoError(t, err)
	other := spec(BotHomeExpectExisting)
	other.Generation = "bh_gen2"
	_, err = prepareBotHome(other, testSession)
	require.ErrorContains(t, err, "has generation bh_gen1")
	other.Expect = BotHomeExpectNew
	_, err = prepareBotHome(other, testSession)
	require.ErrorContains(t, err, "has generation bh_gen1")
}

func TestPrepareBotHome_RefusesWhenBothSidesPopulated(t *testing.T) {
	useTempWorkspaces(t)
	_, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.NoError(t, err)
	home, _ := BotHomeDir(testOrg, testBot)
	writeFile(t, filepath.Join(home, "canary.txt"), "new-side", 0o600)
	legacy := populateLegacy(t, testSession)

	_, err = prepareBotHome(spec(BotHomeExpectExisting), testSession)
	require.ErrorContains(t, err, "refusing to choose or overwrite either")
	require.Equal(t, "new-side", readFile(t, filepath.Join(home, "canary.txt")))
	require.Equal(t, "home-canary-1", readFile(t, filepath.Join(legacy, "canary.txt")))
}

func TestPrepareBotHome_RefusesHomeWithoutOwnerRecord(t *testing.T) {
	useTempWorkspaces(t)
	home, _ := BotHomeDir(testOrg, testBot)
	writeFile(t, filepath.Join(home, "unknown.txt"), "x", 0o600)
	_, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.ErrorContains(t, err, "without an owner record")
	require.Equal(t, "x", readFile(t, filepath.Join(home, "unknown.txt")))
}

func TestPrepareBotHome_RefusesAnotherTenantsOwnerRecord(t *testing.T) {
	useTempWorkspaces(t)
	_, err := prepareBotHome(&BotHomeSpec{OrganizationID: "org_b", BotID: testBot, Generation: testGen, Expect: BotHomeExpectNew}, testSession)
	require.NoError(t, err)
	otherRoot, _ := BotHomeRoot("org_b", testBot)
	myRoot, _ := BotHomeRoot(testOrg, testBot)
	require.NotEqual(t, otherRoot, myRoot)
	// A copied owner record does not let tenant A claim it.
	require.NoError(t, os.MkdirAll(myRoot, 0o755))
	data, err := os.ReadFile(filepath.Join(otherRoot, botHomeOwnerFile))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(myRoot, botHomeOwnerFile), data, 0o600))
	_, err = prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.ErrorContains(t, err, "belongs to org_b/")
}

func TestBotHomePaths_RejectTraversalAndKeepIDsDistinct(t *testing.T) {
	useTempWorkspaces(t)
	for _, bad := range []string{"", "../x", "a/b", `a\b`, "a\x00b"} {
		_, err := BotHomeRoot(testOrg, bad)
		require.Error(t, err, bad)
		_, err = BotHomeRoot(bad, testBot)
		require.Error(t, err, bad)
	}
	seen := map[string]string{}
	for _, id := range []string{"bot", "Bot", "..bot", ".bot", "bot with space", "~626f74", "bot.", "ünï"} {
		root, err := BotHomeRoot(testOrg, id)
		require.NoError(t, err, id)
		require.Equal(t, filepath.Join(workspacesBaseDir, botHomesDirName, testOrg), filepath.Dir(root), id)
		prev, dup := seen[root]
		require.False(t, dup, "%q and %q share %s", id, prev, root)
		seen[root] = id
	}
	_, err := prepareBotHome(&BotHomeSpec{OrganizationID: testOrg, BotID: testBot, Generation: "../g", Expect: BotHomeExpectNew}, testSession)
	require.Error(t, err)
	_, err = prepareBotHome(spec(BotHomeExpectNew), "ses_../../etc")
	require.Error(t, err)
}

func TestPrepareBotHome_ResumesInterruptedMigration(t *testing.T) {
	t.Run("journal written, rename not done", func(t *testing.T) {
		useTempWorkspaces(t)
		legacy := populateLegacy(t, testSession)
		root, _ := BotHomeRoot(testOrg, testBot)
		require.NoError(t, os.MkdirAll(root, 0o755))
		require.NoError(t, writeJSONFileAtomic(filepath.Join(root, botHomeMigrationFile), botHomeMigration{
			Generation: testGen, SessionID: testSession, From: legacy, To: filepath.Join(root, botHomeDataDir),
		}))
		res, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
		require.NoError(t, err)
		require.Equal(t, BotHomeOutcomeMigrated, res.Outcome)
		require.Equal(t, "home-canary-1", readFile(t, filepath.Join(res.Path, "canary.txt")))
		require.NoDirExists(t, legacy)
	})
	t.Run("renamed, owner record not written", func(t *testing.T) {
		useTempWorkspaces(t)
		legacy := populateLegacy(t, testSession)
		root, _ := BotHomeRoot(testOrg, testBot)
		require.NoError(t, os.MkdirAll(root, 0o755))
		home := filepath.Join(root, botHomeDataDir)
		require.NoError(t, writeJSONFileAtomic(filepath.Join(root, botHomeMigrationFile), botHomeMigration{
			Generation: testGen, SessionID: testSession, From: legacy, To: home,
		}))
		require.NoError(t, os.Rename(legacy, home))
		// The API had not recorded the home as established yet.
		res, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
		require.NoError(t, err)
		require.Equal(t, BotHomeOutcomeMigrated, res.Outcome)
		require.Equal(t, "home-canary-1", readFile(t, filepath.Join(home, "canary.txt")))
		require.FileExists(t, filepath.Join(root, botHomeOwnerFile))
		require.NoFileExists(t, filepath.Join(root, botHomeMigrationFile))
	})
	t.Run("both sides populated", func(t *testing.T) {
		useTempWorkspaces(t)
		legacy := populateLegacy(t, testSession)
		root, _ := BotHomeRoot(testOrg, testBot)
		home := filepath.Join(root, botHomeDataDir)
		writeFile(t, filepath.Join(home, "canary.txt"), "other", 0o600)
		require.NoError(t, writeJSONFileAtomic(filepath.Join(root, botHomeMigrationFile), botHomeMigration{
			Generation: testGen, SessionID: testSession, From: legacy, To: home,
		}))
		_, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
		require.ErrorContains(t, err, "refusing to overwrite either")
		require.Equal(t, "other", readFile(t, filepath.Join(home, "canary.txt")))
		require.Equal(t, "home-canary-1", readFile(t, filepath.Join(legacy, "canary.txt")))
	})
	t.Run("journal for another generation", func(t *testing.T) {
		useTempWorkspaces(t)
		legacy := populateLegacy(t, testSession)
		root, _ := BotHomeRoot(testOrg, testBot)
		require.NoError(t, os.MkdirAll(root, 0o755))
		require.NoError(t, writeJSONFileAtomic(filepath.Join(root, botHomeMigrationFile), botHomeMigration{
			Generation: "bh_other", SessionID: testSession, From: legacy, To: filepath.Join(root, botHomeDataDir),
		}))
		_, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
		require.ErrorContains(t, err, "does not match this start")
		require.Equal(t, "home-canary-1", readFile(t, filepath.Join(legacy, "canary.txt")))
	})
}

func TestDestroyBotHome_RetireOnlyTouchesItsOwnScope(t *testing.T) {
	base := useTempWorkspaces(t)
	now := time.Unix(1_800_000_000, 0)
	mk := func(org, bot, gen, canary string) string {
		res, err := prepareBotHome(&BotHomeSpec{OrganizationID: org, BotID: bot, Generation: gen, Expect: BotHomeExpectNew}, "ses_"+bot)
		require.NoError(t, err)
		writeFile(t, filepath.Join(res.Path, "canary.txt"), canary, 0o600)
		return res.Path
	}
	mine := mk(testOrg, testBot, testGen, "mine-1")
	sibling := mk(testOrg, "b-sibling", "bh_s", "sibling")
	otherTenant := mk("org_b", testBot, "bh_t", "other-tenant")

	// Wrong generation is refused and nothing moves.
	_, err := destroyBotHome(&DestroyBotHomeRequest{OrganizationID: testOrg, BotID: testBot, Generation: "bh_wrong", Mode: BotHomeRetire}, now)
	require.ErrorContains(t, err, "refusing to erase")
	require.Equal(t, "mine-1", readFile(t, filepath.Join(mine, "canary.txt")))

	resp, err := destroyBotHome(&DestroyBotHomeRequest{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Mode: BotHomeRetire}, now)
	require.NoError(t, err)
	require.True(t, resp.Existed)
	require.False(t, resp.Purged)
	require.NoDirExists(t, mine)
	require.Equal(t, "mine-1", readFile(t, filepath.Join(resp.RetiredPath, botHomeDataDir, "canary.txt")))
	require.Equal(t, "sibling", readFile(t, filepath.Join(sibling, "canary.txt")))
	require.Equal(t, "other-tenant", readFile(t, filepath.Join(otherTenant, "canary.txt")))

	// A new generation and a second reset leave the first rollback copy alone.
	mk(testOrg, testBot, "bh_gen2", "mine-2")
	resp2, err := destroyBotHome(&DestroyBotHomeRequest{OrganizationID: testOrg, BotID: testBot, Generation: "bh_gen2", Mode: BotHomeRetire}, now.Add(time.Second))
	require.NoError(t, err)
	require.NotEqual(t, resp.RetiredPath, resp2.RetiredPath)
	require.Equal(t, "mine-1", readFile(t, filepath.Join(resp.RetiredPath, botHomeDataDir, "canary.txt")))
	require.Equal(t, "mine-2", readFile(t, filepath.Join(resp2.RetiredPath, botHomeDataDir, "canary.txt")))

	// Purge of an absent home is a no-op that says so.
	resp3, err := destroyBotHome(&DestroyBotHomeRequest{OrganizationID: testOrg, BotID: testBot, Mode: BotHomePurge}, now)
	require.NoError(t, err)
	require.False(t, resp3.Existed)

	// Purge deletes now and leaves no trash.
	resp4, err := destroyBotHome(&DestroyBotHomeRequest{OrganizationID: testOrg, BotID: "b-sibling", Mode: BotHomePurge}, now)
	require.NoError(t, err)
	require.True(t, resp4.Purged)
	require.NoDirExists(t, sibling)
	entries, _ := os.ReadDir(filepath.Join(base, botHomesTrashDirName))
	require.Empty(t, entries)
}

func TestDestroyBotHome_FailedPurgeReportsWhereDataRemains(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	base := useTempWorkspaces(t)
	res, err := prepareBotHome(spec(BotHomeExpectNew), testSession)
	require.NoError(t, err)
	locked := filepath.Join(res.Path, "locked")
	writeFile(t, filepath.Join(locked, "f"), "x", 0o600)
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = filepath.Walk(base, func(p string, _ os.FileInfo, _ error) error { return os.Chmod(p, 0o755) }) })

	_, err = destroyBotHome(&DestroyBotHomeRequest{OrganizationID: testOrg, BotID: testBot, Mode: BotHomePurge}, time.Now())
	require.ErrorContains(t, err, "data may still exist at "+filepath.Join(base, botHomesTrashDirName))
	require.NoDirExists(t, res.Path)

	// The reaper retries the trash.
	_ = filepath.Walk(filepath.Join(base, botHomesTrashDirName), func(p string, _ os.FileInfo, _ error) error { return os.Chmod(p, 0o755) })
	reaped, _ := ReconcileOrphanBotHomes(map[string]bool{}, func(string) bool { return false }, time.Hour, time.Hour, false, time.Now())
	require.Len(t, reaped, 1)
	entries, _ := os.ReadDir(filepath.Join(base, botHomesTrashDirName))
	require.Empty(t, entries)
}

func TestReconcileOrphanBotHomes(t *testing.T) {
	base := useTempWorkspaces(t)
	now := time.Now()
	mk := func(org, bot string) string {
		res, err := prepareBotHome(&BotHomeSpec{OrganizationID: org, BotID: bot, Generation: "bh_" + bot, Expect: BotHomeExpectNew}, "ses_"+bot)
		require.NoError(t, err)
		return filepath.Dir(res.Path)
	}
	live := mk(testOrg, "live")
	dead := mk(testOrg, "dead")
	inUse := mk("org_b", "inuse")
	noOwner := filepath.Join(base, botHomesDirName, testOrg, "partial")
	require.NoError(t, os.MkdirAll(noOwner, 0o755))

	oldRetired := filepath.Join(base, botHomesRetiredDirName, testOrg, "x", "bh_x-"+itoa(now.Add(-48*time.Hour).UnixNano()))
	newRetired := filepath.Join(base, botHomesRetiredDirName, testOrg, "x", "bh_y-"+itoa(now.Add(-time.Hour).UnixNano()))
	require.NoError(t, os.MkdirAll(oldRetired, 0o755))
	require.NoError(t, os.MkdirAll(newRetired, 0o755))

	liveSet := map[string]bool{BotHomeKey(testOrg, "live"): true}
	isInUse := func(key string) bool { return key == BotHomeKey("org_b", "inuse") }

	// Dry run removes nothing.
	reaped, _ := ReconcileOrphanBotHomes(liveSet, isInUse, 0, 24*time.Hour, true, now)
	require.ElementsMatch(t, []string{dead, oldRetired}, reaped)
	require.DirExists(t, dead)

	reaped, skipped := ReconcileOrphanBotHomes(liveSet, isInUse, 0, 24*time.Hour, false, now)
	require.ElementsMatch(t, []string{dead, oldRetired}, reaped)
	require.DirExists(t, live)
	require.DirExists(t, inUse)
	require.DirExists(t, noOwner)
	require.DirExists(t, newRetired)
	require.NoDirExists(t, dead)
	require.NoDirExists(t, oldRetired)
	var reasons []string
	for _, s := range skipped {
		reasons = append(reasons, s.Reason)
	}
	require.Contains(t, reasons, "no owner record")

	// Within grace a dead bot's home stays.
	dead2 := mk(testOrg, "dead2")
	reaped, _ = ReconcileOrphanBotHomes(liveSet, isInUse, time.Hour, 24*time.Hour, false, now)
	require.Empty(t, reaped)
	require.DirExists(t, dead2)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
