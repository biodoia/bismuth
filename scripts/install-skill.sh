#!/usr/bin/env bash
# scripts/install-skill.sh — install bismuth-control skill into
# ~/.claude/skills/ so Hermes (Claude Code) can orchestrate the team.

set -euo pipefail

SKILL_DIR="${HOME}/.claude/skills/bismuth-control"
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$SKILL_DIR"

cd "$SCRIPT_DIR"
GOTMPDIR=/home/lisergico25/.tmp go run ./cmd/bismuth cli skill-install

echo "Installed bismuth-control skill at $SKILL_DIR"
ls -la "$SKILL_DIR"
