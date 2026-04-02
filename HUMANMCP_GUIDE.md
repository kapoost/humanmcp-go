# humanMCP — Project Guide
## Version 0.2

**Live:** https://kapoost-humanmcp.fly.dev
**Repo:** https://github.com/kapoost/humanmcp-go
**MCP registry:** `io.github.kapoost/humanmcp`

---

## What it is

A personal Go server publishing poems, images, and data with:
- Ed25519 cryptographic signatures (proof of authorship)
- 12 MCP tools so AI agents can read, verify, and license your work
- Web UI for humans at `kapoost-humanmcp.fly.dev`
- Owner-only editor at `/new` and `/edit/:slug`

---

## Project structure

```
humanmcp/
├── cmd/
│   ├── server/main.go          — entry point, wires MCP + web + middleware
│   └── keygen/main.go          — generate Ed25519 signing keys (run once)
├── internal/
│   ├── auth/auth.go
│   ├── config/config.go
│   ├── content/
│   │   ├── content.go          — piece store (markdown files)
│   │   ├── blob.go             — blob store (images, vectors, datasets)
│   │   ├── messages.go
│   │   ├── stats.go
│   │   ├── signing.go          — Ed25519 sign/verify
│   │   ├── copyright.go
│   │   ├── frontmatter.go
│   │   └── cache.go
│   ├── mcp/handler.go          — MCP/JSON-RPC 2.0 endpoint (dynamic tool count)
│   └── web/
│       ├── handler_web.go      ← THE REAL FILE (not handler.go)
│       └── templates.go        — all HTML templates (one Go string constant)
├── deploy.zsh                  — safe deploy script (always use this)
├── Dockerfile
├── fly.toml
└── go.mod
```

**Critical:** handler is `handler_web.go` not `handler.go`. `deploy.zsh` auto-deletes stale `handler.go` if it reappears.

---

## How to deploy

```zsh
# 1. Copy downloaded files into repo
cp ~/Downloads/handler_web.go ~/humanmcp/humanmcp/internal/web/handler_web.go
cp ~/Downloads/templates.go   ~/humanmcp/humanmcp/internal/web/templates.go

# 2. Deploy
cd ~/humanmcp/humanmcp
zsh deploy.zsh "your commit message"
```

deploy.zsh: removes stale handler.go → build → test → commit → push → fly deploy with CACHEBUST.

**Never:** `go build ./... # comment` — Go treats comment as package args.

```zsh
fly logs --app kapoost-humanmcp   # check logs
```

---

## Fly.io

**App:** `kapoost-humanmcp` | **Region:** `ams` | **Volume:** `humanmcp_data` at `/data`

### Secrets (set once)

| Secret | Purpose |
|---|---|
| EDIT_TOKEN | Owner login password |
| SIGNING_PRIVATE_KEY | Ed25519 private key (base64) |
| SIGNING_PUBLIC_KEY | Ed25519 public key (hex) |
| SECRET_KEY | Server secret |
| AUTHOR_BIO | Bio (update: `fly secrets set AUTHOR_BIO="..."`) |
| AUTHOR_NAME | kapoost |
| DOMAIN | kapoost-humanmcp.fly.dev |

### fly.toml env

PORT=8080, CONTENT_DIR=/data/content, AUTHOR_NAME=kapoost, DOMAIN=kapoost-humanmcp.fly.dev, AI_METADATA=true

---

## AI-assisted development workflow

1. Zip and upload to Claude: `cd ~/humanmcp && zip -r humanmcp.zip humanmcp/ --exclude "*.git*"`
2. Describe what needs changing
3. Claude generates files, verified with `go test ./...` against your zip
4. Download → copy → `zsh deploy.zsh "message"`

**Why zip:** GitHub repo is often out of sync with local. Claude needs real files.

