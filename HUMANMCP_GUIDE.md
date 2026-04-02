# humanMCP — Project Guide
## Version 0.2 — April 2026

**Live:** https://kapoost-humanmcp.fly.dev
**Repo:** https://github.com/kapoost/humanmcp-go (single active repo)
**Pages:** https://kapoost.github.io/humanmcp-go
**MCP:** `io.github.kapoost/humanmcp`
**Old repo:** `kapoost/humanmcp` — archived, ignore

---

## What it is

A personal Go server that publishes poems, images, and data with:
- Ed25519 cryptographic signatures (proof of authorship)
- 12 MCP tools so AI agents can read, verify, and license your work
- Public REST API + OpenAPI 3.1 spec for ChatGPT/Gemini
- Web UI for humans at `kapoost-humanmcp.fly.dev`

---

## Project structure

```
humanmcp-go/
├── cmd/
│   ├── server/main.go          — entry point
│   └── keygen/main.go          — generate Ed25519 keys (run once)
├── docs/
│   └── index.html              — GitHub Pages landing (Artist/Dev/Agent tabs)
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
│   ├── mcp/handler.go          — 12 MCP tools, dynamic count
│   └── web/
│       ├── handler_web.go      ← THE REAL FILE (not handler.go)
│       └── templates.go        — all HTML as one Go string constant
├── deploy.zsh                  — safe deploy (always use this)
├── HUMANMCP_GUIDE.md           — this file
├── Dockerfile
├── fly.toml
└── go.mod                      — module: github.com/kapoost/humanmcp-go
```

**Critical:** Web handler is `handler_web.go` not `handler.go`.
`deploy.zsh` auto-deletes stale `handler.go` if it reappears.

---

## How to deploy

```zsh
# 1. Copy files from Claude into repo
cp ~/Downloads/handler_web.go ~/humanmcp/humanmcp/internal/web/handler_web.go
cp ~/Downloads/templates.go   ~/humanmcp/humanmcp/internal/web/templates.go
# For pages changes:
cp ~/Downloads/index.html     ~/humanmcp/humanmcp/docs/index.html

# 2. Deploy everything
cd ~/humanmcp/humanmcp
zsh deploy.zsh "your message"
```

deploy.zsh: removes stale handler.go → build → test → commit → push → fly deploy.

**Never:** `go build ./... # inline comment` — Go treats it as package args.

```zsh
fly logs --app kapoost-humanmcp   # check logs
```

---

## Git setup

**Remote:** `https://kapoost@github.com/kapoost/humanmcp-go.git`
(must use kapoost account, not lukaszkapusniak-sudo)

If push is rejected:
```zsh
git remote set-url origin https://kapoost@github.com/kapoost/humanmcp-go.git
git push --force   # if conflict after force-merge
```

---

## Fly.io

**App:** `kapoost-humanmcp` | **Region:** `ams` | **Volume:** `humanmcp_data` at `/data`

### Secrets (set once)

| Secret | Purpose |
|---|---|
| EDIT_TOKEN | Owner login |
| SIGNING_PRIVATE_KEY | Ed25519 private key (base64) |
| SIGNING_PUBLIC_KEY | Ed25519 public key (hex) |
| SECRET_KEY | Server secret |
| AUTHOR_BIO | Bio (`fly secrets set AUTHOR_BIO="..."`) |
| AUTHOR_NAME | kapoost |
| DOMAIN | kapoost-humanmcp.fly.dev |

### fly.toml env

PORT=8080, CONTENT_DIR=/data/content, AI_METADATA=true

---

## GitHub Pages

**URL:** https://kapoost.github.io/humanmcp-go
**Source:** branch `main`, folder `/docs`
**File:** `docs/index.html`

Page has three audience tabs: Artist, Developer, Agent.
Artist tab includes a human-vs-agent challenge using "Piosenka1.txt".

To update: edit `docs/index.html`, run `deploy.zsh`.

