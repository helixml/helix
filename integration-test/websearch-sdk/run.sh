#!/usr/bin/env bash
# Runs the pinned official Exa SDKs against a Helix instance's Exa-compatible
# API. Needs docker; the API must reach the web (SearXNG, Chrome) and have
# SEARCH_LLM_* and SEARCH_EMBEDDINGS_* configured.
#   HELIX_URL=http://localhost:8080 HELIX_API_KEY=hl-... ./run.sh
set -euo pipefail
: "${HELIX_URL:?}" "${HELIX_API_KEY:?}"
EXA_PY_VERSION=2.28.0
EXA_JS_VERSION=2.28.0
dir="$(cd "$(dirname "$0")" && pwd)"
env=(-e HELIX_URL -e HELIX_API_KEY)

echo "== exa-py ${EXA_PY_VERSION}"
docker run --rm --network host "${env[@]}" -v "$dir:/t:ro" python:3.12-slim \
  sh -c "pip install -q exa-py==${EXA_PY_VERSION} && python /t/exa_py_test.py"

echo "== exa-js ${EXA_JS_VERSION}"
docker run --rm --network host "${env[@]}" -v "$dir:/t:ro" node:22-slim \
  sh -c "cd /tmp && npm i -s exa-js@${EXA_JS_VERSION} >/dev/null && cp /t/exa_js_test.mjs . && node exa_js_test.mjs"
