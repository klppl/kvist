#!/bin/bash
# Prepares a fresh Claude Code cloud session: Go modules and the plugin's
# npm packages, so `make check` works right away. Local sessions skip it.
set -euo pipefail
[ "${CLAUDE_CODE_REMOTE:-}" = "true" ] || exit 0
cd "${CLAUDE_PROJECT_DIR:-.}"
go mod download
(cd plugin && npm ci --no-audit --no-fund --silent)
