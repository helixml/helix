#!/usr/bin/env bash

# Set the OLLAMA_MODELS env variable to ensure
# we can access the models later to /workspace/ollama



export OLLAMA_MODELS="/workspace/ollama"

ollama serve &
serve_pid=$!

ollama list

echo "Ollama models directory $OLLAMA_MODELS"
echo "Pull llama3:8b: $PULL_LLAMA3_8B"
echo "Pull llama3:70b: $PULL_LLAMA3_70B"

# If PULL_LLAMA3_8B is set to true, pull it
if [ "$PULL_LLAMA3_8B" = "true" ]; then
    echo "Pulling llama3:8b"
    ollama pull llama3:8b
fi

# If PULL_LLAMA3_70B is set to true, pull it
if [ "$PULL_LLAMA3_70B" = "true" ]; then
    echo "Pulling llama3:70b"
    ollama pull llama3:70b
fi