#!/usr/bin/env bash

# Set the OLLAMA_MODELS env variable to ensure
# we can access the models later to /workspace/ollama



export OLLAMA_MODELS="/workspace/ollama"

ollama serve &
serve_pid=$!

ollama list

echo "Ollama models directory $OLLAMA_MODELS"

# If PULL_LLAMA3_8B is set to true, pull it
if [ -n "${PULL_LLAMA3_8B:-}" ]; then
    echo "Pulling llama3:8b"
    ollama pull llama3:instruct
fi

# If PULL_LLAMA3_70B is set to true, pull it
if [ -n "${PULL_LLAMA3_70B:-}" ]; then
    echo "Pulling llama3:70b"
    ollama pull llama3:70b
fi