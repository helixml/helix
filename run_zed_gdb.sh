#!/bin/bash

# Script to run Zed under gdb to catch crash
export RUST_BACKTRACE=full
export ZED_EXTERNAL_SYNC_ENABLED=true
export ZED_WEBSOCKET_SYNC_ENABLED=true
export ZED_HELIX_URL=localhost:8080
export ZED_HELIX_TOKEN=oh-hallo-insecure-token
export ZED_HELIX_TLS=false
export ZED_AUTO_OPEN_AI_PANEL=true
export ZED_SHOW_AI_ASSISTANT=true
export ZED_CONFIG_DIR=/home/luke/pm/helix/test-zed-config/config
export ZED_DATA_DIR=/home/luke/pm/helix/test-zed-config/data
export ANTHROPIC_API_KEY=sk-ant-api03-PxEAAMM_nLJ8bgNfzCXUOKPEMTKzjdXrJCEKTJW-9zQUGLZrFMOoNLkSZnBn0gHm3vE1M7FBHHMnBZ9sGXvFJRTM7VQ

echo "🔍 Starting Zed under gdb to catch crash..."
echo "Click the agent panel when it appears to trigger the crash"

gdb --batch \
    --ex "handle SIGSEGV stop print" \
    --ex "handle SIGABRT stop print" \
    --ex "run" \
    --ex "thread apply all bt" \
    --ex "quit" \
    ./zed-build/zed