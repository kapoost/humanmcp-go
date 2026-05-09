#!/usr/bin/env bash
# setup.sh — 1-click humanMCP deploy to Fly.io
# Usage: curl -sL https://raw.githubusercontent.com/kapoost/humanmcp-go/main/setup.sh | bash
#    or: git clone ... && cd humanmcp-go && bash setup.sh

set -e

echo ""
echo "  ╔══════════════════════════════════════╗"
echo "  ║       humanMCP — setup               ║"
echo "  ║  One human. One server. Your rules.  ║"
echo "  ╚══════════════════════════════════════╝"
echo ""

# ── Check flyctl ──────────────────────────────────────────────────────────────
if ! command -v fly &>/dev/null; then
  echo "flyctl not found. Installing..."
  curl -L https://fly.io/install.sh | sh
  export PATH="$HOME/.fly/bin:$PATH"
fi

if ! fly auth whoami &>/dev/null; then
  echo ""
  echo "You need a Fly.io account (free tier is fine)."
  echo "Opening login..."
  fly auth login
fi

# ── Ask for details ───────────────────────────────────────────────────────────
echo ""
read -p "Your name (e.g. alice): " NAME
NAME=${NAME:-anonymous}
SLUG=$(echo "$NAME" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | tr -cd 'a-z0-9-')
APP="${SLUG}-humanmcp"

read -p "Short bio (1-2 sentences): " BIO
BIO=${BIO:-"A human with something to say."}

read -p "Region (ams/waw/iad/sjc/...): " REGION
REGION=${REGION:-ams}

# Generate a random edit token
TOKEN=$(openssl rand -hex 16 2>/dev/null || head -c 32 /dev/urandom | xxd -p | head -c 32)

echo ""
echo "  App:    $APP"
echo "  Author: $NAME"
echo "  Region: $REGION"
echo ""
read -p "Continue? (y/n) " -n 1 -r
echo ""
[[ $REPLY =~ ^[Yy]$ ]] || { echo "Cancelled."; exit 1; }

# ── Clone repo if not already in it ───────────────────────────────────────────
if [ ! -f "go.mod" ]; then
  echo ""
  echo "Cloning humanmcp-go..."
  git clone https://github.com/kapoost/humanmcp-go.git
  cd humanmcp-go
fi

# ── Create fly.toml ──────────────────────────────────────────────────────────
cat > fly.toml <<EOF
app = "$APP"
primary_region = "$REGION"

[build]

[env]
  PORT        = "8080"
  CONTENT_DIR = "/data/content"
  AUTHOR_NAME = "$NAME"
  AUTHOR_BIO  = "$BIO"

[http_service]
  internal_port        = 8080
  force_https          = true
  auto_stop_machines   = true
  auto_start_machines  = true
  min_machines_running = 0

[[mounts]]
  source      = "humanmcp_data"
  destination = "/data"

[[vm]]
  memory   = "256mb"
  cpu_kind = "shared"
  cpus     = 1
EOF

# ── Launch app ────────────────────────────────────────────────────────────────
echo ""
echo "Creating Fly app: $APP ..."
fly apps create "$APP" --org personal 2>/dev/null || echo "(app may already exist)"

echo "Creating volume..."
fly volumes create humanmcp_data --size 1 --region "$REGION" --app "$APP" -y 2>/dev/null || echo "(volume may already exist)"

# ── Generate signing keys ────────────────────────────────────────────────────
echo "Generating Ed25519 signing keys..."
if command -v go &>/dev/null; then
  KEYS=$(go run ./cmd/keygen/ 2>/dev/null)
  PRIV=$(echo "$KEYS" | grep PRIVATE | cut -d= -f2-)
  PUB=$(echo "$KEYS" | grep PUBLIC | cut -d= -f2-)
else
  PRIV=""
  PUB=""
  echo "(Go not installed locally — skipping key generation. You can add keys later.)"
fi

# ── Set secrets ───────────────────────────────────────────────────────────────
echo "Setting secrets..."
fly secrets set EDIT_TOKEN="$TOKEN" --app "$APP"
fly secrets set DOMAIN="${APP}.fly.dev" --app "$APP"
if [ -n "$PRIV" ]; then
  fly secrets set SIGNING_PRIVATE_KEY="$PRIV" SIGNING_PUBLIC_KEY="$PUB" --app "$APP"
fi

# ── Deploy ────────────────────────────────────────────────────────────────────
echo ""
echo "Deploying..."
fly deploy --app "$APP"

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo "  ╔══════════════════════════════════════════╗"
echo "  ║              READY                       ║"
echo "  ╚══════════════════════════════════════════╝"
echo ""
echo "  Server:  https://${APP}.fly.dev"
echo "  MCP:     https://${APP}.fly.dev/mcp"
echo "  Login:   https://${APP}.fly.dev/login"
echo "  Token:   $TOKEN"
echo ""
echo "  Save your token! You need it to log in."
echo ""
echo "  Next steps:"
echo "  1. Open https://${APP}.fly.dev/login and paste your token"
echo "  2. Create your first piece (poem, essay, note)"
echo "  3. Share your 1-click follow link:"
echo "     https://humanmcp.net/humannetwork.html?add=https://${APP}.fly.dev"
echo ""
echo "  Custom domain? See: fly certs add yourdomain.com --app $APP"
echo ""
