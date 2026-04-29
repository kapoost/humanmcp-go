# humanMCP Marketplace — Build Prompt

## What you are building

An open marketplace that aggregates multiple humanMCP servers into a single searchable directory. Think of it as "npm for personal AI servers" — every human runs their own humanMCP instance; the marketplace indexes them all.

The marketplace is a **separate project** from humanMCP. It does not host content — it crawls, indexes, and connects.

## Core concept

Every humanMCP server already exposes:
- `/.well-known/agent.json` — identity, capabilities, REST API
- `/.well-known/mcp-server.json` — MCP discovery metadata
- `/openapi.json` — REST API spec
- `/api/content` — list all public pieces
- `/api/search?q=` — full-text search
- `/api/blobs` — images, datasets
- `/api/profile` — author profile
- `/rss.xml` — RSS feed
- `/mcp` — MCP JSON-RPC endpoint

The marketplace crawls these endpoints. No special integration needed from humanMCP instances — they already speak the protocol.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  kapoost's  │     │  alice's    │     │  bob's      │
│  humanMCP   │     │  humanMCP   │     │  humanMCP   │
│  (Fly.io)   │     │  (Pi)       │     │  (Lambda)   │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └───────────┬───────┴───────────────────┘
                   │  crawl /.well-known/agent.json
                   │  crawl /api/content
                   │  crawl /api/search
                   ▼
          ┌────────────────┐
          │   MARKETPLACE  │
          │                │
          │  index + search│
          │  across all    │
          │  instances     │
          └────────┬───────┘
                   │
          ┌────────┴────────┐
          │  Web UI + API   │
          │  + MCP server   │
          └─────────────────┘
