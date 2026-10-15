#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 0 ]]; then
    echo 'Usage: bash pull.sh MODEL_ID [MODEL_ID ...]' >&2
    echo 'Use the model IDs in config.json, or pull.ps1 for an interactive picker.' >&2
    exit 1
fi

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
for model in "$@"; do
    docker compose exec -T ollama ollama pull "$model"
done
