# Design: Remove Broken File Viewer Link from `helix test`

## Overview

Remove the auto-browser-launch and broken URL print statement from the `helix test` command.

## Current Behavior

In `helix/api/cmd/helix/test.go`, the `writeResultsToFile()` function (lines 1428-1439):

```go
fmt.Printf("View results at: %s/files?path=/test-runs/%s\n", helixURL, results.TestID)

// Attempt to open the HTML report in the default browser
if isGraphicalEnvironment() {
    openBrowser(getHelixURL() + "/files?path=/test-runs/" + results.TestID)
}
```

## Change

Remove these lines entirely. The function already prints useful local file paths:
- `Results written to test_results/results_{testID}_{timestamp}.json`
- `HTML report written to test_results/report_{testID}_{timestamp}.html`
- `Latest HTML report written to report_latest.html`
- `Summary written to test_results/summary_{testID}_{timestamp}.md`
- `Latest summary written to summary_latest.md`

## Files Modified

| File | Change |
|------|--------|
| `helix/api/cmd/helix/test.go` | Remove ~6 lines from `writeResultsToFile()` |

## Functions That Become Unused

After removing the `openBrowser()` call:
- `openBrowser()` - can be deleted (only called from this location)
- `isGraphicalEnvironment()` - can be deleted (only called to guard `openBrowser`)

## Testing

1. Run `helix test` with a valid helix.yaml
2. Verify tests complete successfully
3. Verify local file paths are printed
4. Verify no browser window opens
5. Verify no "View results at:" line is printed