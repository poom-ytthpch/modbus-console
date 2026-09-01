#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE="$ROOT/.agent-state"
mkdir -p "$STATE"
SKILLS=()
while IFS= read -r skill; do SKILLS+=("$skill"); done < <(find "$ROOT/.agents/skills" -name SKILL.md -type f -print 2>/dev/null | sort)
PARENT="$(cd "$ROOT/.." && pwd)"
while IFS= read -r skill; do SKILLS+=("$skill"); done < <(find "$PARENT/.agents/skills" -name SKILL.md -type f -print 2>/dev/null | sort)
if [[ ${#SKILLS[@]} -eq 0 ]]; then echo "No skills found" >&2; exit 2; fi
: > "$STATE/skills.concat"
for skill in "${SKILLS[@]}"; do
  echo "===== SKILL: ${skill#$PARENT/} ====="
  cat "$skill"
  cat "$skill" >> "$STATE/skills.concat"
  printf '\n' >> "$STATE/skills.concat"
done
shasum -a 256 "$STATE/skills.concat" | awk '{print $1}' > "$STATE/skills.sha256"
rm "$STATE/skills.concat"
echo "SKILLS_READ=${#SKILLS[@]} fingerprint=$(cat "$STATE/skills.sha256")"
