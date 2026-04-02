#!/usr/bin/env zsh
# deploy.zsh — safe deploy for humanmcp-go
# Usage: zsh deploy.zsh "commit message"
# Repo:  https://github.com/kapoost/humanmcp-go

set -e

MSG=${1:-"chore: update"}
WEB=internal/web

# ── 1. Remove stale handler.go if it crept back ───────────────────────────────
if [[ -f "$WEB/handler.go" ]]; then
  echo "⚠️  Removing stale $WEB/handler.go"
  rm "$WEB/handler.go"
  git rm --cached "$WEB/handler.go" 2>/dev/null || true
fi

# ── 2. Remove .DS_Store files from git tracking ───────────────────────────────
git ls-files --error-unmatch .DS_Store 2>/dev/null && git rm --cached .DS_Store 2>/dev/null || true
git ls-files --error-unmatch internal/.DS_Store 2>/dev/null && git rm --cached internal/.DS_Store 2>/dev/null || true

# ── 3. Ensure .gitignore has .DS_Store ────────────────────────────────────────
if ! grep -q "^\.DS_Store" .gitignore 2>/dev/null; then
  echo ".DS_Store" >> .gitignore
fi

# ── 4. Build ──────────────────────────────────────────────────────────────────
echo "🔨 Building..."
go build ./...
echo "   ✓ build clean"

# ── 5. Test ───────────────────────────────────────────────────────────────────
echo "🧪 Testing..."
go test ./...
echo "   ✓ all tests pass"

# ── 6. Commit & push (skip gracefully if nothing changed) ─────────────────────
echo "📦 Committing..."
git add -A
if git diff --cached --quiet; then
  echo "   (nothing to commit — skipping)"
else
  git commit -m "$MSG"
  git push
fi

# ── 7. Deploy (always runs) ───────────────────────────────────────────────────
echo "🚀 Deploying..."
fly deploy --build-arg CACHEBUST=$(date +%s) --app kapoost-humanmcp

echo "✅ Done"
echo "   App:   https://kapoost-humanmcp.fly.dev"
echo "   Pages: https://kapoost.github.io/humanmcp-go"
