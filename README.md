# humanMCP

A personal content server speaking Model Context Protocol (MCP/JSON-RPC 2.0).

**Live:** https://kapoost.humanmcp.net
**Landing page:** https://humanmcp.net
**Marketplace:** https://marketplace.humanmcp.net
**Network explorer:** https://humanmcp.net/humannetwork.html
**Author:** kapoost (Łukasz Kapuśniak) — poet, builder, sailor. Warsaw / Malta.

## What it is

humanMCP lets you publish poems, essays, notes, images, listings, and typed data artifacts with cryptographic proof of authorship, explicit license terms, and full control over who can access what. AI agents connect via MCP and interact with your content natively.

Every human can run their own instance. One server, one person, their rules.

## MCP Tools (30+)

**Content & Discovery**
| Tool | Description |
|---|---|
| `get_author_profile` | Who is kapoost — bio, content overview, how to browse |
| `list_content` | Browse all pieces with metadata, filter by type or tag |
| `read_content` | Read a piece — respects all access gates |
| `search_content` | Full-text search across all pieces |
| `request_access` | Get gate details for locked content |
| `submit_answer` | Unlock challenge-gated content |
| `list_blobs` | Browse typed data artifacts |
| `read_blob` | Read image, contact, dataset, vector (respects audience) |

**IP & Verification**
| Tool | Description |
|---|---|
| `verify_content` | Verify Ed25519 signature |
| `get_certificate` | Full IP certificate: license, price, originality index, hash, signature |
| `upgrade_timestamp` | Upgrade OTS proof to Bitcoin-anchored |

**Interaction**
| Tool | Description |
|---|---|
| `request_license` | Declare intended use, get terms, logged for audit |
| `leave_comment` | Leave a reaction — visible in author dashboard |
| `leave_message` | Send a direct note (max 2000 chars, URLs welcome) |

**Session & Context**
| Tool | Description |
|---|---|
| `bootstrap_session` | Unlock private context with session code |
| `recall` | Retrieve saved memories |
| `remember` | Save observations |
| `query_vault` | Search personal knowledge vault |
| `list_vault` | List vault documents |

**Skills**
| Tool | Description |
|---|---|
| `list_skills` / `get_skill` | Agent instruction catalog |
| `upsert_skill` / `delete_skill` | Manage skills (agent token) |

**Listings**
| Tool | Description |
|---|---|
| `list_listings` / `read_listing` | Browse classified ads |
| `respond_to_listing` | Send response to listing |
| `subscribe_listings` / `unsubscribe_listings` | Webhook subscriptions |

**Meta**
| Tool | Description |
|---|---|
| `about_humanmcp` | Open-source project info |

## Connect

```json
{
  "mcpServers": {
    "kapoost": {
      "type": "http",
      "url": "https://kapoost.humanmcp.net/mcp"
    }
  }
}
```

## Content types

**Pieces** (Markdown files):
- Types: `poem`, `essay`, `note`, `contact`
- Access: `public` / `members` / `locked`
- Gates: `challenge` (Q&A), `time`, `manual`, `trade`
- Licenses: `free`, `cc-by`, `cc-by-nc`, `commercial`, `exclusive`, `all-rights`

**Blobs** (typed data artifacts):
- Types: `image`, `contact`, `vector`, `document`, `dataset`, `capsule`
- Audience: `[agent:claude, human:alice, agent:*]`
- Auto-signed on save if SIGNING_PRIVATE_KEY is set

## Ecosystem

- **[humanMCP Marketplace](https://marketplace.humanmcp.net)** — federated search across all humanMCP servers. Find listings, offers, trades by humans. [MCP endpoint](https://marketplace.humanmcp.net/mcp) · [Source](https://github.com/kapoost/humanmcp-marketplace)
- **[humanNetwork](https://humanmcp.net/humannetwork.html)** — visual explorer aggregating content from the growing network of humanMCP servers

## Contact

Public links: `read_blob slug:"kapoost-contact"` — name, handle, github, instagram, facebook, landing page.

Private email: `read_content slug:"kapoost-contact-private"` — gated. Answer the challenge to access.

## Intellectual property

Every piece is signed with Ed25519. `get_certificate` returns:
- SHA-256 content hash
- Ed25519 signature + public key
- **Originality Index** (0.0–1.0): burstiness (Fano Factor), lexical density (CTTR), Shannon entropy, structural signature — grades S/A/B/C/D
- License terms and price in sats (for commercial licenses)

## Discovery & REST API

**Agent discovery:**
- `/.well-known/agent.json` — agent profile card
- `/.well-known/mcp-server.json` — MCP server discovery
- `/openapi.json` — OpenAPI 3.1 spec (ChatGPT, Gemini)
- `/llms.txt` — LLM preferences (signed)
- `/for-agents` — agent onboarding page
- `/connect` — connection methods page

**REST API (for agents without MCP):**
- `GET /api/content` — list all pieces
- `GET /api/content/{slug}` — read piece
- `GET /api/search?q=...` — full-text search
- `GET /api/profile` — author name, bio, tags
- `GET /api/blobs` — list data artifacts
- `GET /listings/feed.json` — listings feed

**SEO:**
- `robots.txt`, `sitemap.xml`, `humans.txt`

## Limits

| Field | Limit |
|---|---|
| Message / comment text | 2000 chars |
| Blob inline text | 512 KB |
| File upload | 50 MB |
| Slug | 64 chars |
| Title | 256 chars |

## Stack

- Go 1.22, zero external dependencies
- Fly.io (region: waw), persistent volume at `/data`
- Ed25519 signing (stdlib crypto)
- Plain Markdown files as database
- No JS except 8-line drag-drop on `/new` page

## Run locally

```bash
go build ./cmd/server/
EDIT_TOKEN=secret AUTHOR_NAME=yourname ./server
```

## Deploy

```bash
fly launch --name yourname-humanmcp
fly secrets set EDIT_TOKEN=secret AUTHOR_NAME=yourname
fly deploy
```

## Signing keys (optional but recommended)

```bash
go run ./cmd/keygen/
fly secrets set SIGNING_PRIVATE_KEY="..." SIGNING_PUBLIC_KEY="..."
```

## Future

- C2PA manifest embedding for blob files (when CA trust chain opens to individuals)
- Lightning Network payment gate for commercial licenses
- Scored conversational gate (agent brings API key, Claude evaluates answers)
- IP rate limiting + engagement tokens for anti-spam

## Tests

136 tests across content, MCP, and upload/signature/license suites.

```bash
go test ./...
```
