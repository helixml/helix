#!/usr/bin/env bash
# update_pr_summary.sh — refresh the CURSOR_SUMMARY block of a PR description.
#
# Codifies Cursor-Bugbot placement semantics (verified against real
# helixml/helix PRs #1477/#1520/#1728): GET body -> re-GET immediately before
# the PATCH (anti-clobber of concurrent human edits) -> replace ONLY the
# content between the <!-- CURSOR_SUMMARY --> / <!-- /CURSOR_SUMMARY -->
# markers, everything outside preserved byte-for-byte; when markers are absent,
# prepend block + blank line at the top -> one PATCH attempt -> GET-verify the
# block carries our sha. The footer sha marks the summary's freshness across
# re-reviews. Exit 0 ONLY after the verify GET passes — never claim an
# unverified summary.
#
# usage: update_pr_summary.sh [summary.json]        # default: ./summary.json
# env:   GH_TOKEN or GITHUB_TOKEN  (get_secret right before running; auth-ops.md §1)
#        GH_API                   (API base, default https://api.github.com; GHE override)
#        HELIX_BOT_ATTRIBUTION    (footer attribution login, default helixml-bot;
#                                  honest attribution — never "Cursor Bugbot")
#
# summary.json shape:
#   { "repo": "owner/repo", "pr": 123, "commit_sha": "<40-char sha reviewed>",
#     "risk": "Low|Medium|High",            # descriptive only, never a verdict gate
#     "risk_file": "<one paragraph: what the PR touches + realistic failure mode>",
#     "overview_file": "<file: 2-4 short factual paragraphs of what the PR DOES — behavior, components, connections; no findings/verdicts>" }
set -euo pipefail

die() { printf 'update_pr_summary: %s\n' "$*" >&2; exit 1; }

SUMMARY_JSON="${1:-summary.json}"
API="${GH_API:-https://api.github.com}"
TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
ATTR="${HELIX_BOT_ATTRIBUTION:-helixml-bot}"

[ -n "$TOKEN" ] || die "no token in GH_TOKEN/GITHUB_TOKEN — get_secret immediately before running (auth-ops.md §1)"
command -v curl >/dev/null || die "curl required"
command -v jq >/dev/null || die "jq required"
command -v python3 >/dev/null || die "python3 required (marker merge)"
[ -f "$SUMMARY_JSON" ] || die "summary manifest not found: $SUMMARY_JSON"

SUM_DIR="$(cd "$(dirname "$SUMMARY_JSON")" && pwd)"
REPO="$(jq -re '.repo' "$SUMMARY_JSON")"             || die "summary.json missing .repo"
PR="$(jq -re '.pr | tostring' "$SUMMARY_JSON")"      || die "summary.json missing .pr"
SHA="$(jq -re '.commit_sha' "$SUMMARY_JSON")"        || die "summary.json missing .commit_sha"
RISK="$(jq -re '.risk' "$SUMMARY_JSON")"             || die "summary.json missing .risk"
RISK_REL="$(jq -re '.risk_file' "$SUMMARY_JSON")"    || die "summary.json missing .risk_file"
OVER_REL="$(jq -re '.overview_file' "$SUMMARY_JSON")" || die "summary.json missing .overview_file"

[[ "$REPO" == */* && -n "${REPO%/*}" && -n "${REPO#*/}" ]] || die "repo must be owner/repo, got '$REPO'"
[[ "$PR" =~ ^[0-9]+$ ]] || die "pr must be a number, got '$PR'"
[[ "$SHA" =~ ^[0-9a-fA-F]{40}$ ]] || die "commit_sha must be a full 40-char sha, got '$SHA'"
case "$RISK" in Low|Medium|High) ;; *) die "risk must be Low|Medium|High — descriptive only, it never changes the verdict policy" ;; esac

resolve() { if [ -f "$1" ]; then printf '%s' "$1"; else printf '%s' "$SUM_DIR/$1"; fi; }
RISK_FILE="$(resolve "$RISK_REL")";  [ -f "$RISK_FILE" ]  || die "risk file not found: $RISK_REL"
OVER_FILE="$(resolve "$OVER_REL")";  [ -f "$OVER_FILE" ]  || die "overview file not found: $OVER_REL"

API_PR="$API/repos/$REPO/pulls/$PR"
BODY="$(mktemp)"; BLOCK="$(mktemp)"; NEWBODY="$(mktemp)"; PAYLOAD="$(mktemp)"; RESP="$(mktemp)"; HEADERS="$(mktemp)"
trap 'rm -f "$BODY" "$BLOCK" "$NEWBODY" "$PAYLOAD" "$RESP" "$HEADERS"' EXIT

