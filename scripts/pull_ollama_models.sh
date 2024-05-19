#!/usr/bin/env bash

# Set the OLLAMA_MODELS env variable to ensure
# we can access the models later to /workspace/ollama

export OLLAMA_MODELS="/workspace/ollama"

ollama serve &
serve_pid=$!

ollama list

echo "Ollama models directory $OLLAMA_MODELS"
echo "Pull ollama models: $PULL_OLLAMA_MODELS"

# Check if PULL_OLLAMA_MODELS is set and not empty
if [ -n "$PULL_OLLAMA_MODELS" ]; then
    # Split the comma-separated list into an array
    IFS=',' read -r -a models <<< "$PULL_OLLAMA_MODELS"

    # Iterate over each model and pull it
    for model in "${models[@]}"; do
        # drone requires variable to have double quotes to avoid splitting on
        # commas, but, we end up with the double quotes here. Remove them
        model=$(echo "$model" | tr -d '"')
        echo "Pulling $model"
        ollama pull "$model"
    done
fi
