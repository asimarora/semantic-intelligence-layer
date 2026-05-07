#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

COMPOSE_FILE="${1:-compose.yaml}"
shift $(( $# > 0 ? 1 : 0 ))

cd "${REPO_ROOT}"
exec docker compose -f "${COMPOSE_FILE}" up -d "$@"