**Live data patch** (browser console, logged in):
```javascript
fetch('/api/blobs/OLD_SLUG', {
  method: 'PUT',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({ Slug: 'NEW_SLUG', BlobType: 'image', Access: 'public',
    MimeType: 'image/jpeg', FileRef: 'files/file.jpg', Title: 'Title' })
}).then(r=>r.json()).then(d=>{window._r=d})
```
Keys must be **PascalCase** (no JSON struct tags in Go).

---

## Option D — unified slugs for images

When you post type=`image` with a file at `/new`:
- One timestamp slug generated: `fmt.Sprintf("%d", time.Now().Unix())`
- Piece saved with that slug
- Blob created with the **same slug**, title, access, tags, signed
- Thumbnail appears on index immediately — no manual patching needed

Text pieces (poem, essay, note): human-readable slug from title, unchanged.

---

## Signing

Auto-signed on every save. Payload: `sha256(slug | title | body)`.

**✓ SIGNED badge** = signature exists. **Real verification** via MCP:
- `verify_content {slug}` → verified or tamper detected
- `get_certificate {slug}` → full IP cert: hash, sig, originality, license

Public key in `/.well-known/mcp-server.json`. Private key only in Fly secret.

---

## Web routes

| Route | Access | Description |
|---|---|---|
| `/` | public | Post list — type badges, thumbnails, excerpts |
| `/p/:slug` | public | Read piece — signing block, license, leave a message |
| `/images` | public | Image gallery |
| `/files/:filename` | public | Raw file serving |
| `/connect` | public | MCP connection guide |
| `/contact` | public | Comment/contact — "re: Title" badge, no dropdown |
| `/new` | owner | Create post or upload image (Option D) |
| `/edit/:slug` | owner | Edit existing post |
| `/delete/:slug` | owner | Delete (POST only) |
| `/dashboard` | owner | Stats + messages |
| `/messages` | owner | Messages list |
| `/login` `/logout` | public | Auth |
| `/.well-known/mcp-server.json` | public | MCP agent discovery |

**Removed:** `/upload` (broken, covered by `/new`)

---

## MCP tools (12)

get_author_profile, list_content, read_content, request_access, submit_answer,
list_blobs, read_blob, verify_content, get_certificate, request_license,
leave_comment, leave_message

Endpoint: `https://kapoost-humanmcp.fly.dev/mcp`

---

## Templates

All in `templates.go` as `allTemplates`.

| Template | Purpose |
|---|---|
| `index.html` | Two-column list: title left, thumbnail/excerpt right |
| `piece.html` | Signing block, license, leave a message |
| `new.html` | Create/edit + Option D image upload |
| `images.html` | Image gallery |
| `dashboard.html` | Stats, hourly chart, funnel |
| `contact.html` | "re: Title" badge, no dropdown |
| `connect.html` | MCP guide, live tool count |
| `login.html` | Owner login |
| `messages.html` | Messages list |
| `css` | Dark mode, colored badges, two-column layout |
| `header` | Owner bar: + post, + image, gallery, messages, stats |
| `footer` | Connect MCP · GitHub · v0.2 |

**Funcs:** `formatDate`, `lower`, `truncate`, `nl2br`, `join`, `isoDate`, `slice`, `not`, `filenameFromRef`, `licenseLabel`

---

## Changelog

### v0.2 (April 2026)
- **Option D** — unified timestamp slug: piece + blob created atomically, thumbnail instant
- **Two-column index** — title/tags left, thumbnail or italic excerpt right
- **Signing info block** on piece pages — ED25519 + copy sig, license label, leave a message
- **Simplified contact** — no dropdown, hidden `regarding`, "re: Title" badge
- **Removed `/upload`** — broken route gone, `+ image` goes to `/new`
- **`handleFile` path fix** — correct `{parent(ContentDir)}/blobs/files/` path
- **Dynamic tool count** via `SetToolCounter` interface
- **Colored type badges** with dark mode
- **`truncate` and `licenseLabel`** template funcs

### v0.1
- Ed25519 signing, 12 MCP tools, dashboard, image gallery, gates, AI metadata assist