```

## Tech stack

- **Backend**: Go (same ecosystem as humanMCP)
- **Storage**: SQLite for index, no heavy infra
- **Search**: FTS5 (SQLite full-text search) — no vector DB needed at this scale
- **Deploy**: Fly.io single machine, or any Docker host
- **Frontend**: server-rendered HTML, same monospace aesthetic as humanMCP

## Data model

```sql
-- Registered humanMCP instances
CREATE TABLE servers (
    domain      TEXT PRIMARY KEY,        -- e.g. kapoost-humanmcp.fly.dev
    name        TEXT NOT NULL,           -- author name
    bio         TEXT,
    tags        TEXT,                    -- comma-separated interests
    protocol    TEXT DEFAULT 'MCP/2025-03-26',
    mcp_url     TEXT,                    -- /mcp endpoint
    rest_url    TEXT,                    -- base URL for REST
    openapi_url TEXT,
    agent_json  TEXT,                    -- cached agent.json
    last_crawl  DATETIME,
    status      TEXT DEFAULT 'active',   -- active, unreachable, removed
    registered  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexed content across all servers
CREATE TABLE pieces (
    id          INTEGER PRIMARY KEY,
    server      TEXT REFERENCES servers(domain),
    slug        TEXT NOT NULL,
    title       TEXT NOT NULL,
    type        TEXT,                    -- poem, essay, note, image, etc.
    access      TEXT DEFAULT 'public',
    body        TEXT,                    -- for FTS indexing (public pieces only)
    tags        TEXT,
    description TEXT,
    signature   TEXT,                    -- Ed25519 sig from origin
    published   DATETIME,
    indexed_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(server, slug)
);

-- FTS5 virtual table for fast search
CREATE VIRTUAL TABLE pieces_fts USING fts5(
    title, body, tags, description,
    content=pieces,
    content_rowid=id
);

-- Listings across all servers
CREATE TABLE listings (
    id          INTEGER PRIMARY KEY,
    server      TEXT REFERENCES servers(domain),
    slug        TEXT NOT NULL,
    title       TEXT NOT NULL,
    type        TEXT,                    -- sell, buy, offer, trade
    price       TEXT,
    body        TEXT,
    tags        TEXT,
    status      TEXT DEFAULT 'open',
    published   DATETIME,
    UNIQUE(server, slug)
);
```

## API design

### Public REST API

```
GET  /                          → web UI: browse servers and content
GET  /servers                   → list all registered servers
GET  /servers/{domain}          → server detail + its content
GET  /search?q={query}          → federated search across all servers
GET  /search?q={query}&type=poem → filter by content type
GET  /search?q={query}&server=kapoost → filter by server
GET  /feed                      → aggregated RSS/JSON feed
GET  /tags                      → tag cloud across all servers
GET  /tags/{tag}                → all content with this tag
GET  /random                    → random public piece from any server
GET  /openapi.json              → OpenAPI spec
GET  /.well-known/agent.json    → marketplace agent card
```

### Registration API

```
POST /register                  → register a humanMCP instance
     body: { "domain": "kapoost-humanmcp.fly.dev" }

     The marketplace will:
     1. Fetch /.well-known/agent.json to verify it's a humanMCP
     2. Fetch /api/content to index public pieces
     3. Add to crawl schedule

DELETE /servers/{domain}        → owner removes their instance
       auth: must prove ownership (e.g. DNS TXT record or token from their server)
```

### MCP endpoint

```
POST /mcp                       → MCP JSON-RPC 2.0

Tools:
- search_marketplace(query, type?, server?) → federated search
- list_servers() → all registered humanMCP instances
- get_server(domain) → server detail
- read_piece(domain, slug) → proxied read from origin server
- random_piece(type?) → discover something new
- list_tags() → what people write about
```

## Crawl logic

```go
// Every 6 hours per server:
func crawl(domain string) {
    // 1. Check health
    agent, err := fetch(domain + "/.well-known/agent.json")
    if err → mark unreachable, skip

    // 2. Index content
    pieces, _ := fetch(domain + "/api/content")
    for _, p := range pieces {
        // Only index public pieces
        if p.Access != "public" { continue }
        upsert(pieces_table, p)
        updateFTS(p)
    }

    // 3. Index listings
    listings, _ := fetch(domain + "/listings/feed.json")
    for _, l := range listings {
        upsert(listings_table, l)
    }

    // 4. Update server metadata
    update(servers_table, domain, agent)
}
```

## Web UI

Monospace terminal aesthetic, matching humanMCP's mIRC style.

### Home page

```
┌──────────────────────────────────────────────────┐
│  [ humanMCP marketplace ]                        │
│  ─────────────────────────────────────────────── │
│                                                  │
│  [/] search    12 servers · 147 pieces · 23 tags │
│                                                  │
│  --- recent ──────────────────────────────────── │
│  29 Apr  "Suma człowieczeństwa"  poem   kapoost │
│  28 Apr  "On Recursion"          essay  alice    │
│  27 Apr  "Port w deszczu"        poem   kapoost │
│  26 Apr  "Neural Lullaby"        poem   bob      │
│                                                  │
│  --- servers ─────────────────────────────────── │
│  kapoost    sailor, poet, CTO       14 pieces   │
│  alice      researcher, writer       8 pieces   │
│  bob        musician, developer     23 pieces   │
│                                                  │
│  --- tags ────────────────────────────────────── │
│  #sea(12) #ai(9) #code(7) #music(5) #sailing(4)│
│                                                  │
│  Poems written by humans · rss · about           │
└──────────────────────────────────────────────────┘
```

### Search results

```
┌──────────────────────────────────────────────────┐
│  search: "morze"                    3 results    │
│  ─────────────────────────────────────────────── │
│                                                  │
│  kapoost — "Port w deszczu"                poem  │
│  …sztorm nadchodzi z morza, światła gasną…       │
│  → https://kapoost-humanmcp.fly.dev/p/port       │
│                                                  │
│  alice — "Baltic notes"                    essay  │
│  …the Baltic sea remembers everything…           │
│  → https://alice-humanmcp.fly.dev/p/baltic       │
│                                                  │
│  kapoost — "Akt III"                       poem  │
│  …morze szumi pod oknem, nie pozwala spać…       │
│  → https://kapoost-humanmcp.fly.dev/p/akt-iii    │
└──────────────────────────────────────────────────┘
```

## Key principles

1. **No content hosting** — marketplace indexes metadata + public text for search. Full content is always served from the origin humanMCP. Links point back to the author's server.

2. **No accounts required** — registration is just providing your domain. Verification is automated (check for valid humanMCP at that domain).

3. **Respect access controls** — only index `public` pieces. Locked, members-only, gated content is listed by title only with "🔒 locked" badge.

4. **Verify signatures** — display Ed25519 signature status. Marketplace can verify pieces are authentically signed by the origin server.

5. **Opt-in, opt-out** — servers register voluntarily. They can remove themselves anytime. Marketplace respects `robots.txt` and any `X-HumanMCP-Index: no` header.

6. **Federated, not centralized** — marketplace is one possible aggregator. Anyone can run their own. The protocol (agent.json + REST API) is the standard, not this marketplace.

7. **Attribution always** — every search result, every listing links back to the origin. Author name is always visible. No anonymous aggregation.

## Registration flow

```
Human: "I want to list my humanMCP on the marketplace"

1. Human gives their domain: kapoost-humanmcp.fly.dev
2. Marketplace fetches /.well-known/agent.json
3. Validates it's a legit humanMCP (has agentInteraction.protocol)
4. Fetches /api/content, indexes public pieces
5. Generates verification token
6. Human adds token to their humanMCP (env var or config)
7. Marketplace verifies by fetching /api/profile?verify={token}
8. Done — server appears in directory within minutes
```

## Compatibility contract

The marketplace relies on these humanMCP endpoints (all already implemented):

| Endpoint | Required | Purpose |
|---|---|---|
| `/.well-known/agent.json` | yes | Identity + capabilities |
| `/api/content` | yes | List all pieces |
| `/api/search?q=` | recommended | Federated search passthrough |
| `/api/blobs` | optional | Image/file index |
| `/openapi.json` | optional | Machine-readable API spec |
| `/rss.xml` | optional | RSS feed |
| `/listings/feed.json` | optional | Listings aggregation |
| `/api/profile` | optional | Public profile |

Any server exposing `agent.json` + `/api/content` is marketplace-compatible.

## Implementation order

1. **SQLite schema + crawl one server** (kapoost's) — prove the index works
2. **Search API** — FTS5 across indexed content
3. **Web UI** — monospace terminal style, search + browse
4. **Registration endpoint** — POST /register
5. **MCP server** — so agents can search the marketplace via MCP
6. **Scheduled crawl** — goroutine, every 6h per server
7. **RSS aggregation** — combined feed
8. **Signature verification** — verify Ed25519 sigs on indexed content

## What this enables

- Agent asks: "find me poems about the sea by any human" → marketplace searches all servers
- Human browses: "who else runs humanMCP?" → server directory
- Discovery: "show me all people writing about AI ethics" → tag-based browsing
- Trade: "list all open offers across all humanMCPs" → listing aggregation
- Verification: "is this poem really by kapoost?" → Ed25519 sig check against origin

## Name suggestions

- `humanMCP.directory`
- `hive.humanmcp.net`
- `index.humanmcp.net`
- `agora` (Greek marketplace)
- `rynek` (Polish for marketplace/town square)

## File structure

```
marketplace/
├── cmd/server/main.go
├── internal/
│   ├── crawl/          # crawler + scheduler
│   ├── index/          # SQLite + FTS5
│   ├── api/            # REST + MCP handlers
│   └── web/            # templates + static
├── schema.sql
├── Dockerfile
├── fly.toml
└── README.md
```

## Example: first crawl output

```json
{
  "servers": 1,
  "pieces_indexed": 14,
  "listings_indexed": 1,
  "tags_found": ["ai", "prompts", "multi-agent", "sea", "sailing", "code"],
  "search_ready": true,
  "next_crawl": "2026-04-29T15:00:00Z"
}
```

---

*This marketplace exists because every human deserves to be found — not by an algorithm, but by another mind looking for what they wrote.*