# Build the exact block (content lines prefixed "> ", inner blank lines "> ").
python3 - "$RISK_FILE" "$OVER_FILE" "$RISK" "$SHA" "$ATTR" "$BLOCK" <<'PY'
import sys
risk_file, over_file, risk, sha, attr, out = sys.argv[1:7]

def paras(path):
    text = open(path).read().strip()
    assert text, f"{path} is empty"
    return [p.strip() for p in text.split("\n\n") if p.strip()]

def quote(para):
    return [("> " + ln) if ln.strip() else "> " for ln in para.split("\n")]

risk_p = paras(risk_file)[:1]
over_p = paras(over_file)
assert 1 <= len(over_p) <= 4, "overview must be 1-4 paragraphs (brief: 2-4)"

lines = ["---", "> [!NOTE]", f"> **{risk} Risk**"]
lines += quote(risk_p[0])
lines += ["> ", "> **Overview**"]
for i, p in enumerate(over_p):
    lines += quote(p)
    if i < len(over_p) - 1:
        lines.append("> ")
lines += ["> ", f"> <sup>Reviewed by {attr} for commit {sha}.</sup>"]
open(out, "w").write("\n".join(lines))
PY

# Merge into the CURRENT body: replace only between the markers (outside is
# byte-for-byte preserved); prepend with both markers + blank line when absent;
# refuse on ambiguous/half-open markers instead of guessing.
merge_body() { # merge_body <current-body-file> <block-file> <out-file>
  python3 - "$1" "$2" > "$3" <<'PY'
import sys
body = open(sys.argv[1]).read()
block = open(sys.argv[2]).read()
START, END = "<!-- CURSOR_SUMMARY -->", "<!-- /CURSOR_SUMMARY -->"
if START in body or END in body:
    assert START in body and END in body, "half-open CURSOR_SUMMARY markers in body — refusing to merge"
    assert body.count(START) == 1 and body.count(END) == 1, "multiple marker pairs — refusing to guess"
    i, j = body.index(START) + len(START), body.index(END)
    assert i <= j, "marker order inverted — refusing to merge"
    merged = body[:i] + "\n" + block + "\n" + body[j:]
else:
    merged = START + "\n" + block + "\n" + END + "\n\n" + body
sys.stdout.write(merged)
PY
}

GET() {
  # jq -j (not -r): .body must round-trip byte-for-byte — -r's trailing newline would corrupt the tail on re-runs.
  curl -sS -f -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" "$API_PR" \
    | jq -j '.body // ""' > "$1" || die "GET $API_PR failed — cannot build summary against unknown current body"
}

GET "$BODY"
merge_body "$BODY" "$BLOCK" "$NEWBODY"
GET "$BODY"          # re-GET immediately before the PATCH; rebuild against the freshest body
merge_body "$BODY" "$BLOCK" "$NEWBODY"

jq -Rs '{body:.}' "$NEWBODY" > "$PAYLOAD"
STATUS="$(curl -sS -o "$RESP" -D "$HEADERS" -w '%{http_code}' \
  -X PATCH "$API_PR" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  --data @"$PAYLOAD")" || die "PATCH transport failure (no retry — re-mint once per auth-ops.md §2, else stop)"

if [ "${STATUS:0:1}" != "2" ]; then
  { printf '== verbatim GitHub response ==\nstatus: %s\n' "$STATUS"
    printf 'x-github-request-id: %s\n' "$(grep -im1 '^x-github-request-id:' "$HEADERS" | tr -d '\r' | cut -d' ' -f2-)"
    printf 'x-accepted-github-permissions: %s\n' "$(grep -im1 '^x-accepted-github-permissions:' "$HEADERS" | tr -d '\r' | cut -d' ' -f2-)"
    printf 'message: %s\n' "$(jq -r '.message // "?"' "$RESP" 2>/dev/null || true)"
    printf -- '-- body --\n'; cat "$RESP"; printf '\n'; } >&2
  die "PATCH refused with $STATUS — verbatim response dumped (auth-ops.md §3). Do NOT claim a summary was written."
fi

REVIEW_URL="$(jq -r '.html_url // "?"' "$RESP")"

VERIFY="$(curl -sS -f -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.github+json" "$API_PR" | jq -r '.body // ""')" \
  || die "verify GET failed — summary may have written but is UNVERIFIED; do not claim success"
{ printf '%s' "$VERIFY" | grep -qF '<!-- CURSOR_SUMMARY -->' && printf '%s' "$VERIFY" | grep -qF "for commit $SHA.</sup>"; } \
  || die "UNVERIFIED: after PATCH $STATUS the fetched body has no CURSOR_SUMMARY block stamped for $SHA — do not claim success"

printf 'VERIFIED: CURSOR_SUMMARY block present, stamped for commit %s — %s\n' "$SHA" "$REVIEW_URL"