---

## AI-assisted development workflow

1. **Zip and upload** before asking for changes:
   ```zsh
   cd ~/humanmcp && zip -r humanmcp.zip humanmcp/ --exclude "*.git*" --exclude "*.DS_Store*"
   ```
2. Describe what needs changing
3. Claude generates files, tests against your zip
4. Download → copy → `zsh deploy.zsh "message"`

**Live data patch** (browser console, logged in, PascalCase keys):
```javascript
fetch('/api/blobs/OLD_SLUG', {
  method: 'PUT',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({ Slug: 'NEW_SLUG', BlobType: 'image',
    Access: 'public', MimeType: 'image/jpeg', FileRef: 'files/file.jpg', Title: 'Title' })
}).then(r=>r.json()).then(d=>{window._r=d})
```

---

## Bugs fixed (cumulative)

| Bug | Fix |
|---|---|
| `images.html` missing | Added to `allTemplates` |
| `handleDelete` swallowed errors | Returns 404/500 |
| `handleFile` wrong path | `{parent(ContentDir)}/blobs/files/slug` |
| Hardcoded `ToolCount: 12` | Dynamic via `SetToolCounter` interface |
| Image thumbnails missing | `blobImageMap()` keyed by slug + lowercase title |
| Stale `handler.go` conflicts | `deploy.zsh` auto-deletes it |
| `/upload` broken (blob-uploader.html undefined) | Route removed |
| Image slug mismatch | Option D: unified timestamp slug on creation |
| Delete form nested inside edit form | Standalone form, one-click delete |
| Git remote pointing to archived `humanmcp` | Fixed to `humanmcp-go` |

---

## Option D — unified slugs for images

When you post type=`image` with a file at `/new`:
1. Timestamp slug generated: `fmt.Sprintf("%d", time.Now().Unix())`
2. Piece saved with that slug
3. Blob created with same slug, title, access, tags — signed
4. Thumbnail appears on index immediately, no patching needed

For existing images with slug mismatch → patch via browser console (see above).

---

## Signing

Auto-signed on every save. Payload: `sha256(slug | title | body)`.

**Verify via MCP:**
- `verify_content {slug}` → verified or tamper detected
- `get_certificate {slug}` → hash, sig, originality, license

Public key in `/.well-known/mcp-server.json`. Private key in Fly secret only.

---

## Web routes

| Route | Access | Description |
|---|---|---|
| `/` | public | Index — type badges, thumbnails, excerpts |
| `/p/:slug` | public | Piece — signing block, license, leave a message |
| `/images` | public | Image gallery |
| `/files/:filename` | public | Raw file serving |
| `/connect` | public | Claude/ChatGPT/Gemini/REST connection guide |
| `/contact` | public | Comment — "re: Title" badge, no dropdown |
| `/openapi.json` | public | OpenAPI 3.1 spec for ChatGPT actions |
| `/.well-known/mcp-server.json` | public | MCP agent discovery |
| `/new` | owner | Create post or upload image (Option D) |
| `/edit/:slug` | owner | Edit post |
| `/delete/:slug` | owner | Delete (POST, standalone form) |
| `/dashboard` | owner | Stats + messages |
| `/messages` | owner | Messages list |
| `/login` `/logout` | public | Auth |

**Removed:** `/upload` (was broken, covered by `/new`)

---

## MCP tools (12)

get_author_profile, list_content, read_content, request_access, submit_answer,
list_blobs, read_blob, verify_content, get_certificate, request_license,
leave_comment, leave_message

Endpoint: `https://kapoost-humanmcp.fly.dev/mcp`

---

## Templates

All in `templates.go` as `allTemplates`.

| Template | Notes |
|---|---|
| `index.html` | Two-column: title left, thumbnail/excerpt right |
| `piece.html` | Signing block, license, leave a message |
| `new.html` | Create/edit + Option D image upload |
| `images.html` | Image gallery |
| `dashboard.html` | Stats, hourly chart |
| `contact.html` | "re: Title" badge, no dropdown, hidden `regarding` |
| `connect.html` | Claude/ChatGPT/Gemini/REST + OpenAPI |
| `login.html` | Owner login |
| `messages.html` | Messages list |
| `css` | Dark mode, colored type badges, two-column layout |
| `header` | Owner bar: + post, + image, gallery, messages, stats |
| `footer` | Connect MCP · GitHub · v0.2 |

**Funcs:** `formatDate`, `lower`, `truncate`, `nl2br`, `join`, `isoDate`,
`slice`, `not`, `filenameFromRef`, `licenseLabel`

---

## Changelog

### v0.2 (April 2026)
- Option D unified slug — image piece + blob created atomically
- Two-column index — title left, thumbnail/excerpt right
- Signing info block on piece pages — ED25519, copy sig, license, leave a message
- Simplified contact — no dropdown, "re: Title" badge
- `/openapi.json` endpoint — OpenAPI 3.1 for ChatGPT Custom GPT Actions
- `/connect` updated — Claude, ChatGPT, Gemini, REST sections
- GitHub Pages at `docs/index.html` — Artist/Developer/Agent tabs, challenge with Piosenka1.txt
- Removed `/upload` (broken)
- `handleFile` path fix
- Dynamic tool count via `SetToolCounter`
- Colored type badges + dark mode variants
- `truncate` and `licenseLabel` template funcs
- Standalone delete form (was nested inside edit form)
- Single repo (`humanmcp-go`), old `humanmcp` archived
- Git remote fixed to kapoost account

### v0.1
- Ed25519 signing, 12 MCP tools, dashboard, image gallery, gates, AI metadata assist

---

## OpenTimestamps — Bitcoin anchoring

Every piece gets a Bitcoin-anchored timestamp via [OpenTimestamps](https://opentimestamps.org), independent of this server.

### How it works

1. On save → `sha256(slug|title|body)` is submitted async to `alice.btc.calendar.opentimestamps.org`
2. The OTS stub is stored in the piece's `OTSProof` frontmatter field
3. After ~1hr, Bitcoin confirms → the stub upgrades to a full anchored proof
4. `get_certificate` shows OTS status; `upgrade_timestamp` fetches the upgraded proof

### MCP tools

| Tool | What it does |
|---|---|
| `get_certificate {slug}` | Returns cert including `ots_proof` base64 field |
| `upgrade_timestamp {slug}` | Fetches upgraded Bitcoin-anchored proof from calendar |

### Verify independently (anyone, no account)

```zsh
# 1. Get the proof from an agent
# Call: get_certificate {slug} → copy ots_proof field

# 2. Decode to file
echo "BASE64_OTS_PROOF" | base64 -d > piece.ots

# 3. Verify
pip install opentimestamps-client
ots verify piece.ots
# Returns: Good timestamp anchored in Bitcoin block XXXXXX (YYYY-MM-DD)
```

### Signing vs timestamping

| | Ed25519 signature | OpenTimestamps |
|---|---|---|
| What it proves | You authored it | It existed at this time |
| Verification needs | Your public key | Bitcoin blockchain |
| Independent of server | ✅ | ✅ |
| Tamper-evident | ✅ | ✅ |
| Timestamp trusted by | Anyone with the key | Anyone with Bitcoin |

### Where the proof is stored

In the markdown frontmatter of each piece file on the Fly.io volume:
```yaml
---
Slug: piosenki
Signature: nmP3foSeP67...
OTSProof: AE9wZW5UaW1lc3...   ← base64 OTS bytes
---
```

The `signing.go` functions involved:
- `TimestampPiece(p)` — submits to OTS calendar, returns stub
- `UpgradeTimestamp(proof)` — fetches upgraded proof
- `OTSProofInfo(proof)` — human-readable status string
